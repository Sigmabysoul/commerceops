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
	"github.com/commerceops/commerceops/services/api/internal/marketplace/amazon"
	"github.com/commerceops/commerceops/services/api/internal/marketplace/flipkart"
	"github.com/commerceops/commerceops/services/api/internal/marketplace/meesho"
	"github.com/commerceops/commerceops/services/api/internal/marketplace/myntra"
	"github.com/commerceops/commerceops/services/api/internal/marketplace/snapdeal"
	"github.com/commerceops/commerceops/services/api/internal/platform/objectstorage"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MaxUploadBytes          = 20 << 20
	workerLeaseDuration     = 2 * time.Minute
	workerHeartbeatInterval = 30 * time.Second
)

var (
	ErrInvalidFile         = errors.New("invalid source file")
	ErrNotFound            = errors.New("processing job not found")
	ErrJobActive           = errors.New("processing job is already active")
	ErrLeaseLost           = errors.New("processing job lease is no longer owned")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with different content")
)

type Service struct {
	db                *pgxpool.Pool
	authorizer        *authorization.Service
	audit             audit.Recorder
	storage           objectstorage.Storage
	extractor         pdfextractor.Extractor
	wake              chan struct{}
	workerID          string
	leaseDuration     time.Duration
	heartbeatInterval time.Duration
	processor         processor
}
type normalizedRecord struct {
	Page              int
	AWB, OrderID, SKU string
	Quantity          *int
	Documents         []normalizedDocument
	Warnings          []string
	AssociationMethod string
	Confidence        string
	Metadata          map[string]any
}
type normalizedDocument struct {
	Page                   int
	Role, ExtractionMethod string
}
type processor struct {
	marketplace, parserVersion, documentName string
	auditCountKey                            string
	requireOrderID                           bool
	parse                                    func([]pdfextractor.Page) ([]normalizedRecord, error)
	parseData                                func([]byte) ([]normalizedRecord, error)
	contentType, extension                   string
	requireIdempotency                       bool
	allowMissingAWB                          bool
}
type work struct{ CompanyID, UserID, JobID, StorageKey, WorkerID string }
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
	ID                 string          `json:"id"`
	SourcePage         int             `json:"source_page"`
	MarketplaceOrderID *string         `json:"marketplace_order_id"`
	AWB                *string         `json:"awb"`
	Status             string          `json:"status"`
	Items              []Item          `json:"items"`
	Documents          []Document      `json:"documents"`
	Metadata           json.RawMessage `json:"metadata"`
}
type Document struct {
	SourcePage       int    `json:"source_page"`
	Role             string `json:"role"`
	ExtractionMethod string `json:"extraction_method"`
}
type JobDetails struct {
	Job    Job         `json:"job"`
	Orders []Order     `json:"orders"`
	Errors []ErrorItem `json:"errors"`
}

