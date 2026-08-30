package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfgenerator"
	"github.com/jackc/pgx/v5"
)

const generationVersion = "flipkart-a4-v1"

var ErrGenerationFailed = errors.New("print generation failed")

type GenerateInput struct {
	SortLabels     bool   `json:"sort_labels"`
	ExportInvoices bool   `json:"export_invoices"`
	IdempotencyKey string `json:"idempotency_key"`
}

type PrintJob struct {
	ID                string     `json:"id"`
	BatchID           string     `json:"batch_id"`
	Status            string     `json:"status"`
	SortLabels        bool       `json:"sort_labels"`
	ExportInvoices    bool       `json:"export_invoices"`
	GenerationVersion string     `json:"generation_version"`
	ErrorCode         *string    `json:"error_code"`
	ErrorMessage      *string    `json:"error_message"`
	CompletedAt       *time.Time `json:"completed_at"`
	CreatedAt         time.Time  `json:"created_at"`
	Artifacts         []Artifact `json:"artifacts"`
}

type Artifact struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	PageCount int    `json:"page_count"`
}

type printPage struct {
	OrderID, SourceID, ProcessingJobID, StorageKey string
	SourcePage                                     int
}

func (s *Service) Generate(ctx context.Context, principal auth.Principal, batchID string, input GenerateInput) (PrintJob, bool, error) {
	if s.storage == nil || s.generator == nil {
		return PrintJob{}, false, ErrGenerationFailed
	}
	if err := s.authorizePrint(ctx, principal); err != nil {
		return PrintJob{}, false, err
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if !uuidRE.MatchString(batchID) || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 {
		return PrintJob{}, false, ErrInvalidInput
	}
	hash, _ := json.Marshal(input)
	digest := sha256.Sum256(append([]byte(batchID+"|"), hash...))
	requestHash := hex.EncodeToString(digest[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PrintJob{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var jobID string
	err = tx.QueryRow(ctx, `INSERT INTO print_jobs(company_id,batch_id,requested_by,sort_labels,export_invoices,generation_version,idempotency_key,request_hash) SELECT $1,id,$2,$3,$4,$5,$6,$7 FROM batches WHERE company_id=$1 AND id=$8 AND status='ready' ON CONFLICT(company_id,idempotency_key) DO NOTHING RETURNING id`, principal.CompanyID, principal.UserID, input.SortLabels, input.ExportInvoices, generationVersion, input.IdempotencyKey, requestHash, batchID).Scan(&jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingHash string
		if err = tx.QueryRow(ctx, `SELECT id,request_hash FROM print_jobs WHERE company_id=$1 AND idempotency_key=$2`, principal.CompanyID, input.IdempotencyKey).Scan(&jobID, &existingHash); errors.Is(err, pgx.ErrNoRows) {
			return PrintJob{}, false, ErrInvalidState
		}
		if err != nil {
			return PrintJob{}, false, err
		}
		if existingHash != requestHash {
			return PrintJob{}, false, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return PrintJob{}, false, err
		}
		job, getErr := s.GetPrintJob(ctx, principal, jobID)
		return job, true, getErr
	}
	if err != nil {
		return PrintJob{}, false, err
	}
	rows, err := tx.Query(ctx, `SELECT mo.id,mo.source_file_id,mo.processing_job_id,mo.source_page,sf.storage_key FROM batch_members bm JOIN marketplace_orders mo ON mo.company_id=bm.company_id AND mo.id=bm.marketplace_order_id JOIN source_files sf ON sf.company_id=mo.company_id AND sf.id=mo.source_file_id JOIN marketplace_order_items moi ON moi.company_id=mo.company_id AND moi.order_id=mo.id JOIN products p ON p.company_id=moi.company_id AND p.id=moi.product_id WHERE bm.company_id=$1 AND bm.batch_id=$2 ORDER BY CASE WHEN $3 THEN p.internal_code END,CASE WHEN $3 THEN moi.raw_sku END,CASE WHEN $3 THEN COALESCE(mo.marketplace_order_id,'') END,bm.position`, principal.CompanyID, batchID, input.SortLabels)
	if err != nil {
		return PrintJob{}, false, err
	}
	pages := make([]printPage, 0)
	for rows.Next() {
		var page printPage
		if err = rows.Scan(&page.OrderID, &page.SourceID, &page.ProcessingJobID, &page.SourcePage, &page.StorageKey); err != nil {
			rows.Close()
			return PrintJob{}, false, err
		}
		pages = append(pages, page)
	}
	rows.Close()
	if err = rows.Err(); err != nil || len(pages) == 0 {
		return PrintJob{}, false, ErrGenerationFailed
	}
	for index, page := range pages {
		if _, err = tx.Exec(ctx, `INSERT INTO print_job_items(company_id,print_job_id,marketplace_order_id,source_file_id,processing_job_id,source_page,output_position) VALUES($1,$2,$3,$4,$5,$6,$7)`, principal.CompanyID, jobID, page.OrderID, page.SourceID, page.ProcessingJobID, page.SourcePage, index+1); err != nil {
			return PrintJob{}, false, err
		}
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "print.requested", "print_job", jobID, map[string]any{"batch_id": batchID, "sort_labels": input.SortLabels, "export_invoices": input.ExportInvoices, "page_count": len(pages), "generation_version": generationVersion}); err != nil {
		return PrintJob{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PrintJob{}, false, err
	}

	generatedPages, err := s.loadPages(ctx, pages)
	if err == nil {
		var result pdfgenerator.Result
		result, err = s.generator.Generate(ctx, generatedPages, input.ExportInvoices)
		if err == nil {
			err = s.persistArtifacts(ctx, principal, jobID, len(pages), input.ExportInvoices, result)
		}
	}
	if err != nil {
		s.failPrintJob(ctx, principal, jobID, err)
		return PrintJob{}, false, ErrGenerationFailed
	}
	job, err := s.GetPrintJob(ctx, principal, jobID)
	return job, false, err
}

func (s *Service) loadPages(ctx context.Context, pages []printPage) ([]pdfgenerator.Page, error) {
	data := make(map[string][]byte)
	result := make([]pdfgenerator.Page, 0, len(pages))
	for _, page := range pages {
		pdf, ok := data[page.SourceID]
		if !ok {
			object, err := s.storage.Get(ctx, page.StorageKey)
			if err != nil {
				return nil, err
			}
			pdf, err = io.ReadAll(io.LimitReader(object, (20<<20)+1))
			closeErr := object.Close()
			if err != nil || closeErr != nil || len(pdf) == 0 || len(pdf) > 20<<20 {
				return nil, ErrGenerationFailed
			}
			data[page.SourceID] = pdf
		}
		result = append(result, pdfgenerator.Page{SourceID: page.SourceID, PDF: pdf, Number: page.SourcePage})
	}
	return result, nil
}

func (s *Service) persistArtifacts(ctx context.Context, principal auth.Principal, jobID string, pages int, exportInvoices bool, result pdfgenerator.Result) error {
	type output struct {
		kind string
		data []byte
	}
	if len(result.Labels) == 0 || exportInvoices != (len(result.Invoices) > 0) {
		return ErrGenerationFailed
	}
	outputs := []output{{"labels", result.Labels}}
	if exportInvoices {
		outputs = append(outputs, output{"invoices", result.Invoices})
	}
	stored := make([]string, 0, len(outputs))
	committed := false
	defer func() {
		if !committed {
			for _, saved := range stored {
				_ = s.storage.Delete(ctx, saved)
			}
		}
	}()
	for _, item := range outputs {
		key := path.Join(principal.CompanyID, "print-jobs", jobID, item.kind+".pdf")
		if err := s.storage.Put(ctx, key, bytes.NewReader(item.data), int64(len(item.data)), "application/pdf"); err != nil {
			for _, saved := range stored {
				_ = s.storage.Delete(ctx, saved)
			}
			return err
		}
		stored = append(stored, key)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for index, item := range outputs {
		sum := sha256.Sum256(item.data)
		if _, err = tx.Exec(ctx, `INSERT INTO print_artifacts(company_id,print_job_id,kind,storage_key,size_bytes,sha256,page_count) VALUES($1,$2,$3,$4,$5,$6,$7)`, principal.CompanyID, jobID, item.kind, stored[index], len(item.data), hex.EncodeToString(sum[:]), pages); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE print_jobs SET status='ready',completed_at=now(),updated_at=now() WHERE company_id=$1 AND id=$2 AND status='generating'`, principal.CompanyID, jobID); err != nil {
		return err
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "print.generated", "print_job", jobID, map[string]any{"page_count": pages, "artifacts": len(outputs)}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err == nil {
		committed = true
	}
	return err
}

func (s *Service) failPrintJob(ctx context.Context, principal auth.Principal, jobID string, cause error) {
	message := fmt.Sprintf("%.500s", cause.Error())
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `UPDATE print_jobs SET status='failed',error_code='GENERATION_FAILED',error_message=$1,completed_at=now(),updated_at=now() WHERE company_id=$2 AND id=$3 AND status='generating'`, message, principal.CompanyID, jobID); err != nil {
		return
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "print.generation_failed", "print_job", jobID, map[string]any{"error_code": "GENERATION_FAILED"}); err != nil {
		return
	}
	_ = tx.Commit(ctx)
}

func (s *Service) authorizePrint(ctx context.Context, principal auth.Principal) error {
	if err := s.authorizer.RequireModule(ctx, principal, "flipkart"); err != nil {
		return err
	}
	return s.authorizer.RequirePermission(ctx, principal, "labels.print")
}

func (s *Service) GetPrintJob(ctx context.Context, principal auth.Principal, id string) (PrintJob, error) {
	if err := s.authorizePrint(ctx, principal); err != nil {
		return PrintJob{}, err
	}
	var job PrintJob
	err := s.db.QueryRow(ctx, `SELECT id,batch_id,status,sort_labels,export_invoices,generation_version,error_code,error_message,completed_at,created_at FROM print_jobs WHERE company_id=$1 AND id=$2`, principal.CompanyID, id).Scan(&job.ID, &job.BatchID, &job.Status, &job.SortLabels, &job.ExportInvoices, &job.GenerationVersion, &job.ErrorCode, &job.ErrorMessage, &job.CompletedAt, &job.CreatedAt)
	if err != nil {
		return PrintJob{}, mapDBError(err)
	}
	rows, err := s.db.Query(ctx, `SELECT id,kind,size_bytes,sha256,page_count FROM print_artifacts WHERE company_id=$1 AND print_job_id=$2 ORDER BY kind`, principal.CompanyID, id)
	if err != nil {
		return PrintJob{}, err
	}
	defer rows.Close()
	job.Artifacts = make([]Artifact, 0)
	for rows.Next() {
		var item Artifact
		if err := rows.Scan(&item.ID, &item.Kind, &item.SizeBytes, &item.SHA256, &item.PageCount); err != nil {
			return PrintJob{}, err
		}
		job.Artifacts = append(job.Artifacts, item)
	}
	return job, rows.Err()
}

func (s *Service) DownloadArtifact(ctx context.Context, principal auth.Principal, id string) ([]byte, string, error) {
	if err := s.authorizePrint(ctx, principal); err != nil {
		return nil, "", err
	}
	var key, kind string
	var size int64
	if err := s.db.QueryRow(ctx, `SELECT storage_key,kind,size_bytes FROM print_artifacts WHERE company_id=$1 AND id=$2`, principal.CompanyID, id).Scan(&key, &kind, &size); err != nil {
		return nil, "", mapDBError(err)
	}
	if size <= 0 || size > 100<<20 {
		return nil, "", ErrGenerationFailed
	}
	object, err := s.storage.Get(ctx, key)
	if err != nil {
		return nil, "", err
	}
	defer object.Close()
	data, err := io.ReadAll(io.LimitReader(object, size+1))
	if err != nil || int64(len(data)) != size {
		return nil, "", ErrGenerationFailed
	}
	return data, kind + ".pdf", nil
}
