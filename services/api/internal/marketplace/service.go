package marketplace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/audit"
	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/marketplace/flipkart"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MaxUploadBytes = 20 << 20

var (
	ErrInvalidFile = errors.New("invalid PDF file")
	ErrNotFound    = errors.New("processing job not found")
	ErrQueueFull   = errors.New("processing queue is full")
)

type Service struct {
	db         *pgxpool.Pool
	authorizer *authorization.Service
	audit      audit.Recorder
	storageDir string
	queue      chan work
}
type work struct{ CompanyID, UserID, JobID, StorageKey string }
type UploadResult struct {
	Job             Job  `json:"job"`
	DuplicateSource bool `json:"duplicate_source"`
}
type Job struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	ParserVersion  string    `json:"parser_version"`
	TotalPages     int       `json:"total_pages"`
	ProcessedPages int       `json:"processed_pages"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type ErrorItem struct {
	Page                    *int `json:"source_page"`
	Severity, Code, Message string
}
type Item struct {
	RawSKU           *string         `json:"raw_sku"`
	ProductID        *string         `json:"product_id"`
	Quantity         *int            `json:"quantity"`
	QuantitySource   string          `json:"quantity_source"`
	ResolutionStatus string          `json:"resolution_status"`
	Warnings         json.RawMessage `json:"warnings"`
}
type Order struct {
	ID                 string  `json:"id"`
	SourcePage         int     `json:"source_page"`
	MarketplaceOrderID *string `json:"marketplace_order_id"`
	AWB                *string `json:"awb"`
	Status             string  `json:"status"`
	Items              []Item  `json:"items"`
}
type JobDetails struct {
	Job    Job         `json:"job"`
	Orders []Order     `json:"orders"`
	Errors []ErrorItem `json:"errors"`
}

func NewService(db *pgxpool.Pool, authorizer *authorization.Service, storageDir string) *Service {
	s := &Service{db: db, authorizer: authorizer, storageDir: storageDir, queue: make(chan work, 32)}
	for range 2 {
		go s.worker()
	}
	go s.recoverJobs()
	return s
}

func (s *Service) Upload(ctx context.Context, p auth.Principal, filename string, data []byte) (UploadResult, error) {
	if err := s.authorizer.RequireModule(ctx, p, "flipkart"); err != nil {
		return UploadResult{}, err
	}
	if err := s.authorizer.RequirePermission(ctx, p, "labels.upload"); err != nil {
		return UploadResult{}, err
	}
	if err := s.authorizer.RequirePermission(ctx, p, "labels.process"); err != nil {
		return UploadResult{}, err
	}
	if len(data) == 0 || len(data) > MaxUploadBytes || !strings.HasPrefix(string(data[:min(5, len(data))]), "%PDF-") {
		return UploadResult{}, ErrInvalidFile
	}
	hashBytes := sha256.Sum256(data)
	hash := hex.EncodeToString(hashBytes[:])
	var existing Job
	err := s.db.QueryRow(ctx, `SELECT j.id,j.status,j.parser_version,j.total_pages,j.processed_pages,j.created_at,j.updated_at FROM source_files f JOIN processing_jobs j ON j.company_id=f.company_id AND j.source_file_id=f.id WHERE f.company_id=$1 AND f.marketplace_key='flipkart' AND f.sha256=$2 ORDER BY j.created_at DESC LIMIT 1`, p.CompanyID, hash).Scan(&existing.ID, &existing.Status, &existing.ParserVersion, &existing.TotalPages, &existing.ProcessedPages, &existing.CreatedAt, &existing.UpdatedAt)
	if err == nil {
		return UploadResult{Job: existing, DuplicateSource: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return UploadResult{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var sourceID string
	if err = tx.QueryRow(ctx, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,'flipkart','pending',$2,'application/pdf',$3,$4,$5) RETURNING id`, p.CompanyID, safeFilename(filename), len(data), hash, p.UserID).Scan(&sourceID); err != nil {
		return UploadResult{}, err
	}
	storageKey := filepath.Join(p.CompanyID, sourceID+".pdf")
	fullPath := filepath.Join(s.storageDir, storageKey)
	if err = os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return UploadResult{}, err
	}
	if err = os.WriteFile(fullPath, data, 0o640); err != nil {
		return UploadResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(fullPath)
		}
	}()
	if _, err = tx.Exec(ctx, `UPDATE source_files SET storage_key=$1 WHERE company_id=$2 AND id=$3`, storageKey, p.CompanyID, sourceID); err != nil {
		return UploadResult{}, err
	}
	var job Job
	err = tx.QueryRow(ctx, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version) VALUES($1,$2,'flipkart','queued',$3) RETURNING id,status,parser_version,total_pages,processed_pages,created_at,updated_at`, p.CompanyID, sourceID, flipkart.ParserVersion).Scan(&job.ID, &job.Status, &job.ParserVersion, &job.TotalPages, &job.ProcessedPages, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return UploadResult{}, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "flipkart.file_uploaded", "processing_job", job.ID, map[string]any{"source_file_id": sourceID, "sha256": hash, "size_bytes": len(data)}); err != nil {
		return UploadResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return UploadResult{}, err
	}
	committed = true
	select {
	case s.queue <- work{p.CompanyID, p.UserID, job.ID, storageKey}:
	default:
		s.failJob(context.Background(), p.CompanyID, job.ID, "QUEUE_FULL", "Processing queue capacity was reached")
		return UploadResult{}, ErrQueueFull
	}
	return UploadResult{Job: job}, nil
}

func (s *Service) Get(ctx context.Context, p auth.Principal, id string) (JobDetails, error) {
	if err := s.authorizer.RequireModule(ctx, p, "flipkart"); err != nil {
		return JobDetails{}, err
	}
	if err := s.authorizer.RequirePermission(ctx, p, "labels.process"); err != nil {
		return JobDetails{}, err
	}
	var out JobDetails
	err := s.db.QueryRow(ctx, `SELECT id,status,parser_version,total_pages,processed_pages,created_at,updated_at FROM processing_jobs WHERE company_id=$1 AND id=$2`, p.CompanyID, id).Scan(&out.Job.ID, &out.Job.Status, &out.Job.ParserVersion, &out.Job.TotalPages, &out.Job.ProcessedPages, &out.Job.CreatedAt, &out.Job.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobDetails{}, ErrNotFound
	}
	if err != nil {
		return JobDetails{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,source_page,marketplace_order_id,awb,status FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2 ORDER BY source_page,id`, p.CompanyID, id)
	if err != nil {
		return JobDetails{}, err
	}
	defer rows.Close()
	out.Orders = []Order{}
	for rows.Next() {
		var o Order
		if err = rows.Scan(&o.ID, &o.SourcePage, &o.MarketplaceOrderID, &o.AWB, &o.Status); err != nil {
			return JobDetails{}, err
		}
		ir, er := s.db.Query(ctx, `SELECT raw_sku,product_id,quantity,quantity_source,resolution_status,warnings FROM marketplace_order_items WHERE company_id=$1 AND order_id=$2`, p.CompanyID, o.ID)
		if er != nil {
			return JobDetails{}, er
		}
		o.Items = []Item{}
		for ir.Next() {
			var i Item
			if er = ir.Scan(&i.RawSKU, &i.ProductID, &i.Quantity, &i.QuantitySource, &i.ResolutionStatus, &i.Warnings); er != nil {
				ir.Close()
				return JobDetails{}, er
			}
			o.Items = append(o.Items, i)
		}
		ir.Close()
		out.Orders = append(out.Orders, o)
	}
	erows, err := s.db.Query(ctx, `SELECT source_page,severity,code,message FROM processing_errors WHERE company_id=$1 AND processing_job_id=$2 ORDER BY source_page NULLS FIRST,created_at`, p.CompanyID, id)
	if err != nil {
		return JobDetails{}, err
	}
	defer erows.Close()
	out.Errors = []ErrorItem{}
	for erows.Next() {
		var e ErrorItem
		if err = erows.Scan(&e.Page, &e.Severity, &e.Code, &e.Message); err != nil {
			return JobDetails{}, err
		}
		out.Errors = append(out.Errors, e)
	}
	return out, nil
}