func NewService(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor) (*Service, error) {
	return newProcessingService(db, authorizer, storage, extractor, flipkartProcessor())
}
func NewAmazonService(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor) (*Service, error) {
	return newProcessingService(db, authorizer, storage, extractor, amazonProcessor())
}
func NewMeeshoService(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor) (*Service, error) {
	return newProcessingService(db, authorizer, storage, extractor, meeshoProcessor())
}
func NewMyntraService(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage) (*Service, error) {
	return newProcessingService(db, authorizer, storage, nil, myntraProcessor())
}
func NewSnapdealService(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor) (*Service, error) {
	return newProcessingService(db, authorizer, storage, extractor, snapdealProcessor())
}
func newProcessingService(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor, p processor) (*Service, error) {
	s, err := newServiceForProcessor(db, authorizer, storage, extractor, p)
	if err != nil {
		return nil, err
	}
	go s.recoverJobs()
	for range 2 {
		go s.worker()
	}
	return s, nil
}
func newService(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor) (*Service, error) {
	return newServiceForProcessor(db, authorizer, storage, extractor, flipkartProcessor())
}
func newServiceForProcessor(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor, p processor) (*Service, error) {
	workerID, err := randomUUID()
	if err != nil {
		return nil, fmt.Errorf("create worker identity: %w", err)
	}
	return newServiceWithProcessorAndWorkerID(db, authorizer, storage, extractor, workerID, p), nil
}
func newServiceWithWorkerID(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor, workerID string) *Service {
	return newServiceWithProcessorAndWorkerID(db, authorizer, storage, extractor, workerID, flipkartProcessor())
}
func newServiceWithProcessorAndWorkerID(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, extractor pdfextractor.Extractor, workerID string, p processor) *Service {
	return &Service{
		db: db, authorizer: authorizer, storage: storage, extractor: extractor,
		wake: make(chan struct{}, 1), workerID: workerID,
		leaseDuration: workerLeaseDuration, heartbeatInterval: workerHeartbeatInterval, processor: p,
	}
}
func flipkartProcessor() processor {
	return processor{marketplace: "flipkart", parserVersion: flipkart.ParserVersion, documentName: "Flipkart", auditCountKey: "labels", parse: func(pages []pdfextractor.Page) ([]normalizedRecord, error) {
		labels, err := flipkart.Parse(pages)
		if err != nil {
			return nil, err
		}
		out := make([]normalizedRecord, 0, len(labels))
		for _, label := range labels {
			out = append(out, normalizedRecord{Page: label.Page, AWB: label.AWB, OrderID: label.OrderID, SKU: label.SKU, Quantity: label.Quantity})
		}
		return out, nil
	}}
}
func amazonProcessor() processor {
	return processor{marketplace: "amazon", parserVersion: amazon.ParserVersion, documentName: "Amazon", auditCountKey: "documents", requireOrderID: true, parse: func(pages []pdfextractor.Page) ([]normalizedRecord, error) {
		documents, err := amazon.Parse(pages)
		if err != nil {
			return nil, err
		}
		out := make([]normalizedRecord, 0, len(documents))
		for _, document := range documents {
			record := normalizedRecord{Page: document.Page, AWB: document.AWB, OrderID: document.OrderID, SKU: document.SKU, Quantity: document.Quantity, Warnings: document.Warnings, AssociationMethod: document.AssociationMethod, Confidence: document.Confidence}
			for _, source := range document.Sources {
				record.Documents = append(record.Documents, normalizedDocument{Page: source.Page, Role: source.Role, ExtractionMethod: source.ExtractionMethod})
			}
			out = append(out, record)
		}
		return out, nil
	}}
}
func meeshoProcessor() processor {
	return processor{marketplace: "meesho", parserVersion: meesho.ParserVersion, documentName: "Meesho", auditCountKey: "labels", requireOrderID: true, parse: func(pages []pdfextractor.Page) ([]normalizedRecord, error) {
		labels, err := meesho.Parse(pages)
		if err != nil {
			return nil, err
		}
		out := make([]normalizedRecord, 0, len(labels))
		for _, label := range labels {
			out = append(out, normalizedRecord{Page: label.Page, AWB: label.AWB, OrderID: label.OrderID, SKU: label.SKU, Quantity: label.Quantity, Documents: []normalizedDocument{{Page: label.Page, Role: "shipping_label", ExtractionMethod: label.ExtractionMethod}}, AssociationMethod: "single_document", Confidence: "high"})
		}
		return out, nil
	}}
}
func myntraProcessor() processor {
	return processor{marketplace: "myntra", parserVersion: myntra.ParserVersion, documentName: "Myntra CSV", auditCountKey: "rows", requireOrderID: true, contentType: "text/csv", extension: ".csv", requireIdempotency: true, parseData: func(data []byte) ([]normalizedRecord, error) {
		records, err := myntra.Parse(data)
		if err != nil {
			return nil, err
		}
		out := make([]normalizedRecord, 0, len(records))
		for _, record := range records {
			out = append(out, normalizedRecord{Page: record.Row, AWB: record.TrackingID, OrderID: record.OrderID, SKU: record.SellerSKU, Warnings: record.Warnings, AssociationMethod: "csv_row", Confidence: "high", Metadata: map[string]any{
				"source_kind": "csv", "source_row": record.Row, "myntra_sku_code": record.MyntraSKU, "store_packet_id": record.StorePacketID, "order_release_id": record.OrderReleaseID, "marketplace_status": record.Status, "packed_on": record.PackedOn, "created_on": record.CreatedOn,
			}})
		}
		return out, nil
	}}
}
func snapdealProcessor() processor {
	return processor{marketplace: "snapdeal", parserVersion: snapdeal.ParserVersion, documentName: "Snapdeal", auditCountKey: "documents", requireOrderID: true, allowMissingAWB: true, parse: func(pages []pdfextractor.Page) ([]normalizedRecord, error) {
		documents, err := snapdeal.Parse(pages)
		if err != nil {
			return nil, err
		}
		out := make([]normalizedRecord, 0, len(documents))
		for _, d := range documents {
			record := normalizedRecord{Page: d.Page, AWB: d.AWB, OrderID: d.OrderID, SKU: d.SKU, Quantity: d.Quantity, Warnings: d.Warnings, AssociationMethod: d.AssociationMethod, Confidence: d.Confidence, Metadata: map[string]any{"compact_shipping_sku": d.CompactSKU}}
			for _, source := range d.Sources {
				record.Documents = append(record.Documents, normalizedDocument{Page: source.Page, Role: source.Role, ExtractionMethod: source.ExtractionMethod})
			}
			out = append(out, record)
		}
		return out, nil
	}}
}
func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) Upload(ctx context.Context, p auth.Principal, filename string, data []byte) (UploadResult, error) {
	return s.UploadWithIdempotency(ctx, p, filename, data, "")
}
func (s *Service) UploadWithIdempotency(ctx context.Context, p auth.Principal, filename string, data []byte, idempotencyKey string) (UploadResult, error) {
	if err := s.authorizer.RequireModule(ctx, p, s.processor.marketplace); err != nil {
		return UploadResult{}, err
	}
	if err := s.authorizer.RequirePermission(ctx, p, "labels.upload"); err != nil {
		return UploadResult{}, err
	}
	if err := s.authorizer.RequirePermission(ctx, p, "labels.process"); err != nil {
		return UploadResult{}, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if s.processor.requireIdempotency && (idempotencyKey == "" || len(idempotencyKey) > 128) {
		return UploadResult{}, ErrInvalidFile
	}
	isPDF := s.processor.parseData == nil
	if len(data) == 0 || len(data) > MaxUploadBytes || (isPDF && !bytes.HasPrefix(data, []byte("%PDF-"))) || (!isPDF && !strings.EqualFold(filepath.Ext(filename), s.processor.extension)) {
		return UploadResult{}, ErrInvalidFile
	}
	hashBytes := sha256.Sum256(data)
	hash := hex.EncodeToString(hashBytes[:])
	if idempotencyKey != "" {
		var existing Job
		var existingHash string
		err := s.db.QueryRow(ctx, `SELECT j.id,j.status,j.parser_version,j.total_pages,j.processed_pages,j.created_at,j.updated_at,f.sha256 FROM processing_jobs j JOIN source_files f ON f.company_id=j.company_id AND f.id=j.source_file_id WHERE j.company_id=$1 AND j.marketplace_key=$2 AND j.upload_idempotency_key=$3`, p.CompanyID, s.processor.marketplace, idempotencyKey).Scan(&existing.ID, &existing.Status, &existing.ParserVersion, &existing.TotalPages, &existing.ProcessedPages, &existing.CreatedAt, &existing.UpdatedAt, &existingHash)
		if err == nil {
			if existingHash != hash {
				return UploadResult{}, ErrIdempotencyConflict
			}
			return UploadResult{Job: existing, DuplicateSource: true}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return UploadResult{}, err
		}
	}
	if existing, err := s.findDuplicate(ctx, p.CompanyID, hash); err == nil {
		return UploadResult{Job: existing, DuplicateSource: true}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return UploadResult{}, err
	}
	sourceID, err := randomUUID()
	if err != nil {
		return UploadResult{}, err
	}
	extension, contentType := ".pdf", "application/pdf"
	if !isPDF {
		extension, contentType = s.processor.extension, s.processor.contentType
	}
	storageKey := path.Join(p.CompanyID, sourceID+extension)
	if err = s.storage.Put(ctx, storageKey, bytes.NewReader(data), int64(len(data)), contentType); err != nil {
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
	err = tx.QueryRow(ctx, `INSERT INTO source_files(id,company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(company_id,marketplace_key,sha256) DO NOTHING RETURNING id`, sourceID, p.CompanyID, s.processor.marketplace, storageKey, safeFilename(filename), contentType, len(data), hash, p.UserID).Scan(&inserted)
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
	uploadRequestHash := ""
	if idempotencyKey != "" {
		uploadRequestHash = hash
	}
	err = tx.QueryRow(ctx, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,upload_idempotency_key,upload_request_hash) VALUES($1,$2,$3,'queued',$4,NULLIF($5,''),NULLIF($6,'')) RETURNING id,status,parser_version,total_pages,processed_pages,created_at,updated_at`, p.CompanyID, sourceID, s.processor.marketplace, s.processor.parserVersion, idempotencyKey, uploadRequestHash).Scan(&job.ID, &job.Status, &job.ParserVersion, &job.TotalPages, &job.ProcessedPages, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return UploadResult{}, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, s.processor.marketplace+".file_uploaded", "processing_job", job.ID, map[string]any{"source_file_id": sourceID, "sha256": hash, "size_bytes": len(data)}); err != nil {
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
	err := s.db.QueryRow(ctx, `SELECT j.id,j.status,j.parser_version,j.total_pages,j.processed_pages,j.created_at,j.updated_at FROM source_files f JOIN processing_jobs j ON j.company_id=f.company_id AND j.source_file_id=f.id WHERE f.company_id=$1 AND f.marketplace_key=$2 AND f.sha256=$3 AND j.marketplace_key=$2 ORDER BY j.created_at DESC LIMIT 1`, companyID, s.processor.marketplace, hash).Scan(&job.ID, &job.Status, &job.ParserVersion, &job.TotalPages, &job.ProcessedPages, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return job, err
}

func (s *Service) Get(ctx context.Context, p auth.Principal, id string) (JobDetails, error) {
	if err := s.authorizer.RequireModule(ctx, p, s.processor.marketplace); err != nil {
		return JobDetails{}, err
	}
	if err := s.authorizer.RequirePermission(ctx, p, "labels.process"); err != nil {
		return JobDetails{}, err
	}
	var out JobDetails
	err := s.db.QueryRow(ctx, `SELECT id,status,parser_version,total_pages,processed_pages,created_at,updated_at FROM processing_jobs WHERE company_id=$1 AND id=$2 AND marketplace_key=$3`, p.CompanyID, id, s.processor.marketplace).Scan(&out.Job.ID, &out.Job.Status, &out.Job.ParserVersion, &out.Job.TotalPages, &out.Job.ProcessedPages, &out.Job.CreatedAt, &out.Job.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobDetails{}, ErrNotFound
	}
	if err != nil {
		return JobDetails{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,source_page,marketplace_order_id,awb,status,extraction_metadata FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2 AND marketplace_key=$3 ORDER BY source_page,id`, p.CompanyID, id, s.processor.marketplace)
	if err != nil {
		return JobDetails{}, err
	}
	defer rows.Close()
	out.Orders = []Order{}
	for rows.Next() {
		var o Order
		if err = rows.Scan(&o.ID, &o.SourcePage, &o.MarketplaceOrderID, &o.AWB, &o.Status, &o.Metadata); err != nil {
			return JobDetails{}, err
		}
		ir, queryErr := s.db.Query(ctx, `SELECT raw_sku,product_id,quantity,quantity_source,resolution_status,warnings FROM marketplace_order_items WHERE company_id=$1 AND order_id=$2`, p.CompanyID, o.ID)
		if queryErr != nil {
			return JobDetails{}, queryErr
		}
		o.Items = []Item{}
		o.Documents = []Document{}
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
		dr, queryErr := s.db.Query(ctx, `SELECT source_page,document_role,extraction_method FROM marketplace_order_documents WHERE company_id=$1 AND order_id=$2 ORDER BY source_page,document_role`, p.CompanyID, o.ID)
		if queryErr != nil {
			return JobDetails{}, queryErr
		}
		for dr.Next() {
			var document Document
			if queryErr = dr.Scan(&document.SourcePage, &document.Role, &document.ExtractionMethod); queryErr != nil {
				dr.Close()
				return JobDetails{}, queryErr
			}
			o.Documents = append(o.Documents, document)
		}
		if queryErr = dr.Err(); queryErr != nil {
			dr.Close()
			return JobDetails{}, queryErr
		}
		dr.Close()
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
	if err := s.authorizer.RequireModule(ctx, p, s.processor.marketplace); err != nil {
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
	err = tx.QueryRow(ctx, `SELECT status FROM processing_jobs WHERE company_id=$1 AND id=$2 AND marketplace_key=$3 FOR UPDATE`, p.CompanyID, id, s.processor.marketplace).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	if status == "queued" || status == "processing" {
		return Job{}, ErrJobActive
	}
	if _, err = tx.Exec(ctx, `DELETE FROM marketplace_order_documents WHERE company_id=$1 AND order_id IN(SELECT id FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2 AND marketplace_key=$3)`, p.CompanyID, id, s.processor.marketplace); err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM marketplace_order_items WHERE company_id=$1 AND order_id IN(SELECT id FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2 AND marketplace_key=$3)`, p.CompanyID, id, s.processor.marketplace); err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2 AND marketplace_key=$3`, p.CompanyID, id, s.processor.marketplace); err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM processing_errors WHERE company_id=$1 AND processing_job_id=$2`, p.CompanyID, id); err != nil {
		return Job{}, err
	}
	var job Job
	err = tx.QueryRow(ctx, `UPDATE processing_jobs SET status='queued',total_pages=0,processed_pages=0,started_at=NULL,completed_at=NULL,worker_id=NULL,lease_expires_at=NULL,updated_at=now(),parser_version=$1 WHERE company_id=$2 AND id=$3 AND marketplace_key=$4 RETURNING id,status,parser_version,total_pages,processed_pages,created_at,updated_at`, s.processor.parserVersion, p.CompanyID, id, s.processor.marketplace).Scan(&job.ID, &job.Status, &job.ParserVersion, &job.TotalPages, &job.ProcessedPages, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return Job{}, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, s.processor.marketplace+".processing_retried", "processing_job", id, map[string]any{"previous_status": status}); err != nil {
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
	var eligible bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM processing_jobs WHERE marketplace_key=$1 AND (status='queued' OR (status='processing' AND (lease_expires_at IS NULL OR lease_expires_at<=now()))))`, s.processor.marketplace).Scan(&eligible)
	if err == nil && eligible {
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
	err = tx.QueryRow(ctx, `SELECT j.company_id,f.uploaded_by,j.id,f.storage_key FROM processing_jobs j JOIN source_files f ON f.company_id=j.company_id AND f.id=j.source_file_id AND f.marketplace_key=$1 WHERE j.marketplace_key=$1 AND (j.status='queued' OR (j.status='processing' AND (j.lease_expires_at IS NULL OR j.lease_expires_at<=now()))) ORDER BY j.created_at,j.id FOR UPDATE OF j SKIP LOCKED LIMIT 1`, s.processor.marketplace).Scan(&item.CompanyID, &item.UserID, &item.JobID, &item.StorageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return work{}, false, nil
	}
	if err != nil {
		return work{}, false, err
	}
	result, err := tx.Exec(ctx, `UPDATE processing_jobs SET status='processing',worker_id=$3,lease_expires_at=now()+($4*interval '1 second'),started_at=COALESCE(started_at,now()),completed_at=NULL,updated_at=now() WHERE company_id=$1 AND id=$2 AND marketplace_key=$5 AND (status='queued' OR (status='processing' AND (lease_expires_at IS NULL OR lease_expires_at<=now())))`, item.CompanyID, item.JobID, s.workerID, int64(s.leaseDuration/time.Second), s.processor.marketplace)
	if err != nil {
		return work{}, false, err
	}
	if result.RowsAffected() != 1 {
		return work{}, false, nil
	}
	if err = tx.Commit(ctx); err != nil {
		return work{}, false, err
	}
	item.WorkerID = s.workerID
	return item, true, nil
}
func (s *Service) processNext() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	item, claimed, err := s.claim(ctx)
	if err != nil || !claimed {
		return claimed, err
	}
	if err = s.executeWithHeartbeat(ctx, item); err != nil {
		if errors.Is(err, ErrLeaseLost) {
			return true, nil
		}
		failureCtx, failureCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer failureCancel()
		code, message := s.classifyWorkerError(err)
		if failErr := s.failJob(failureCtx, item, code, message); failErr != nil {
			if errors.Is(failErr, ErrLeaseLost) {
				return true, nil
			}
			return true, fmt.Errorf("process error: %v; persist failure: %w", err, failErr)
		}
	}
	return true, nil
}
func (s *Service) executeWithHeartbeat(parent context.Context, item work) error {
	ctx, cancel := context.WithCancel(parent)
	heartbeatDone := make(chan error, 1)
	go func() {
		heartbeatDone <- s.heartbeat(ctx, cancel, item)
	}()
	executeErr := s.execute(ctx, item)
	cancel()
	heartbeatErr := <-heartbeatDone
	if executeErr == nil {
		return nil
	}
	if heartbeatErr != nil {
		return heartbeatErr
	}
	return executeErr
}
func (s *Service) heartbeat(ctx context.Context, cancel context.CancelFunc, item work) error {
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.renewLease(ctx, item); err != nil {
				cancel()
				return err
			}
		}
	}
}
func (s *Service) renewLease(ctx context.Context, item work) error {
	result, err := s.db.Exec(ctx, `UPDATE processing_jobs SET lease_expires_at=now()+($4*interval '1 second'),updated_at=now() WHERE company_id=$1 AND id=$2 AND marketplace_key=$5 AND status='processing' AND worker_id=$3 AND lease_expires_at>now()`, item.CompanyID, item.JobID, item.WorkerID, int64(s.leaseDuration/time.Second), s.processor.marketplace)
	if err != nil {
		return fmt.Errorf("renew processing lease: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
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
	var labels []normalizedRecord
	totalUnits := 0
	if s.processor.parseData != nil {
		labels, err = s.processor.parseData(data)
		totalUnits = len(labels)
	} else {
		var pages []pdfextractor.Page
		pages, err = s.extractor.Extract(ctx, data)
		if err != nil {
			return fmt.Errorf("PDF extraction failed: %w", err)
		}
		totalUnits = len(pages)
		labels, err = s.processor.parse(pages)
	}
	if err != nil {
		return fmt.Errorf("%s parse failed: %w", s.processor.documentName, err)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var sourceID string
	if err = tx.QueryRow(ctx, `SELECT source_file_id FROM processing_jobs WHERE company_id=$1 AND id=$2 AND marketplace_key=$4 AND status='processing' AND worker_id=$3 AND lease_expires_at>now() FOR UPDATE`, w.CompanyID, w.JobID, w.WorkerID, s.processor.marketplace).Scan(&sourceID); errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	} else if err != nil {
		return err
	}
	if err = s.audit.Record(ctx, tx, w.CompanyID, w.UserID, s.processor.marketplace+".processing_started", "processing_job", w.JobID, map[string]any{"parser_version": s.processor.parserVersion}); err != nil {
		return err
	}
	needsReview := false
	for _, label := range labels {
		status := "resolved"
		warnings := append([]string{}, label.Warnings...)
		if len(label.Warnings) > 0 {
			status = "needs_review"
		}
		var productID *string
		if label.AWB == "" {
			if !s.processor.allowMissingAWB {
				status = "needs_review"
			}
			warnings = append(warnings, "missing_awb")
		}
		if label.OrderID == "" {
			if s.processor.requireOrderID {
				status = "needs_review"
			}
			warnings = append(warnings, "missing_order_id")
		}
		if label.SKU == "" {
			status = "needs_review"
			warnings = append(warnings, "missing_sku")
		} else {
			var id string
			resolutionErr := tx.QueryRow(ctx, `SELECT m.product_id FROM sku_mappings m JOIN products p ON p.company_id=m.company_id AND p.id=m.product_id WHERE m.company_id=$1 AND m.marketplace_key=$2 AND m.sku=$3 AND m.status='active' AND p.status='active'`, w.CompanyID, s.processor.marketplace, label.SKU).Scan(&id)
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
				if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, w.CompanyID+"|"+s.processor.marketplace+"|"+identifier); err != nil {
					return err
				}
			}
		}
		var duplicate bool
		if label.AWB != "" || label.OrderID != "" {
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM marketplace_orders WHERE company_id=$1 AND marketplace_key=$2 AND status<>'duplicate' AND (($3<>'' AND awb=$3) OR ($4<>'' AND marketplace_order_id=$4)))`, w.CompanyID, s.processor.marketplace, label.AWB, label.OrderID).Scan(&duplicate); err != nil {
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
		metadataValues := map[string]any{"extractor": "poppler", "association_method": label.AssociationMethod, "association_confidence": label.Confidence, "fields_detected": map[string]bool{"awb": label.AWB != "", "order_id": label.OrderID != "", "sku": label.SKU != "", "quantity": label.Quantity != nil}}
		for key, value := range label.Metadata {
			metadataValues[key] = value
		}
		if s.processor.parseData != nil {
			metadataValues["extractor"] = "csv"
		}
		metadata, _ := json.Marshal(metadataValues)
		var orderID string
		if err = tx.QueryRow(ctx, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,awb,status,parser_version,extraction_metadata) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),$8,$9,$10) RETURNING id`, w.CompanyID, s.processor.marketplace, sourceID, w.JobID, label.Page, label.OrderID, label.AWB, status, s.processor.parserVersion, metadata).Scan(&orderID); err != nil {
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
		for _, document := range label.Documents {
			if _, err = tx.Exec(ctx, `INSERT INTO marketplace_order_documents(company_id,order_id,source_file_id,source_page,document_role,extraction_method) VALUES($1,$2,$3,$4,$5,$6)`, w.CompanyID, orderID, sourceID, document.Page, document.Role, document.ExtractionMethod); err != nil {
				return err
			}
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
	result, err := tx.Exec(ctx, `UPDATE processing_jobs SET status=$1,total_pages=$2,processed_pages=$2,completed_at=now(),worker_id=NULL,lease_expires_at=NULL,updated_at=now() WHERE company_id=$3 AND id=$4 AND marketplace_key=$6 AND status='processing' AND worker_id=$5`, finalStatus, totalUnits, w.CompanyID, w.JobID, w.WorkerID, s.processor.marketplace)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	if err = s.audit.Record(ctx, tx, w.CompanyID, w.UserID, s.processor.marketplace+".processing_completed", "processing_job", w.JobID, map[string]any{"status": finalStatus, "source_units": totalUnits, s.processor.auditCountKey: len(labels)}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("processing commit failed: %w", err)
	}
	return nil
}
func (s *Service) failJob(ctx context.Context, w work, code, message string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var owned bool
	if err = tx.QueryRow(ctx, `SELECT true FROM processing_jobs WHERE company_id=$1 AND id=$2 AND marketplace_key=$4 AND status='processing' AND worker_id=$3 AND lease_expires_at>now() FOR UPDATE`, w.CompanyID, w.JobID, w.WorkerID, s.processor.marketplace).Scan(&owned); errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	} else if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO processing_errors(company_id,processing_job_id,severity,code,message) VALUES($1,$2,'error',$3,$4)`, w.CompanyID, w.JobID, code, message); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE processing_jobs SET status='failed',completed_at=now(),worker_id=NULL,lease_expires_at=NULL,updated_at=now() WHERE company_id=$1 AND id=$2 AND marketplace_key=$4 AND status='processing' AND worker_id=$3`, w.CompanyID, w.JobID, w.WorkerID, s.processor.marketplace)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
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
	raw := hex.EncodeToString(value)
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32], nil
}
func (s *Service) classifyWorkerError(err error) (string, string) {
	value := err.Error()
	switch {
	case strings.Contains(value, "storage"):
		return "STORAGE_READ_FAILED", "Stored source file could not be read"
	case strings.Contains(value, "PDF extraction"):
		return "PDF_EXTRACTION_FAILED", "PDF text extraction failed or exceeded a resource limit"
	case strings.Contains(value, s.processor.documentName+" parse"):
		return "UNSUPPORTED_" + strings.ToUpper(s.processor.marketplace) + "_DOCUMENT", "No supported " + s.processor.documentName + " document could be extracted"
	case strings.Contains(value, "commit"):
		return "PROCESSING_COMMIT_FAILED", "Processing results could not be committed"
	default:
		return "PROCESSING_DATABASE_FAILED", "Processing could not be completed"
	}
}
