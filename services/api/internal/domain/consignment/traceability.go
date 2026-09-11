// This file integrates verified Trace Box quantities with Consignment-owned progress and gates.
package consignment

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
)

type TraceLinkInput struct {
	LineID             string  `json:"line_id"`
	TraceBoxIdentifier string  `json:"trace_box_identifier"`
	Quantity           int64   `json:"quantity"`
	ExpectedVersion    int     `json:"expected_version"`
	Notes              *string `json:"notes"`
	IdempotencyKey     string  `json:"idempotency_key"`
}

type TraceUnlinkInput struct {
	ExpectedVersion int     `json:"expected_version"`
	Notes           *string `json:"notes"`
	IdempotencyKey  string  `json:"idempotency_key"`
}

type TraceEvidenceInput struct {
	EvidenceType       string  `json:"evidence_type"`
	ReferenceValue     string  `json:"reference_value"`
	TraceBoxIdentifier *string `json:"trace_box_identifier"`
	ExpectedVersion    int     `json:"expected_version"`
	Notes              *string `json:"notes"`
	IdempotencyKey     string  `json:"idempotency_key"`
}

type TraceLink struct {
	AllocationEventID string    `json:"allocation_event_id"`
	TraceBoxID        string    `json:"trace_box_id"`
	OpaqueIdentifier  string    `json:"opaque_identifier"`
	Quantity          int64     `json:"quantity"`
	Eligible          bool      `json:"eligible"`
	CreatedAt         time.Time `json:"created_at"`
}

type TraceEvidence struct {
	EventID          string    `json:"event_id"`
	EvidenceType     string    `json:"evidence_type"`
	ReferenceValue   string    `json:"reference_value"`
	TraceBoxID       *string   `json:"trace_box_id"`
	OpaqueIdentifier *string   `json:"opaque_identifier"`
	CreatedAt        time.Time `json:"created_at"`
}

type DepartmentProgress struct {
	DepartmentID     string `json:"department_id"`
	DepartmentName   string `json:"department_name"`
	RequiredQuantity int64  `json:"required_quantity"`
	TracedQuantity   int64  `json:"traced_quantity"`
	ReadyQuantity    int64  `json:"ready_quantity"`
	PackedQuantity   int64  `json:"packed_quantity"`
}

func (s *Service) LinkTraceBox(ctx context.Context, p auth.Principal, consignmentID string, input TraceLinkInput) (Consignment, bool, error) {
	if err := s.authorizeWork(ctx, p); err != nil {
		return Consignment{}, false, err
	}
	input.LineID = strings.TrimSpace(input.LineID)
	input.TraceBoxIdentifier = strings.ToUpper(strings.TrimSpace(input.TraceBoxIdentifier))
	if !uuidRE.MatchString(input.LineID) || input.TraceBoxIdentifier == "" || input.Quantity <= 0 || input.ExpectedVersion <= 0 {
		return Consignment{}, false, ErrInvalidInput
	}
	operation := fmt.Sprintf("trace_link:%s:%s:%d", input.LineID, input.TraceBoxIdentifier, input.Quantity)
	return s.action(ctx, p, consignmentID, operation, input.IdempotencyKey, input.ExpectedVersion, input.Notes, func(ctx context.Context, tx pgx.Tx, item *Consignment, eventID string) error {
		if !item.TraceabilityRequired || item.Status == "outbound" || item.Status == "completed" || item.Status == "cancelled" {
			return ErrInvalidState
		}
		var productID, departmentID string
		var required, ready, packed int64
		if err := tx.QueryRow(ctx, `SELECT product_id,department_id,required_quantity,ready_quantity,packed_quantity FROM consignment_lines WHERE company_id=$1 AND consignment_id=$2 AND id=$3`, p.CompanyID, item.ID, input.LineID).Scan(&productID, &departmentID, &required, &ready, &packed); err != nil {
			return mapDBError(err)
		}
		if err := s.requireDepartmentWork(ctx, p, departmentID); err != nil {
			return err
		}
		var boxID string
		if err := tx.QueryRow(ctx, `SELECT id FROM trace_boxes WHERE company_id=$1 AND opaque_identifier=$2`, p.CompanyID, input.TraceBoxIdentifier).Scan(&boxID); err != nil {
			return mapDBError(err)
		}
		verified, err := s.trace.VerifiedProductQuantity(ctx, tx, p.CompanyID, boxID, productID)
		if err != nil {
			return ErrIncomplete
		}
		var boxAllocated, lineAllocated int64
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity_delta),0) FROM consignment_trace_box_allocations WHERE company_id=$1 AND trace_box_id=$2 AND product_id=$3`, p.CompanyID, boxID, productID).Scan(&boxAllocated); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity_delta),0) FROM consignment_trace_box_allocations WHERE company_id=$1 AND consignment_line_id=$2`, p.CompanyID, input.LineID).Scan(&lineAllocated); err != nil {
			return err
		}
		if boxAllocated+input.Quantity > verified || lineAllocated+input.Quantity > required || lineAllocated+input.Quantity < ready || lineAllocated+input.Quantity < packed {
			return ErrIncomplete
		}
		if _, err = tx.Exec(ctx, `INSERT INTO consignment_trace_box_allocations(company_id,event_id,event_type,consignment_id,consignment_line_id,product_id,trace_box_id,quantity_delta) VALUES($1,$2,'trace_box_linked',$3,$4,$5,$6,$7)`, p.CompanyID, eventID, item.ID, input.LineID, productID, boxID, input.Quantity); err != nil {
			return mapDBError(err)
		}
		_, err = tx.Exec(ctx, `UPDATE consignments SET version=version+1,updated_at=now() WHERE company_id=$1 AND id=$2`, p.CompanyID, item.ID)
		return err
	})
}

