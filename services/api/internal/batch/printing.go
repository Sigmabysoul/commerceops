// This file connects batch workflows to printing without moving print policy into HTTP handlers in the batch package.
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

var ErrGenerationFailed = errors.New("print generation failed")

type GenerateInput struct {
	SortLabels     bool   `json:"sort_labels"`
	ExportInvoices bool   `json:"export_invoices"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ReprintInput struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

type PrintJob struct {
	ID                string     `json:"id"`
	BatchID           string     `json:"batch_id"`
	Status            string     `json:"status"`
	SortLabels        bool       `json:"sort_labels"`
	ExportInvoices    bool       `json:"export_invoices"`
	GenerationVersion string     `json:"generation_version"`
	SourcePrintJobID  *string    `json:"source_print_job_id"`
	ReprintReason     *string    `json:"reprint_reason"`
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
	MarketplaceKey, SKU                            string
	SourcePage, InvoicePage, Quantity              int
}

func (s *Service) Generate(ctx context.Context, principal auth.Principal, batchID string, input GenerateInput) (PrintJob, bool, error) {
	if s.storage == nil || len(s.generators) == 0 {
		return PrintJob{}, false, ErrGenerationFailed
	}
	if err := s.authorizePrint(ctx, principal); err != nil {
		return PrintJob{}, false, err
	}
	return s.generate(ctx, principal, batchID, input, nil, nil)
}

func (s *Service) Reprint(ctx context.Context, principal auth.Principal, sourceID string, input ReprintInput) (PrintJob, bool, error) {
	if s.storage == nil || len(s.generators) == 0 {
		return PrintJob{}, false, ErrGenerationFailed
	}
	if err := s.authorizeReprint(ctx, principal); err != nil {
		return PrintJob{}, false, err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if !uuidRE.MatchString(sourceID) || input.Reason == "" || len(input.Reason) > 500 || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 {
		return PrintJob{}, false, ErrInvalidInput
	}
	source, err := s.getPrintJob(ctx, principal.CompanyID, sourceID)
	if err != nil {
		return PrintJob{}, false, err
	}
	if source.Status != "ready" {
		return PrintJob{}, false, ErrInvalidState
	}
	generateInput := GenerateInput{SortLabels: source.SortLabels, ExportInvoices: source.ExportInvoices, IdempotencyKey: input.IdempotencyKey}
	return s.generate(ctx, principal, source.BatchID, generateInput, &sourceID, &input.Reason)
}

func (s *Service) generate(ctx context.Context, principal auth.Principal, batchID string, input GenerateInput, sourceID, reprintReason *string) (PrintJob, bool, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if !uuidRE.MatchString(batchID) || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 {
		return PrintJob{}, false, ErrInvalidInput
	}
	marketplace, err := s.requireBatchModule(ctx, principal, batchID)
	if err != nil {
		return PrintJob{}, false, err
	}
	generator, ok := s.generators[marketplace]
	if !ok || generator.generator == nil {
		return PrintJob{}, false, ErrGenerationFailed
	}
	hash, _ := json.Marshal(struct {
		Input    GenerateInput `json:"input"`
		SourceID *string       `json:"source_print_job_id"`
		Reason   *string       `json:"reprint_reason"`
	}{input, sourceID, reprintReason})
	digest := sha256.Sum256(append([]byte(batchID+"|"), hash...))
	requestHash := hex.EncodeToString(digest[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PrintJob{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var jobID string
	err = tx.QueryRow(ctx, `INSERT INTO print_jobs(company_id,batch_id,requested_by,sort_labels,export_invoices,generation_version,idempotency_key,request_hash,source_print_job_id,reprint_reason) SELECT $1,id,$2,$3,$4,$5,$6,$7,$9,$10 FROM batches WHERE company_id=$1 AND id=$8 AND status='ready' ON CONFLICT(company_id,idempotency_key) DO NOTHING RETURNING id`, principal.CompanyID, principal.UserID, input.SortLabels, input.ExportInvoices, generator.version, input.IdempotencyKey, requestHash, batchID, sourceID, reprintReason).Scan(&jobID)
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
		job, getErr := s.getPrintJob(ctx, principal.CompanyID, jobID)
		return job, true, getErr
	}
	if err != nil {
		return PrintJob{}, false, err
	}
	rows, err := tx.Query(ctx, `SELECT mo.id,mo.source_file_id,mo.processing_job_id,b.marketplace_key,CASE WHEN b.marketplace_key='amazon' THEN COALESCE(label.source_page,0) ELSE mo.source_page END,COALESCE(invoice.source_page,0),sf.storage_key,COALESCE(moi.raw_sku,''),COALESCE(moi.quantity,0) FROM batch_members bm JOIN batches b ON b.company_id=bm.company_id AND b.id=bm.batch_id JOIN marketplace_orders mo ON mo.company_id=bm.company_id AND mo.id=bm.marketplace_order_id JOIN source_files sf ON sf.company_id=mo.company_id AND sf.id=mo.source_file_id JOIN marketplace_order_items moi ON moi.company_id=mo.company_id AND moi.order_id=mo.id JOIN products p ON p.company_id=moi.company_id AND p.id=moi.product_id LEFT JOIN marketplace_order_documents label ON label.company_id=mo.company_id AND label.order_id=mo.id AND label.document_role='shipping_label' LEFT JOIN marketplace_order_documents invoice ON invoice.company_id=mo.company_id AND invoice.order_id=mo.id AND invoice.document_role='invoice' WHERE bm.company_id=$1 AND bm.batch_id=$2 ORDER BY CASE WHEN $3 THEN p.internal_code END,CASE WHEN $3 THEN moi.raw_sku END,CASE WHEN $3 THEN COALESCE(mo.marketplace_order_id,'') END,bm.position`, principal.CompanyID, batchID, input.SortLabels)
	if err != nil {
		return PrintJob{}, false, err
	}
	pages := make([]printPage, 0)
	for rows.Next() {
		var page printPage
		if err = rows.Scan(&page.OrderID, &page.SourceID, &page.ProcessingJobID, &page.MarketplaceKey, &page.SourcePage, &page.InvoicePage, &page.StorageKey, &page.SKU, &page.Quantity); err != nil {
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
	action := "print.requested"
	metadata := map[string]any{"batch_id": batchID, "marketplace": marketplace, "sort_labels": input.SortLabels, "export_invoices": input.ExportInvoices, "page_count": len(pages), "generation_version": generator.version}
	if sourceID != nil {
		action = "print.reprinted"
		metadata["source_print_job_id"] = *sourceID
		metadata["reason"] = *reprintReason
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, action, "print_job", jobID, metadata); err != nil {
		return PrintJob{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PrintJob{}, false, err
	}

	generatedPages, err := s.loadPages(ctx, pages)
	if err == nil {
		var result pdfgenerator.Result
		result, err = generator.generator.Generate(ctx, generatedPages, input.ExportInvoices)
		if err == nil {
			err = s.persistArtifacts(ctx, principal, jobID, len(pages), input.ExportInvoices, result)
		}
	}
	if err != nil {
		s.failPrintJob(ctx, principal, jobID, err)
		return PrintJob{}, false, ErrGenerationFailed
	}
	job, err := s.getPrintJob(ctx, principal.CompanyID, jobID)
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
		result = append(result, pdfgenerator.Page{SourceID: page.SourceID, PDF: pdf, Number: page.SourcePage, InvoiceNumber: page.InvoicePage, SKU: page.SKU, Quantity: page.Quantity})
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
	return s.authorizer.RequirePermission(ctx, principal, "labels.print")
}

func (s *Service) authorizeReprint(ctx context.Context, principal auth.Principal) error {
	return s.authorizer.RequirePermission(ctx, principal, "labels.reprint")
}

func (s *Service) requireBatchModule(ctx context.Context, principal auth.Principal, batchID string) (string, error) {
	var marketplace string
	if err := s.db.QueryRow(ctx, `SELECT marketplace_key FROM batches WHERE company_id=$1 AND id=$2`, principal.CompanyID, batchID).Scan(&marketplace); err != nil {
		return "", mapDBError(err)
	}
	if err := s.authorizer.RequireModule(ctx, principal, marketplace); err != nil {
		return "", err
	}
	return marketplace, nil
}

func (s *Service) GetPrintJob(ctx context.Context, principal auth.Principal, id string) (PrintJob, error) {
	if err := s.authorizePrint(ctx, principal); err != nil {
		return PrintJob{}, err
	}
	job, err := s.getPrintJob(ctx, principal.CompanyID, id)
	if err != nil {
		return PrintJob{}, err
	}
	if _, err = s.requireBatchModule(ctx, principal, job.BatchID); err != nil {
		return PrintJob{}, err
	}
	return job, nil
}

func (s *Service) getPrintJob(ctx context.Context, companyID, id string) (PrintJob, error) {
	var job PrintJob
	err := s.db.QueryRow(ctx, `SELECT id,batch_id,status,sort_labels,export_invoices,generation_version,source_print_job_id,reprint_reason,error_code,error_message,completed_at,created_at FROM print_jobs WHERE company_id=$1 AND id=$2`, companyID, id).Scan(&job.ID, &job.BatchID, &job.Status, &job.SortLabels, &job.ExportInvoices, &job.GenerationVersion, &job.SourcePrintJobID, &job.ReprintReason, &job.ErrorCode, &job.ErrorMessage, &job.CompletedAt, &job.CreatedAt)
	if err != nil {
		return PrintJob{}, mapDBError(err)
	}
	rows, err := s.db.Query(ctx, `SELECT id,kind,size_bytes,sha256,page_count FROM print_artifacts WHERE company_id=$1 AND print_job_id=$2 ORDER BY kind`, companyID, id)
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

func (s *Service) ListPrintJobs(ctx context.Context, principal auth.Principal, batchID string) ([]PrintJob, error) {
	if err := s.authorizePrint(ctx, principal); err != nil {
		return nil, err
	}
	if !uuidRE.MatchString(batchID) {
		return nil, ErrInvalidInput
	}
	if _, err := s.requireBatchModule(ctx, principal, batchID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM print_jobs WHERE company_id=$1 AND batch_id=$2 ORDER BY created_at DESC,id DESC LIMIT 200`, principal.CompanyID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	items := make([]PrintJob, 0, len(ids))
	for _, id := range ids {
		item, getErr := s.getPrintJob(ctx, principal.CompanyID, id)
		if getErr != nil {
			return nil, getErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) DownloadArtifact(ctx context.Context, principal auth.Principal, id string) ([]byte, string, error) {
	if err := s.authorizePrint(ctx, principal); err != nil {
		return nil, "", err
	}
	var key, kind, batchID string
	var size int64
	if err := s.db.QueryRow(ctx, `SELECT pa.storage_key,pa.kind,pa.size_bytes,pj.batch_id FROM print_artifacts pa JOIN print_jobs pj ON pj.company_id=pa.company_id AND pj.id=pa.print_job_id WHERE pa.company_id=$1 AND pa.id=$2`, principal.CompanyID, id).Scan(&key, &kind, &size, &batchID); err != nil {
		return nil, "", mapDBError(err)
	}
	if _, err := s.requireBatchModule(ctx, principal, batchID); err != nil {
		return nil, "", err
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
