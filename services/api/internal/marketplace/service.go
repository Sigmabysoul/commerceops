package marketplace

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/audit"
	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/marketplace/flipkart"
	"github.com/commerceops/commerceops/services/api/internal/platform/objectstorage"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MaxUploadBytes = 20 << 20

var (
	ErrInvalidFile = errors.New("invalid PDF file")
	ErrNotFound    = errors.New("processing job not found")
	ErrJobActive   = errors.New("processing job is already active")
)

type Service struct {
	db         *pgxpool.Pool
	authorizer *authorization.Service
	audit      audit.Recorder
	storage    objectstorage.Storage
	extractor  pdfextractor.Extractor
	wake       chan struct{}
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
	Page     *int   `json:"source_page"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
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

func NewService(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor) *Service {
	s := newService(db, authorizer, storage, extractor)
	go s.recoverJobs()
	for range 2 {
		go s.worker()
	}
	return s
}
func newService(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor) *Service {
	return &Service{db: db, authorizer: authorizer, storage: storage, extractor: extractor, wake: make(chan struct{}, 1)}
}
func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
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
	if len(data) == 0 || len(data) > MaxUploadBytes || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return UploadResult{}, ErrInvalidFile
	}
	hashBytes := sha256.Sum256(data)
	hash := hex.EncodeToString(hashBytes[:])
	if existing, err := s.findDuplicate(ctx, p.CompanyID, hash); err == nil {
		return UploadResult{Job: existing, DuplicateSource: true}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return UploadResult{}, err
	}
	sourceID, err := randomUUID()
	if err != nil {
		return UploadResult{}, err
	}
	storageKey := path.Join(p.CompanyID, sourceID+".pdf")
	if err = s.storage.Put(ctx, storageKey, bytes.NewReader(data), int64(len(data)), "application/pdf"); err != nil {
		return UploadResult{}, err
	}
	keep := false
	defer func() {
		if !keep {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.storage.Delete(cleanupCtx, storageKey)
		}
	}()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var inserted string
	err = tx.QueryRow(ctx, `INSERT INTO source_files(id,company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,$2,'flipkart',$3,$4,'application/pdf',$5,$6,$7) ON CONFLICT(company_id,marketplace_key,sha256) DO NOTHING RETURNING id`, sourceID, p.CompanyID, storageKey, safeFilename(filename), len(data), hash, p.UserID).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		existing, findErr := s.findDuplicate(ctx, p.CompanyID, hash)
		if findErr != nil {
			return UploadResult{}, findErr
		}
		return UploadResult{Job: existing, DuplicateSource: true}, nil
	}
	if err != nil {
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
	keep = true
	s.signal()
	return UploadResult{Job: job}, nil
}
func (s *Service) findDuplicate(ctx context.Context, companyID, hash string) (Job, error) {
	var job Job
	err := s.db.QueryRow(ctx, `SELECT j.id,j.status,j.parser_version,j.total_pages,j.processed_pages,j.created_at,j.updated_at FROM source_files f JOIN processing_jobs j ON j.company_id=f.company_id AND j.source_file_id=f.id WHERE f.company_id=$1 AND f.marketplace_key='flipkart' AND f.sha256=$2 AND j.marketplace_key='flipkart' ORDER BY j.created_at DESC LIMIT 1`, companyID, hash).Scan(&job.ID, &job.Status, &job.ParserVersion, &job.TotalPages, &job.ProcessedPages, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return job, err
}

func (s *Service) Get(ctx context.Context, p auth.Principal, id string) (JobDetails, error) {
	if err := s.authorizer.RequireModule(ctx, p, "flipkart"); err != nil {
		return JobDetails{}, err
	}
	if err := s.authorizer.RequirePermission(ctx, p, "labels.process"); err != nil {
		return JobDetails{}, err
	}
	var out JobDetails
	err := s.db.QueryRow(ctx, `SELECT id,status,parser_version,total_pages,processed_pages,created_at,updated_at FROM processing_jobs WHERE company_id=$1 AND id=$2 AND marketplace_key='flipkart'`, p.CompanyID, id).Scan(&out.Job.ID, &out.Job.Status, &out.Job.ParserVersion, &out.Job.TotalPages, &out.Job.ProcessedPages, &out.Job.CreatedAt, &out.Job.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobDetails{}, ErrNotFound
	}
	if err != nil {
		return JobDetails{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,source_page,marketplace_order_id,awb,status FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2 AND marketplace_key='flipkart' ORDER BY source_page,id`, p.CompanyID, id)
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
		ir, queryErr := s.db.Query(ctx, `SELECT raw_sku,product_id,quantity,quantity_source,resolution_status,warnings FROM marketplace_order_items WHERE company_id=$1 AND order_id=$2`, p.CompanyID, o.ID)
		if queryErr != nil {
			return JobDetails{}, queryErr
		}
		o.Items = []Item{}
		for ir.Next() {
			var item Item
			if queryErr = ir.Scan(&item.RawSKU, &item.ProductID, &item.Quantity, &item.QuantitySource, &item.ResolutionStatus, &item.Warnings); queryErr != nil {
				ir.Close()
				return JobDetails{}, queryErr
			}
			o.Items = append(o.Items, item)
		}
		if queryErr = ir.Err(); queryErr != nil {
			ir.Close()
			return JobDetails{}, queryErr
		}
		ir.Close()
		out.Orders = append(out.Orders, o)
	}
	if err = rows.Err(); err != nil {
		return JobDetails{}, err
	}
	erows, err := s.db.Query(ctx, `SELECT source_page,severity,code,message FROM processing_errors WHERE company_id=$1 AND processing_job_id=$2 ORDER BY source_page NULLS FIRST,created_at`, p.CompanyID, id)
	if err != nil {
		return JobDetails{}, err
	}
	defer erows.Close()
	out.Errors = []ErrorItem{}
	for erows.Next() {
		var item ErrorItem
		if err = erows.Scan(&item.Page, &item.Severity, &item.Code, &item.Message); err != nil {
			return JobDetails{}, err
		}
		out.Errors = append(out.Errors, item)
	}
	return out, erows.Err()
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
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM processing_jobs WHERE company_id=$1 AND id=$2 AND marketplace_key='flipkart' FOR UPDATE`, p.CompanyID, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	if status == "queued" || status == "processing" {
		return Job{}, ErrJobActive
	}
	if _, err = tx.Exec(ctx, `DELETE FROM marketplace_order_items WHERE company_id=$1 AND order_id IN(SELECT id FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2 AND marketplace_key='flipkart')`, p.CompanyID, id); err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2 AND marketplace_key='flipkart'`, p.CompanyID, id); err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM processing_errors WHERE company_id=$1 AND processing_job_id=$2`, p.CompanyID, id); err != nil {
		return Job{}, err
	}
	var job Job
	err = tx.QueryRow(ctx, `UPDATE processing_jobs SET status='queued',total_pages=0,processed_pages=0,started_at=NULL,completed_at=NULL,updated_at=now(),parser_version=$1 WHERE company_id=$2 AND id=$3 AND marketplace_key='flipkart' RETURNING id,status,parser_version,total_pages,processed_pages,created_at,updated_at`, flipkart.ParserVersion, p.CompanyID, id).Scan(&job.ID, &job.Status, &job.ParserVersion, &job.TotalPages, &job.ProcessedPages, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return Job{}, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "flipkart.processing_retried", "processing_job", id, map[string]any{"previous_status": status}); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	s.signal()
	return job, nil
}

func (s *Service) worker() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		for {
			succeeded, err := s.processNext()
			if err != nil || !succeeded {
				break
			}
		}
		select {
		case <-s.wake:
		case <-ticker.C:
		}
	}
}
func (s *Service) recoverJobs() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := s.db.Exec(ctx, `UPDATE processing_jobs SET status='queued',started_at=NULL,updated_at=now() WHERE marketplace_key='flipkart' AND status='processing'`)
	if err == nil {
		s.signal()
	}
}
func (s *Service) claim(ctx context.Context) (work, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return work{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item work
	err = tx.QueryRow(ctx, `SELECT j.company_id,f.uploaded_by,j.id,f.storage_key FROM processing_jobs j JOIN source_files f ON f.company_id=j.company_id AND f.id=j.source_file_id AND f.marketplace_key='flipkart' WHERE j.marketplace_key='flipkart' AND j.status='queued' ORDER BY j.created_at FOR UPDATE OF j SKIP LOCKED LIMIT 1`).Scan(&item.CompanyID, &item.UserID, &item.JobID, &item.StorageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return work{}, false, nil
	}
	if err != nil {
		return work{}, false, err
	}
	result, err := tx.Exec(ctx, `UPDATE processing_jobs SET status='processing',started_at=now(),updated_at=now() WHERE company_id=$1 AND id=$2 AND marketplace_key='flipkart' AND status='queued'`, item.CompanyID, item.JobID)
	if err != nil {
		return work{}, false, err
	}
	if result.RowsAffected() != 1 {
		return work{}, false, nil
	}
	if err = tx.Commit(ctx); err != nil {
		return work{}, false, err
	}
	return item, true, nil
}
func (s *Service) processNext() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	item, claimed, err := s.claim(ctx)
	if err != nil || !claimed {
		return claimed, err
	}
	if err = s.execute(ctx, item); err != nil {
		failureCtx, failureCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer failureCancel()
		code,message:=classifyWorkerError(err)
		if failErr := s.failJob(failureCtx, item, code, message); failErr != nil {
			return true, fmt.Errorf("process error: %v; persist failure: %w", err, failErr)
		}
	}
	return true, nil
}
func (s *Service) execute(ctx context.Context, w work) error {
	object, err := s.storage.Get(ctx, w.StorageKey)
	if err != nil {
		return fmt.Errorf("storage read failed: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(object, MaxUploadBytes+1))
	closeErr := object.Close()
	if err != nil {
		return fmt.Errorf("storage read failed: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("storage close failed: %w", closeErr)
	}
	if len(data) > MaxUploadBytes {
		return ErrInvalidFile
	}
	pages, err := s.extractor.Extract(ctx, data)
	if err != nil {
		return fmt.Errorf("PDF extraction failed: %w", err)
	}
	labels, err := flipkart.Parse(pages)
	if err != nil {
		return fmt.Errorf("Flipkart parse failed: %w", err)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var sourceID string
	if err = tx.QueryRow(ctx, `SELECT source_file_id FROM processing_jobs WHERE company_id=$1 AND id=$2 AND marketplace_key='flipkart' AND status='processing' FOR UPDATE`, w.CompanyID, w.JobID).Scan(&sourceID); err != nil {
		return err
	}
	if err = s.audit.Record(ctx, tx, w.CompanyID, w.UserID, "flipkart.processing_started", "processing_job", w.JobID, map[string]any{"parser_version": flipkart.ParserVersion}); err != nil {
		return err
	}
	needsReview := false
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
			var id string
			resolutionErr := tx.QueryRow(ctx, `SELECT m.product_id FROM sku_mappings m JOIN products p ON p.company_id=m.company_id AND p.id=m.product_id WHERE m.company_id=$1 AND m.marketplace_key='flipkart' AND m.sku=$2 AND m.status='active' AND p.status='active'`, w.CompanyID, label.SKU).Scan(&id)
			if resolutionErr == nil {
				productID = &id
			} else if errors.Is(resolutionErr, pgx.ErrNoRows) {
				status = "needs_review"
				warnings = append(warnings, "unresolved_sku")
			} else {
				return resolutionErr
			}
		}
		if label.Quantity == nil {
			status = "needs_review"
			warnings = append(warnings, "missing_quantity")
		}
		for _, identifier := range []string{label.AWB, label.OrderID} {
			if identifier != "" {
				if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, w.CompanyID+"|flipkart|"+identifier); err != nil {
					return err
				}
			}
		}
		var duplicate bool
		if label.AWB != "" || label.OrderID != "" {
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM marketplace_orders WHERE company_id=$1 AND marketplace_key='flipkart' AND status<>'duplicate' AND (($2<>'' AND awb=$2) OR ($3<>'' AND marketplace_order_id=$3)))`, w.CompanyID, label.AWB, label.OrderID).Scan(&duplicate); err != nil {
				return err
			}
		}
		if duplicate {
			status = "duplicate"
			warnings = append(warnings, "duplicate_identifier")
		}
		if status != "resolved" {
			needsReview = true
		}
		metadata, _ := json.Marshal(map[string]any{"extractor": "poppler", "fields_detected": map[string]bool{"awb": label.AWB != "", "order_id": label.OrderID != "", "sku": label.SKU != "", "quantity": label.Quantity != nil}})
		var orderID string
		if err = tx.QueryRow(ctx, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,awb,status,parser_version,extraction_metadata) VALUES($1,'flipkart',$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9) RETURNING id`, w.CompanyID, sourceID, w.JobID, label.Page, label.OrderID, label.AWB, status, flipkart.ParserVersion, metadata).Scan(&orderID); err != nil {
			return err
		}
		warningJSON, _ := json.Marshal(warnings)
		quantitySource := "extracted"
		if label.Quantity == nil {
			quantitySource = "missing"
		}
		resolutionStatus := "resolved"
		if productID == nil {
			resolutionStatus = "unresolved"
		}
		var sku *string
		if label.SKU != "" {
			sku = &label.SKU
		}
		if _, err = tx.Exec(ctx, `INSERT INTO marketplace_order_items(company_id,order_id,raw_sku,product_id,quantity,quantity_source,resolution_status,warnings) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, w.CompanyID, orderID, sku, productID, label.Quantity, quantitySource, resolutionStatus, warningJSON); err != nil {
			return err
		}
		for _, code := range warnings {
			if _, err = tx.Exec(ctx, `INSERT INTO processing_errors(company_id,processing_job_id,source_page,severity,code,message) VALUES($1,$2,$3,'warning',$4,$5)`, w.CompanyID, w.JobID, label.Page, strings.ToUpper(code), strings.ReplaceAll(code, "_", " ")); err != nil {
				return err
			}
		}
	}
	finalStatus := "processed"
	if needsReview {
		finalStatus = "needs_review"
	}
	if _, err = tx.Exec(ctx, `UPDATE processing_jobs SET status=$1,total_pages=$2,processed_pages=$2,completed_at=now(),updated_at=now() WHERE company_id=$3 AND id=$4 AND marketplace_key='flipkart' AND status='processing'`, finalStatus, len(pages), w.CompanyID, w.JobID); err != nil {
		return err
	}
	if err = s.audit.Record(ctx, tx, w.CompanyID, w.UserID, "flipkart.processing_completed", "processing_job", w.JobID, map[string]any{"status": finalStatus, "pages": len(pages), "labels": len(labels)}); err != nil {
		return err
	}
	if err=tx.Commit(ctx);err!=nil{return fmt.Errorf("processing commit failed: %w",err)}
	return nil
}
func (s *Service) failJob(ctx context.Context, w work, code, message string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `INSERT INTO processing_errors(company_id,processing_job_id,severity,code,message) SELECT $1,$2,'error',$3,$4 WHERE EXISTS(SELECT 1 FROM processing_jobs WHERE company_id=$1 AND id=$2 AND marketplace_key='flipkart')`, w.CompanyID, w.JobID, code, message); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE processing_jobs SET status='failed',completed_at=now(),updated_at=now() WHERE company_id=$1 AND id=$2 AND marketplace_key='flipkart' AND status='processing'`, w.CompanyID, w.JobID); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
func randomUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	raw:=hex.EncodeToString(value)
	return raw[0:8]+"-"+raw[8:12]+"-"+raw[12:16]+"-"+raw[16:20]+"-"+raw[20:32],nil
}
func classifyWorkerError(err error)(string,string){value:=err.Error();switch{case strings.Contains(value,"storage") :return "STORAGE_READ_FAILED","Stored source file could not be read";case strings.Contains(value,"PDF extraction"):return "PDF_EXTRACTION_FAILED","PDF text extraction failed or exceeded a resource limit";case strings.Contains(value,"Flipkart parse"):return "UNSUPPORTED_FLIPKART_DOCUMENT","No supported Flipkart label could be extracted";case strings.Contains(value,"commit"):return "PROCESSING_COMMIT_FAILED","Processing results could not be committed";default:return "PROCESSING_DATABASE_FAILED","Processing could not be completed"}}