func (s *Service) UnlinkTraceBox(ctx context.Context, p auth.Principal, consignmentID, allocationEventID string, input TraceUnlinkInput) (Consignment, bool, error) {
	if err := s.authorizeWork(ctx, p); err != nil {
		return Consignment{}, false, err
	}
	allocationEventID = strings.TrimSpace(allocationEventID)
	if !uuidRE.MatchString(allocationEventID) || input.ExpectedVersion <= 0 {
		return Consignment{}, false, ErrInvalidInput
	}
	operation := "trace_unlink:" + allocationEventID
	return s.action(ctx, p, consignmentID, operation, input.IdempotencyKey, input.ExpectedVersion, input.Notes, func(ctx context.Context, tx pgx.Tx, item *Consignment, eventID string) error {
		if !item.TraceabilityRequired || item.Status == "outbound" || item.Status == "completed" || item.Status == "cancelled" {
			return ErrInvalidState
		}
		var lineID, productID, boxID, departmentID string
		var quantity, ready, packed int64
		err := tx.QueryRow(ctx, `SELECT a.consignment_line_id,a.product_id,a.trace_box_id,a.quantity_delta,l.department_id,l.ready_quantity,l.packed_quantity FROM consignment_trace_box_allocations a JOIN consignment_lines l ON l.company_id=a.company_id AND l.id=a.consignment_line_id WHERE a.company_id=$1 AND a.consignment_id=$2 AND a.event_id=$3 AND a.event_type='trace_box_linked' AND NOT EXISTS(SELECT 1 FROM consignment_trace_box_allocations u WHERE u.company_id=a.company_id AND u.source_allocation_event_id=a.event_id)`, p.CompanyID, item.ID, allocationEventID).Scan(&lineID, &productID, &boxID, &quantity, &departmentID, &ready, &packed)
		if err != nil {
			return mapDBError(err)
		}
		if err = s.requireDepartmentWork(ctx, p, departmentID); err != nil {
			return err
		}
		var linked int64
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity_delta),0) FROM consignment_trace_box_allocations WHERE company_id=$1 AND consignment_line_id=$2`, p.CompanyID, lineID).Scan(&linked); err != nil {
			return err
		}
		if linked-quantity < ready || linked-quantity < packed {
			return ErrIncomplete
		}
		if _, err = tx.Exec(ctx, `INSERT INTO consignment_trace_box_allocations(company_id,event_id,event_type,consignment_id,consignment_line_id,product_id,trace_box_id,quantity_delta,source_allocation_event_id) VALUES($1,$2,'trace_box_unlinked',$3,$4,$5,$6,$7,$8)`, p.CompanyID, eventID, item.ID, lineID, productID, boxID, -quantity, allocationEventID); err != nil {
			return mapDBError(err)
		}
		_, err = tx.Exec(ctx, `UPDATE consignments SET version=version+1,updated_at=now() WHERE company_id=$1 AND id=$2`, p.CompanyID, item.ID)
		return err
	})
}

func (s *Service) RecordTraceEvidence(ctx context.Context, p auth.Principal, consignmentID string, input TraceEvidenceInput) (Consignment, bool, error) {
	if err := s.authorize(ctx, p, "consignments.manage"); err != nil {
		return Consignment{}, false, err
	}
	input.EvidenceType = strings.TrimSpace(input.EvidenceType)
	input.ReferenceValue = strings.TrimSpace(input.ReferenceValue)
	input.TraceBoxIdentifier = trim(input.TraceBoxIdentifier)
	if (input.EvidenceType != "pouch_reference" && input.EvidenceType != "file_reference") || input.ReferenceValue == "" || len(input.ReferenceValue) > 500 || input.ExpectedVersion <= 0 {
		return Consignment{}, false, ErrInvalidInput
	}
	operation := "trace_evidence:" + input.EvidenceType + ":" + input.ReferenceValue
	if input.TraceBoxIdentifier != nil {
		operation += ":" + strings.ToUpper(*input.TraceBoxIdentifier)
	}
	return s.action(ctx, p, consignmentID, operation, input.IdempotencyKey, input.ExpectedVersion, input.Notes, func(ctx context.Context, tx pgx.Tx, item *Consignment, eventID string) error {
		var boxID *string
		if input.TraceBoxIdentifier != nil {
			var id string
			if err := tx.QueryRow(ctx, `SELECT id FROM trace_boxes WHERE company_id=$1 AND opaque_identifier=$2`, p.CompanyID, strings.ToUpper(*input.TraceBoxIdentifier)).Scan(&id); err != nil {
				return mapDBError(err)
			}
			boxID = &id
		}
		if _, err := tx.Exec(ctx, `INSERT INTO consignment_trace_evidence(company_id,event_id,consignment_id,evidence_type,reference_value,trace_box_id) VALUES($1,$2,$3,$4,$5,$6)`, p.CompanyID, eventID, item.ID, input.EvidenceType, input.ReferenceValue, boxID); err != nil {
			return mapDBError(err)
		}
		_, err := tx.Exec(ctx, `UPDATE consignments SET version=version+1,updated_at=now() WHERE company_id=$1 AND id=$2`, p.CompanyID, item.ID)
		return err
	})
}

func (s *Service) authorizeWork(ctx context.Context, p auth.Principal) error {
	if err := s.authorize(ctx, p, "consignments.work"); err != nil {
		return s.authorize(ctx, p, "consignments.manage")
	}
	return nil
}
func (s *Service) eligibleLineQuantity(ctx context.Context, tx pgx.Tx, companyID, lineID string) (int64, error) {
	var quantity int64
	err := tx.QueryRow(ctx, `SELECT COALESCE(sum(a.quantity_delta),0) FROM consignment_trace_box_allocations a WHERE a.company_id=$1 AND a.consignment_line_id=$2 AND EXISTS(SELECT 1 FROM trace_box_events e WHERE e.company_id=a.company_id AND e.trace_box_id=a.trace_box_id AND e.event_type='ready_for_shipment' AND NOT EXISTS(SELECT 1 FROM trace_box_events n WHERE n.company_id=e.company_id AND n.trace_box_id=e.trace_box_id AND (n.created_at,n.id)>(e.created_at,e.id) AND n.event_type IN ('content_added','content_removed','qc_completed','work_completed','packing_completed','final_check_completed','ready_for_shipment'))) AND NOT EXISTS(SELECT 1 FROM trace_box_handovers h WHERE h.company_id=a.company_id AND h.trace_box_id=a.trace_box_id AND NOT EXISTS(SELECT 1 FROM trace_box_handover_receipts r WHERE r.company_id=h.company_id AND r.handover_id=h.id))`, companyID, lineID).Scan(&quantity)
	return quantity, err
}
func (s *Service) requireTraceCoverage(ctx context.Context, tx pgx.Tx, companyID string, item Consignment) error {
	rows, err := tx.Query(ctx, `SELECT DISTINCT own.trace_box_id,own.product_id,(SELECT COALESCE(sum(allocation.quantity_delta),0) FROM consignment_trace_box_allocations allocation WHERE allocation.company_id=own.company_id AND allocation.trace_box_id=own.trace_box_id AND allocation.product_id=own.product_id) FROM consignment_trace_box_allocations own WHERE own.company_id=$1 AND own.consignment_id=$2 ORDER BY own.trace_box_id,own.product_id`, companyID, item.ID)
	if err != nil {
		return err
	}
	type allocation struct {
		box, product string
		quantity     int64
	}
	values := []allocation{}
	for rows.Next() {
		var value allocation
		if err = rows.Scan(&value.box, &value.product, &value.quantity); err != nil {
			rows.Close()
			return err
		}
		values = append(values, value)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, value := range values {
		verified, e := s.trace.VerifiedProductQuantity(ctx, tx, companyID, value.box, value.product)
		if e != nil || value.quantity > verified {
			return ErrIncomplete
		}
	}
	for _, line := range item.Lines {
		quantity, e := s.eligibleLineQuantity(ctx, tx, companyID, line.ID)
		if e != nil {
			return e
		}
		if quantity < line.RequiredQuantity {
			return ErrIncomplete
		}
	}
	return nil
}

func loadTraceability(ctx context.Context, q querier, companyID, consignmentID string, lines *[]Line) ([]DepartmentProgress, []TraceEvidence, error) {
	lineIndex := map[string]int{}
	departments := map[string]*DepartmentProgress{}
	for i := range *lines {
		lineIndex[(*lines)[i].ID] = i
		(*lines)[i].TraceLinks = []TraceLink{}
		key := (*lines)[i].DepartmentID
		if departments[key] == nil {
			departments[key] = &DepartmentProgress{DepartmentID: key, DepartmentName: (*lines)[i].DepartmentName}
		}
		d := departments[key]
		d.RequiredQuantity += (*lines)[i].RequiredQuantity
		d.ReadyQuantity += (*lines)[i].ReadyQuantity
		d.PackedQuantity += (*lines)[i].PackedQuantity
	}
	rows, err := q.Query(ctx, `SELECT a.consignment_line_id,a.event_id,a.trace_box_id,b.opaque_identifier,a.quantity_delta,a.created_at,EXISTS(SELECT 1 FROM trace_box_events e WHERE e.company_id=a.company_id AND e.trace_box_id=a.trace_box_id AND e.event_type='ready_for_shipment' AND NOT EXISTS(SELECT 1 FROM trace_box_events n WHERE n.company_id=e.company_id AND n.trace_box_id=e.trace_box_id AND (n.created_at,n.id)>(e.created_at,e.id) AND n.event_type IN ('content_added','content_removed','qc_completed','work_completed','packing_completed','final_check_completed','ready_for_shipment'))) AND NOT EXISTS(SELECT 1 FROM trace_box_handovers h WHERE h.company_id=a.company_id AND h.trace_box_id=a.trace_box_id AND NOT EXISTS(SELECT 1 FROM trace_box_handover_receipts r WHERE r.company_id=h.company_id AND r.handover_id=h.id)) FROM consignment_trace_box_allocations a JOIN trace_boxes b ON b.company_id=a.company_id AND b.id=a.trace_box_id WHERE a.company_id=$1 AND a.consignment_id=$2 AND a.event_type='trace_box_linked' AND NOT EXISTS(SELECT 1 FROM consignment_trace_box_allocations u WHERE u.company_id=a.company_id AND u.source_allocation_event_id=a.event_id) ORDER BY a.created_at,a.event_id`, companyID, consignmentID)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var lineID string
		var link TraceLink
		if err = rows.Scan(&lineID, &link.AllocationEventID, &link.TraceBoxID, &link.OpaqueIdentifier, &link.Quantity, &link.CreatedAt, &link.Eligible); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if index, ok := lineIndex[lineID]; ok {
			(*lines)[index].TraceLinks = append((*lines)[index].TraceLinks, link)
			if link.Eligible {
				(*lines)[index].TracedQuantity += link.Quantity
				departments[(*lines)[index].DepartmentID].TracedQuantity += link.Quantity
			}
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}
	progress := make([]DepartmentProgress, 0, len(departments))
	for _, value := range departments {
		progress = append(progress, *value)
	}
	sort.Slice(progress, func(i, j int) bool { return progress[i].DepartmentName < progress[j].DepartmentName })
	evidence := []TraceEvidence{}
	rows, err = q.Query(ctx, `SELECT e.event_id,e.evidence_type,e.reference_value,e.trace_box_id,b.opaque_identifier,e.created_at FROM consignment_trace_evidence e LEFT JOIN trace_boxes b ON b.company_id=e.company_id AND b.id=e.trace_box_id WHERE e.company_id=$1 AND e.consignment_id=$2 ORDER BY e.created_at,e.event_id`, companyID, consignmentID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var value TraceEvidence
		if err = rows.Scan(&value.EventID, &value.EvidenceType, &value.ReferenceValue, &value.TraceBoxID, &value.OpaqueIdentifier, &value.CreatedAt); err != nil {
			return nil, nil, err
		}
		evidence = append(evidence, value)
	}
	return progress, evidence, rows.Err()
}

func filterTraceReadModel(item *Consignment) {
	visible := map[string]bool{}
	for _, line := range item.Lines {
		visible[line.DepartmentID] = true
	}
	filtered := make([]DepartmentProgress, 0, len(item.DepartmentProgress))
	for _, progress := range item.DepartmentProgress {
		if visible[progress.DepartmentID] {
			filtered = append(filtered, progress)
		}
	}
	item.DepartmentProgress = filtered
	item.TraceEvidence = []TraceEvidence{}
}