func (s *Service) Retry(ctx context.Context, p auth.Principal, id string) (Job, error) {
	if err := s.authorizer.RequireModule(ctx, p, "flipkart"); err != nil {
		return Job{}, err
	}
	if err := s.authorizer.RequirePermission(ctx, p, "labels.process"); err != nil {
		return Job{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item work
	item.CompanyID = p.CompanyID
	item.UserID = p.UserID
	item.JobID = id
	var status string
	if err = tx.QueryRow(ctx, `SELECT j.status,f.storage_key FROM processing_jobs j JOIN source_files f ON f.company_id=j.company_id AND f.id=j.source_file_id WHERE j.company_id=$1 AND j.id=$2 FOR UPDATE OF j`, p.CompanyID, id).Scan(&status, &item.StorageKey); errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	} else if err != nil {
		return Job{}, err
	}
	if status == "queued" || status == "processing" {
		return Job{}, ErrQueueFull
	}
	if _, err = tx.Exec(ctx, `DELETE FROM marketplace_order_items WHERE company_id=$1 AND order_id IN(SELECT id FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2)`, p.CompanyID, id); err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2`, p.CompanyID, id); err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM processing_errors WHERE company_id=$1 AND processing_job_id=$2`, p.CompanyID, id); err != nil {
		return Job{}, err
	}
	var job Job
	err = tx.QueryRow(ctx, `UPDATE processing_jobs SET status='queued',total_pages=0,processed_pages=0,started_at=NULL,completed_at=NULL,updated_at=now(),parser_version=$1 WHERE company_id=$2 AND id=$3 RETURNING id,status,parser_version,total_pages,processed_pages,created_at,updated_at`, flipkart.ParserVersion, p.CompanyID, id).Scan(&job.ID, &job.Status, &job.ParserVersion, &job.TotalPages, &job.ProcessedPages, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return Job{}, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "flipkart.processing_retried", "processing_job", id, map[string]any{"previous_status": status}); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	select {
	case s.queue <- item:
	default:
		s.failJob(context.Background(), p.CompanyID, id, "QUEUE_FULL", "Processing queue capacity was reached")
		return Job{}, ErrQueueFull
	}
	return job, nil
}

func (s *Service) worker() {
	for item := range s.queue {
		s.process(item)
	}
}
func (s *Service) recoverJobs() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = s.db.Exec(ctx, `UPDATE processing_jobs SET status='queued',started_at=NULL,updated_at=now() WHERE status='processing'`)
	rows, err := s.db.Query(ctx, `SELECT j.company_id,f.uploaded_by,j.id,f.storage_key FROM processing_jobs j JOIN source_files f ON f.company_id=j.company_id AND f.id=j.source_file_id WHERE j.status='queued' ORDER BY j.created_at`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item work
		if rows.Scan(&item.CompanyID, &item.UserID, &item.JobID, &item.StorageKey) == nil {
			s.queue <- item
		}
	}
}
func (s *Service) process(w work) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := s.db.Exec(ctx, `UPDATE processing_jobs SET status='processing',started_at=now(),updated_at=now() WHERE company_id=$1 AND id=$2 AND status='queued'`, w.CompanyID, w.JobID)
	if err != nil {
		return
	}
	if result.RowsAffected() != 1 {
		return
	}
	data, err := os.ReadFile(filepath.Join(s.storageDir, w.StorageKey))
	if err != nil {
		s.failJob(ctx, w.CompanyID, w.JobID, "STORAGE_READ_FAILED", "Stored source file could not be read")
		return
	}
	labels, err := flipkart.Parse(data)
	if err != nil {
		s.failJob(ctx, w.CompanyID, w.JobID, "UNSUPPORTED_PDF", "PDF is malformed, non-Flipkart, or uses an unsupported text encoding")
		return
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		s.failJob(ctx, w.CompanyID, w.JobID, "DATABASE_ERROR", "Processing could not start")
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	needsReview := false
	var sourceID string
	if err = tx.QueryRow(ctx, `SELECT source_file_id FROM processing_jobs WHERE company_id=$1 AND id=$2 FOR UPDATE`, w.CompanyID, w.JobID).Scan(&sourceID); err != nil {
		return
	}
	if err = s.audit.Record(ctx, tx, w.CompanyID, w.UserID, "flipkart.processing_started", "processing_job", w.JobID, map[string]any{"parser_version": flipkart.ParserVersion}); err != nil {
		return
	}
	for _, label := range labels {
		status := "resolved"
		warnings := []string{}
		var productID *string
		if label.AWB == "" {
			status = "needs_review"
			warnings = append(warnings, "missing_awb")
		}
		if label.OrderID == "" {
			warnings = append(warnings, "missing_order_id")
		}
		if label.SKU == "" {
			status = "needs_review"
			warnings = append(warnings, "missing_sku")
		} else {
			var pid string
			er := tx.QueryRow(ctx, `SELECT m.product_id FROM sku_mappings m JOIN products p ON p.company_id=m.company_id AND p.id=m.product_id WHERE m.company_id=$1 AND m.marketplace_key='flipkart' AND m.sku=$2 AND m.status='active' AND p.status='active'`, w.CompanyID, label.SKU).Scan(&pid)
			if er == nil {
				productID = &pid
			} else if errors.Is(er, pgx.ErrNoRows) {
				status = "needs_review"
				warnings = append(warnings, "unresolved_sku")
			} else {
				return
			}
		}
		if label.Quantity == nil {
			status = "needs_review"
			warnings = append(warnings, "missing_quantity")
		}
		var duplicate bool
		if label.AWB != "" || label.OrderID != "" {
			lockKey := w.CompanyID + "|flipkart|" + label.AWB + "|" + label.OrderID
			if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
				return
			}
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM marketplace_orders WHERE company_id=$1 AND marketplace_key='flipkart' AND status<>'duplicate' AND (($2<>'' AND awb=$2) OR ($3<>'' AND marketplace_order_id=$3)))`, w.CompanyID, label.AWB, label.OrderID).Scan(&duplicate); err != nil {
				return
			}
		}
		if duplicate {
			status = "duplicate"
			warnings = append(warnings, "duplicate_identifier")
		}
		if status != "resolved" {
			needsReview = true
		}
		metadata, _ := json.Marshal(map[string]any{"parser": "literal_pdf_text", "fields_detected": map[string]bool{"awb": label.AWB != "", "order_id": label.OrderID != "", "sku": label.SKU != "", "quantity": label.Quantity != nil}})
		var orderID string
		err = tx.QueryRow(ctx, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,awb,status,parser_version,extraction_metadata) VALUES($1,'flipkart',$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9) RETURNING id`, w.CompanyID, sourceID, w.JobID, label.Page, label.OrderID, label.AWB, status, flipkart.ParserVersion, metadata).Scan(&orderID)
		if err != nil {
			return
		}
		warningJSON, _ := json.Marshal(warnings)
		qsource := "extracted"
		if label.Quantity == nil {
			qsource = "missing"
		}
		resolution := "resolved"
		if productID == nil {
			resolution = "unresolved"
		}
		var sku *string
		if label.SKU != "" {
			sku = &label.SKU
		}
		if _, err = tx.Exec(ctx, `INSERT INTO marketplace_order_items(company_id,order_id,raw_sku,product_id,quantity,quantity_source,resolution_status,warnings) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, w.CompanyID, orderID, sku, productID, label.Quantity, qsource, resolution, warningJSON); err != nil {
			return
		}
		for _, code := range warnings {
			if _, err = tx.Exec(ctx, `INSERT INTO processing_errors(company_id,processing_job_id,source_page,severity,code,message) VALUES($1,$2,$3,'warning',$4,$5)`, w.CompanyID, w.JobID, label.Page, strings.ToUpper(code), strings.ReplaceAll(code, "_", " ")); err != nil {
				return
			}
		}
	}
	finalStatus := "processed"
	if needsReview {
		finalStatus = "needs_review"
	}
	if _, err = tx.Exec(ctx, `UPDATE processing_jobs SET status=$1,total_pages=$2,processed_pages=$2,completed_at=now(),updated_at=now() WHERE company_id=$3 AND id=$4`, finalStatus, len(labels), w.CompanyID, w.JobID); err != nil {
		return
	}
	if err = s.audit.Record(ctx, tx, w.CompanyID, w.UserID, "flipkart.processing_completed", "processing_job", w.JobID, map[string]any{"status": finalStatus, "labels": len(labels)}); err != nil {
		return
	}
	_ = tx.Commit(ctx)
}
func (s *Service) failJob(ctx context.Context, companyID, jobID, code, message string) {
	_, _ = s.db.Exec(ctx, `WITH e AS (INSERT INTO processing_errors(company_id,processing_job_id,severity,code,message) VALUES($1,$2,'error',$3,$4)) UPDATE processing_jobs SET status='failed',completed_at=now(),updated_at=now() WHERE company_id=$1 AND id=$2`, companyID, jobID, code, message)
}
func safeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "" {
		return "upload.pdf"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}
