// This file owns the server-authoritative worker transitions layered on Trace Boxes.
package traceability

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
)

var ErrInvalidTransition = errors.New("trace box workflow transition is not allowed")

var rejectionReasons = allowedValues("damaged", "wrong_sticker", "dirty", "missing_component", "packaging_damaged", "wrong_product", "manufacturing_defect", "other")
var workTypes = allowedValues("sticker_replacement", "cleaning", "repacking", "component_check", "repair", "other")

type Workflow struct {
	Status           string            `json:"status"`
	LatestQC         *QCCheck          `json:"latest_qc"`
	WorkRequirements []WorkRequirement `json:"work_requirements"`
	PendingHandover  *Handover         `json:"pending_handover"`
	PackedAt         *time.Time        `json:"packed_at"`
	FinalCheck       *FinalCheck       `json:"final_check"`
	ReadyAt          *time.Time        `json:"ready_at"`
}

type QCLine struct {
	ProductID        string  `json:"product_id"`
	CheckedQuantity  int64   `json:"checked_quantity"`
	PassedQuantity   int64   `json:"passed_quantity"`
	RejectedQuantity int64   `json:"rejected_quantity"`
	RejectionReason  *string `json:"rejection_reason"`
	RequiredWork     *string `json:"required_work"`
}

type QCInput struct {
	Lines          []QCLine `json:"lines"`
	Notes          *string  `json:"notes"`
	IdempotencyKey string   `json:"idempotency_key"`
}

type QCCheck struct {
	EventID             string    `json:"event_id"`
	CheckedByEmployeeID string    `json:"checked_by_employee_id"`
	CheckedAt           time.Time `json:"checked_at"`
	PassedQuantity      int64     `json:"passed_quantity"`
	RejectedQuantity    int64     `json:"rejected_quantity"`
}

type WorkRequirement struct {
	ID                    string     `json:"id"`
	ProductID             string     `json:"product_id"`
	InternalCode          string     `json:"internal_code"`
	ProductName           string     `json:"product_name"`
	Quantity              int64      `json:"quantity"`
	WorkType              string     `json:"work_type"`
	RejectionReason       string     `json:"rejection_reason"`
	CreatedAt             time.Time  `json:"created_at"`
	CompletedAt           *time.Time `json:"completed_at"`
	CompletedByEmployeeID *string    `json:"completed_by_employee_id"`
}

type CompleteWorkInput struct {
	Notes          *string `json:"notes"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type HandoverInput struct {
	EmployeeID     *string `json:"employee_id"`
	DepartmentID   *string `json:"department_id"`
	Notes          *string `json:"notes"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type ReceiveHandoverInput struct {
	Notes          *string `json:"notes"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type Handover struct {
	ID                 string    `json:"id"`
	TargetEmployeeID   *string   `json:"target_employee_id"`
	TargetDepartmentID *string   `json:"target_department_id"`
	TargetName         string    `json:"target_name"`
	SentAt             time.Time `json:"sent_at"`
}

type GateInput struct {
	Passed         *bool   `json:"passed,omitempty"`
	Notes          *string `json:"notes"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type FinalCheck struct {
	EventID   string    `json:"event_id"`
	Passed    bool      `json:"passed"`
	CheckedAt time.Time `json:"checked_at"`
}

func (s *Service) RecordQC(ctx context.Context, principal auth.Principal, boxID string, input QCInput) (TraceBox, bool, error) {
	if err := s.authorize(ctx, principal, "traceability.manage"); err != nil {
		return TraceBox{}, false, err
	}
	boxID, input.IdempotencyKey = strings.TrimSpace(boxID), strings.TrimSpace(input.IdempotencyKey)
	input.Notes = cleanOptional(input.Notes)
	if !uuidRE.MatchString(boxID) || !validKey(input.IdempotencyKey) || len(input.Lines) == 0 || input.Notes != nil && len(*input.Notes) > 2000 {
		return TraceBox{}, false, ErrInvalidInput
	}
	seen := map[string]bool{}
	for i := range input.Lines {
		line := &input.Lines[i]
		line.ProductID = strings.TrimSpace(line.ProductID)
		line.RejectionReason, line.RequiredWork = cleanOptional(line.RejectionReason), cleanOptional(line.RequiredWork)
		if !uuidRE.MatchString(line.ProductID) || seen[line.ProductID] || line.CheckedQuantity <= 0 || line.PassedQuantity < 0 || line.RejectedQuantity < 0 || line.CheckedQuantity != line.PassedQuantity+line.RejectedQuantity {
			return TraceBox{}, false, ErrInvalidInput
		}
		seen[line.ProductID] = true
		if line.RejectedQuantity == 0 {
			if line.RejectionReason != nil || line.RequiredWork != nil {
				return TraceBox{}, false, ErrInvalidInput
			}
		} else if line.RejectionReason == nil || line.RequiredWork == nil || !rejectionReasons[*line.RejectionReason] || !workTypes[*line.RequiredWork] {
			return TraceBox{}, false, ErrInvalidInput
		}
	}
	sort.Slice(input.Lines, func(i, j int) bool { return input.Lines[i].ProductID < input.Lines[j].ProductID })
	hash := requestHash(struct {
		BoxID string
		Input QCInput
	}{boxID, input})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TraceBox{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if replayed, replayErr := existingEvent(ctx, tx, principal.CompanyID, boxID, "qc_completed", input.IdempotencyKey, hash); replayErr != nil || replayed {
		return s.finishReplay(ctx, tx, principal.CompanyID, boxID, replayed, replayErr)
	}
	if err = lockBox(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	if err = assertNoPendingHandover(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	employeeID, err := actorEmployee(ctx, tx, principal)
	if err != nil {
		return TraceBox{}, false, err
	}
	rows, err := tx.Query(ctx, `SELECT product_id,sum(quantity_delta) FROM trace_box_content_changes WHERE company_id=$1 AND trace_box_id=$2 GROUP BY product_id HAVING sum(quantity_delta)>0`, principal.CompanyID, boxID)
	if err != nil {
		return TraceBox{}, false, err
	}
	contents := map[string]int64{}
	for rows.Next() {
		var id string
		var qty int64
		if err = rows.Scan(&id, &qty); err != nil {
			rows.Close()
			return TraceBox{}, false, err
		}
		contents[id] = qty
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return TraceBox{}, false, err
	}
	if len(contents) != len(input.Lines) {
		return TraceBox{}, false, ErrQuantity
	}
	var passed, rejected int64
	for _, line := range input.Lines {
		if contents[line.ProductID] != line.CheckedQuantity {
			return TraceBox{}, false, ErrQuantity
		}
		passed += line.PassedQuantity
		rejected += line.RejectedQuantity
	}
	eventID, err := insertEvent(ctx, tx, principal, boxID, "qc_completed", input.Notes, input.IdempotencyKey, hash, map[string]any{"passed_quantity": passed, "rejected_quantity": rejected})
	if err != nil {
		return TraceBox{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO trace_box_qc_checks(company_id,event_id,trace_box_id,checked_by_employee_id) VALUES($1,$2,$3,$4)`, principal.CompanyID, eventID, boxID, employeeID); err != nil {
		return TraceBox{}, false, mapDBError(err)
	}
	for _, line := range input.Lines {
		if _, err = tx.Exec(ctx, `INSERT INTO trace_box_qc_lines(company_id,qc_event_id,trace_box_id,product_id,checked_quantity,passed_quantity,rejected_quantity,rejection_reason,required_work) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, principal.CompanyID, eventID, boxID, line.ProductID, line.CheckedQuantity, line.PassedQuantity, line.RejectedQuantity, line.RejectionReason, line.RequiredWork); err != nil {
			return TraceBox{}, false, mapDBError(err)
		}
		if line.RejectedQuantity > 0 {
			if _, err = tx.Exec(ctx, `INSERT INTO trace_box_work_requirements(company_id,trace_box_id,qc_event_id,product_id,quantity,work_type,rejection_reason) VALUES($1,$2,$3,$4,$5,$6,$7)`, principal.CompanyID, boxID, eventID, line.ProductID, line.RejectedQuantity, *line.RequiredWork, *line.RejectionReason); err != nil {
				return TraceBox{}, false, mapDBError(err)
			}
		}
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "trace_box.qc_completed", "trace_box", boxID, map[string]any{"passed_quantity": passed, "rejected_quantity": rejected}); err != nil {
		return TraceBox{}, false, err
	}
	return s.finishAction(ctx, tx, principal.CompanyID, boxID)
}

func (s *Service) CompleteWork(ctx context.Context, principal auth.Principal, boxID, requirementID string, input CompleteWorkInput) (TraceBox, bool, error) {
	if err := s.authorize(ctx, principal, "traceability.manage"); err != nil {
		return TraceBox{}, false, err
	}
	boxID, requirementID, input.IdempotencyKey = strings.TrimSpace(boxID), strings.TrimSpace(requirementID), strings.TrimSpace(input.IdempotencyKey)
	input.Notes = cleanOptional(input.Notes)
	if !uuidRE.MatchString(boxID) || !uuidRE.MatchString(requirementID) || !validKey(input.IdempotencyKey) || input.Notes != nil && len(*input.Notes) > 2000 {
		return TraceBox{}, false, ErrInvalidInput
	}
	hash := requestHash(struct {
		BoxID, RequirementID string
		Input                CompleteWorkInput
	}{boxID, requirementID, input})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TraceBox{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if replayed, replayErr := existingEvent(ctx, tx, principal.CompanyID, boxID, "work_completed", input.IdempotencyKey, hash); replayErr != nil || replayed {
		return s.finishReplay(ctx, tx, principal.CompanyID, boxID, replayed, replayErr)
	}
	if err = lockBox(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	if err = assertNoPendingHandover(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	employeeID, err := actorEmployee(ctx, tx, principal)
	if err != nil {
		return TraceBox{}, false, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM trace_box_work_requirements w WHERE w.company_id=$1 AND w.trace_box_id=$2 AND w.id=$3 AND NOT EXISTS(SELECT 1 FROM trace_box_work_completions c WHERE c.company_id=w.company_id AND c.work_requirement_id=w.id))`, principal.CompanyID, boxID, requirementID).Scan(&exists); err != nil {
		return TraceBox{}, false, err
	}
	if !exists {
		return TraceBox{}, false, ErrInvalidTransition
	}
	eventID, err := insertEvent(ctx, tx, principal, boxID, "work_completed", input.Notes, input.IdempotencyKey, hash, map[string]any{"work_requirement_id": requirementID})
	if err != nil {
		return TraceBox{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO trace_box_work_completions(company_id,event_id,trace_box_id,work_requirement_id,completed_by_employee_id) VALUES($1,$2,$3,$4,$5)`, principal.CompanyID, eventID, boxID, requirementID, employeeID); err != nil {
		return TraceBox{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "trace_box.work_completed", "trace_box", boxID, map[string]any{"work_requirement_id": requirementID}); err != nil {
		return TraceBox{}, false, err
	}
	return s.finishAction(ctx, tx, principal.CompanyID, boxID)
}

func (s *Service) SendHandover(ctx context.Context, principal auth.Principal, boxID string, input HandoverInput) (TraceBox, bool, error) {
	if err := s.authorize(ctx, principal, "traceability.manage"); err != nil {
		return TraceBox{}, false, err
	}
	boxID, input.IdempotencyKey = strings.TrimSpace(boxID), strings.TrimSpace(input.IdempotencyKey)
	input.EmployeeID, input.DepartmentID, input.Notes = cleanOptional(input.EmployeeID), cleanOptional(input.DepartmentID), cleanOptional(input.Notes)
	if !uuidRE.MatchString(boxID) || (input.EmployeeID == nil) == (input.DepartmentID == nil) || input.EmployeeID != nil && !uuidRE.MatchString(*input.EmployeeID) || input.DepartmentID != nil && !uuidRE.MatchString(*input.DepartmentID) || !validKey(input.IdempotencyKey) {
		return TraceBox{}, false, ErrInvalidInput
	}
	hash := requestHash(struct {
		BoxID string
		Input HandoverInput
	}{boxID, input})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TraceBox{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if replayed, replayErr := existingEvent(ctx, tx, principal.CompanyID, boxID, "handover_sent", input.IdempotencyKey, hash); replayErr != nil || replayed {
		return s.finishReplay(ctx, tx, principal.CompanyID, boxID, replayed, replayErr)
	}
	if err = lockBox(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	if err = assertNoPendingHandover(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	var targetExists bool
	metadata := map[string]any{}
	if input.EmployeeID != nil {
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM employees WHERE company_id=$1 AND id=$2 AND status='active')`, principal.CompanyID, *input.EmployeeID).Scan(&targetExists)
		metadata["employee_id"] = *input.EmployeeID
	} else {
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM consignment_departments WHERE company_id=$1 AND id=$2 AND status='active')`, principal.CompanyID, *input.DepartmentID).Scan(&targetExists)
		metadata["department_id"] = *input.DepartmentID
	}
	if err != nil {
		return TraceBox{}, false, err
	}
	if !targetExists {
		return TraceBox{}, false, ErrNotFound
	}
	eventID, err := insertEvent(ctx, tx, principal, boxID, "handover_sent", input.Notes, input.IdempotencyKey, hash, metadata)
	if err != nil {
		return TraceBox{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO trace_box_handovers(company_id,trace_box_id,sent_event_id,target_employee_id,target_department_id) VALUES($1,$2,$3,$4,$5)`, principal.CompanyID, boxID, eventID, input.EmployeeID, input.DepartmentID); err != nil {
		return TraceBox{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "trace_box.handover_sent", "trace_box", boxID, metadata); err != nil {
		return TraceBox{}, false, err
	}
	return s.finishAction(ctx, tx, principal.CompanyID, boxID)
}

func (s *Service) ReceiveHandover(ctx context.Context, principal auth.Principal, boxID, handoverID string, input ReceiveHandoverInput) (TraceBox, bool, error) {
	if err := s.authorize(ctx, principal, "traceability.manage"); err != nil {
		return TraceBox{}, false, err
	}
	boxID, handoverID, input.IdempotencyKey = strings.TrimSpace(boxID), strings.TrimSpace(handoverID), strings.TrimSpace(input.IdempotencyKey)
	input.Notes = cleanOptional(input.Notes)
	if !uuidRE.MatchString(boxID) || !uuidRE.MatchString(handoverID) || !validKey(input.IdempotencyKey) {
		return TraceBox{}, false, ErrInvalidInput
	}
	hash := requestHash(struct {
		BoxID, HandoverID string
		Input             ReceiveHandoverInput
	}{boxID, handoverID, input})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TraceBox{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if replayed, replayErr := existingEvent(ctx, tx, principal.CompanyID, boxID, "handover_received", input.IdempotencyKey, hash); replayErr != nil || replayed {
		return s.finishReplay(ctx, tx, principal.CompanyID, boxID, replayed, replayErr)
	}
	if err = lockBox(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	var targetEmployee, targetDepartment *string
	err = tx.QueryRow(ctx, `SELECT h.target_employee_id,h.target_department_id FROM trace_box_handovers h WHERE h.company_id=$1 AND h.trace_box_id=$2 AND h.id=$3 AND NOT EXISTS(SELECT 1 FROM trace_box_handover_receipts r WHERE r.company_id=h.company_id AND r.handover_id=h.id)`, principal.CompanyID, boxID, handoverID).Scan(&targetEmployee, &targetDepartment)
	if errors.Is(err, pgx.ErrNoRows) {
		return TraceBox{}, false, ErrInvalidTransition
	}
	if err != nil {
		return TraceBox{}, false, err
	}
	employeeID, err := actorEmployee(ctx, tx, principal)
	if err != nil {
		return TraceBox{}, false, err
	}
	allowed := targetEmployee != nil && *targetEmployee == employeeID
	if targetDepartment != nil {
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM consignment_department_members WHERE company_id=$1 AND department_id=$2 AND employee_id=$3)`, principal.CompanyID, *targetDepartment, employeeID).Scan(&allowed); err != nil {
			return TraceBox{}, false, err
		}
	}
	if !allowed {
		return TraceBox{}, false, ErrInvalidTransition
	}
	eventID, err := insertEvent(ctx, tx, principal, boxID, "handover_received", input.Notes, input.IdempotencyKey, hash, map[string]any{"handover_id": handoverID, "employee_id": employeeID})
	if err != nil {
		return TraceBox{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO trace_box_handover_receipts(company_id,event_id,trace_box_id,handover_id,received_by_employee_id) VALUES($1,$2,$3,$4,$5)`, principal.CompanyID, eventID, boxID, handoverID, employeeID); err != nil {
		return TraceBox{}, false, mapDBError(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO trace_box_custody_changes(company_id,event_id,event_type,trace_box_id,employee_id) VALUES($1,$2,'handover_received',$3,$4)`, principal.CompanyID, eventID, boxID, employeeID); err != nil {
		return TraceBox{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "trace_box.handover_received", "trace_box", boxID, map[string]any{"handover_id": handoverID}); err != nil {
		return TraceBox{}, false, err
	}
	return s.finishAction(ctx, tx, principal.CompanyID, boxID)
}

func (s *Service) CompletePacking(ctx context.Context, p auth.Principal, boxID string, input GateInput) (TraceBox, bool, error) {
	return s.recordGate(ctx, p, boxID, "packing_completed", input)
}
func (s *Service) CompleteFinalCheck(ctx context.Context, p auth.Principal, boxID string, input GateInput) (TraceBox, bool, error) {
	return s.recordGate(ctx, p, boxID, "final_check_completed", input)
}
func (s *Service) MarkReady(ctx context.Context, p auth.Principal, boxID string, input GateInput) (TraceBox, bool, error) {
	return s.recordGate(ctx, p, boxID, "ready_for_shipment", input)
}

func (s *Service) recordGate(ctx context.Context, p auth.Principal, boxID, eventType string, input GateInput) (TraceBox, bool, error) {
	if err := s.authorize(ctx, p, "traceability.manage"); err != nil {
		return TraceBox{}, false, err
	}
	boxID, input.IdempotencyKey = strings.TrimSpace(boxID), strings.TrimSpace(input.IdempotencyKey)
	input.Notes = cleanOptional(input.Notes)
	if !uuidRE.MatchString(boxID) || !validKey(input.IdempotencyKey) || (eventType == "final_check_completed") != (input.Passed != nil) || eventType == "final_check_completed" && !*input.Passed && input.Notes == nil {
		return TraceBox{}, false, ErrInvalidInput
	}
	hash := requestHash(struct {
		BoxID, EventType string
		Input            GateInput
	}{boxID, eventType, input})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TraceBox{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if replayed, replayErr := existingEvent(ctx, tx, p.CompanyID, boxID, eventType, input.IdempotencyKey, hash); replayErr != nil || replayed {
		return s.finishReplay(ctx, tx, p.CompanyID, boxID, replayed, replayErr)
	}
	if err = lockBox(ctx, tx, p.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	if err = assertNoPendingHandover(ctx, tx, p.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	employeeID, err := actorEmployee(ctx, tx, p)
	if err != nil {
		return TraceBox{}, false, err
	}
	allowed := false
	switch eventType {
	case "packing_completed":
		allowed, err = canPack(ctx, tx, p.CompanyID, boxID)
	case "final_check_completed":
		allowed, err = latestIs(ctx, tx, p.CompanyID, boxID, "packing_completed")
	case "ready_for_shipment":
		allowed, err = latestPassedFinal(ctx, tx, p.CompanyID, boxID)
	}
	if err != nil {
		return TraceBox{}, false, err
	}
	if !allowed {
		return TraceBox{}, false, ErrInvalidTransition
	}
	metadata := map[string]any{}
	if input.Passed != nil {
		metadata["passed"] = *input.Passed
	}
	eventID, err := insertEvent(ctx, tx, p, boxID, eventType, input.Notes, input.IdempotencyKey, hash, metadata)
	if err != nil {
		return TraceBox{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO trace_box_gate_records(company_id,event_id,event_type,trace_box_id,employee_id,passed) VALUES($1,$2,$3,$4,$5,$6)`, p.CompanyID, eventID, eventType, boxID, employeeID, input.Passed); err != nil {
		return TraceBox{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "trace_box."+eventType, "trace_box", boxID, metadata); err != nil {
		return TraceBox{}, false, err
	}
	return s.finishAction(ctx, tx, p.CompanyID, boxID)
}

func actorEmployee(ctx context.Context, tx pgx.Tx, p auth.Principal) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM employees WHERE company_id=$1 AND user_id=$2 AND status='active'`, p.CompanyID, p.UserID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidTransition
	}
	return id, err
}
func assertNoPendingHandover(ctx context.Context, tx pgx.Tx, companyID, boxID string) error {
	var pending bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM trace_box_handovers h WHERE h.company_id=$1 AND h.trace_box_id=$2 AND NOT EXISTS(SELECT 1 FROM trace_box_handover_receipts r WHERE r.company_id=h.company_id AND r.handover_id=h.id))`, companyID, boxID).Scan(&pending)
	if err != nil {
		return err
	}
	if pending {
		return ErrInvalidTransition
	}
	return nil
}
func canPack(ctx context.Context, tx pgx.Tx, companyID, boxID string) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM trace_box_qc_checks q JOIN trace_box_events e ON e.company_id=q.company_id AND e.id=q.event_id WHERE q.company_id=$1 AND q.trace_box_id=$2 AND NOT EXISTS(SELECT 1 FROM trace_box_qc_lines l WHERE l.company_id=q.company_id AND l.qc_event_id=q.event_id AND l.rejected_quantity>0) AND NOT EXISTS(SELECT 1 FROM trace_box_events n WHERE n.company_id=e.company_id AND n.trace_box_id=e.trace_box_id AND (n.created_at,n.id)>(e.created_at,e.id) AND n.event_type IN ('content_added','content_removed','work_completed','qc_completed'))) AND NOT EXISTS(SELECT 1 FROM trace_box_work_requirements w WHERE w.company_id=$1 AND w.trace_box_id=$2 AND NOT EXISTS(SELECT 1 FROM trace_box_work_completions c WHERE c.company_id=w.company_id AND c.work_requirement_id=w.id))`, companyID, boxID).Scan(&ok)
	return ok, err
}
func latestIs(ctx context.Context, tx pgx.Tx, companyID, boxID, eventType string) (bool, error) {
	var got string
	err := tx.QueryRow(ctx, `SELECT event_type FROM trace_box_events WHERE company_id=$1 AND trace_box_id=$2 AND event_type IN ('content_added','content_removed','qc_completed','work_completed','packing_completed','final_check_completed','ready_for_shipment') ORDER BY created_at DESC,id DESC LIMIT 1`, companyID, boxID).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return got == eventType, err
}
func latestPassedFinal(ctx context.Context, tx pgx.Tx, companyID, boxID string) (bool, error) {
	var eventType string
	var passed *bool
	err := tx.QueryRow(ctx, `SELECT e.event_type,g.passed FROM trace_box_events e LEFT JOIN trace_box_gate_records g ON g.company_id=e.company_id AND g.event_id=e.id WHERE e.company_id=$1 AND e.trace_box_id=$2 AND e.event_type IN ('content_added','content_removed','qc_completed','work_completed','packing_completed','final_check_completed','ready_for_shipment') ORDER BY e.created_at DESC,e.id DESC LIMIT 1`, companyID, boxID).Scan(&eventType, &passed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil && eventType == "final_check_completed" && passed != nil && *passed, err
}
func (s *Service) finishReplay(ctx context.Context, tx pgx.Tx, companyID, boxID string, replayed bool, err error) (TraceBox, bool, error) {
	if err != nil {
		return TraceBox{}, false, err
	}
	if !replayed {
		return TraceBox{}, false, nil
	}
	if err = tx.Commit(ctx); err != nil {
		return TraceBox{}, false, err
	}
	item, err := s.load(ctx, companyID, boxID)
	return item, true, err
}
func (s *Service) finishAction(ctx context.Context, tx pgx.Tx, companyID, boxID string) (TraceBox, bool, error) {
	if err := tx.Commit(ctx); err != nil {
		return TraceBox{}, false, err
	}
	item, err := s.load(ctx, companyID, boxID)
	return item, false, err
}
func allowedValues(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func (s *Service) loadWorkflow(ctx context.Context, companyID, boxID string, hasContents bool) (Workflow, error) {
	result := Workflow{Status: "empty", WorkRequirements: []WorkRequirement{}}
	if hasContents {
		result.Status = "awaiting_qc"
	}
	var qc QCCheck
	err := s.db.QueryRow(ctx, `SELECT q.event_id,q.checked_by_employee_id,e.created_at,COALESCE(sum(l.passed_quantity),0),COALESCE(sum(l.rejected_quantity),0) FROM trace_box_qc_checks q JOIN trace_box_events e ON e.company_id=q.company_id AND e.id=q.event_id JOIN trace_box_qc_lines l ON l.company_id=q.company_id AND l.qc_event_id=q.event_id WHERE q.company_id=$1 AND q.trace_box_id=$2 GROUP BY q.event_id,q.checked_by_employee_id,e.created_at ORDER BY e.created_at DESC,q.event_id DESC LIMIT 1`, companyID, boxID).Scan(&qc.EventID, &qc.CheckedByEmployeeID, &qc.CheckedAt, &qc.PassedQuantity, &qc.RejectedQuantity)
	if err == nil {
		result.LatestQC = &qc
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT w.id,w.product_id,p.internal_code,p.name,w.quantity,w.work_type,w.rejection_reason,w.created_at,c.completed_at,c.completed_by_employee_id FROM trace_box_work_requirements w JOIN products p ON p.company_id=w.company_id AND p.id=w.product_id LEFT JOIN (SELECT c.company_id,c.work_requirement_id,e.created_at AS completed_at,c.completed_by_employee_id FROM trace_box_work_completions c JOIN trace_box_events e ON e.company_id=c.company_id AND e.id=c.event_id) c ON c.company_id=w.company_id AND c.work_requirement_id=w.id WHERE w.company_id=$1 AND w.trace_box_id=$2 ORDER BY w.created_at,w.id`, companyID, boxID)
	if err != nil {
		return Workflow{}, err
	}
	openWork := false
	for rows.Next() {
		var item WorkRequirement
		if err = rows.Scan(&item.ID, &item.ProductID, &item.InternalCode, &item.ProductName, &item.Quantity, &item.WorkType, &item.RejectionReason, &item.CreatedAt, &item.CompletedAt, &item.CompletedByEmployeeID); err != nil {
			rows.Close()
			return Workflow{}, err
		}
		if item.CompletedAt == nil {
			openWork = true
		}
		result.WorkRequirements = append(result.WorkRequirements, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return Workflow{}, err
	}
	var handover Handover
	err = s.db.QueryRow(ctx, `SELECT h.id,h.target_employee_id,h.target_department_id,COALESCE(e.display_name,d.name),h.sent_at FROM trace_box_handovers h LEFT JOIN employees e ON e.company_id=h.company_id AND e.id=h.target_employee_id LEFT JOIN consignment_departments d ON d.company_id=h.company_id AND d.id=h.target_department_id WHERE h.company_id=$1 AND h.trace_box_id=$2 AND NOT EXISTS(SELECT 1 FROM trace_box_handover_receipts r WHERE r.company_id=h.company_id AND r.handover_id=h.id) ORDER BY h.sent_at DESC,h.id DESC LIMIT 1`, companyID, boxID).Scan(&handover.ID, &handover.TargetEmployeeID, &handover.TargetDepartmentID, &handover.TargetName, &handover.SentAt)
	if err == nil {
		result.PendingHandover = &handover
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, err
	}
	err = s.db.QueryRow(ctx, `SELECT e.created_at FROM trace_box_gate_records g JOIN trace_box_events e ON e.company_id=g.company_id AND e.id=g.event_id WHERE g.company_id=$1 AND g.trace_box_id=$2 AND g.event_type='packing_completed' ORDER BY e.created_at DESC,e.id DESC LIMIT 1`, companyID, boxID).Scan(&result.PackedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, err
	}
	var final FinalCheck
	err = s.db.QueryRow(ctx, `SELECT g.event_id,g.passed,e.created_at FROM trace_box_gate_records g JOIN trace_box_events e ON e.company_id=g.company_id AND e.id=g.event_id WHERE g.company_id=$1 AND g.trace_box_id=$2 AND g.event_type='final_check_completed' ORDER BY e.created_at DESC,e.id DESC LIMIT 1`, companyID, boxID).Scan(&final.EventID, &final.Passed, &final.CheckedAt)
	if err == nil {
		result.FinalCheck = &final
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, err
	}
	err = s.db.QueryRow(ctx, `SELECT e.created_at FROM trace_box_gate_records g JOIN trace_box_events e ON e.company_id=g.company_id AND e.id=g.event_id WHERE g.company_id=$1 AND g.trace_box_id=$2 AND g.event_type='ready_for_shipment' ORDER BY e.created_at DESC,e.id DESC LIMIT 1`, companyID, boxID).Scan(&result.ReadyAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, err
	}
	var latest string
	err = s.db.QueryRow(ctx, `SELECT event_type FROM trace_box_events WHERE company_id=$1 AND trace_box_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1`, companyID, boxID).Scan(&latest)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, err
	}
	switch latest {
	case "qc_completed":
		if qc.RejectedQuantity == 0 {
			result.Status = "qc_passed"
		} else {
			result.Status = "rework_required"
		}
	case "work_completed":
		result.Status = "awaiting_recheck"
	case "handover_sent":
		result.Status = "in_transit"
	case "handover_received":
		result.Status = "handover_received"
	case "packing_completed":
		result.Status = "packed"
	case "final_check_completed":
		if result.FinalCheck != nil && result.FinalCheck.Passed {
			result.Status = "final_check_passed"
		} else {
			result.Status = "final_check_failed"
		}
	case "ready_for_shipment":
		result.Status = "ready_for_shipment"
	case "content_added", "content_removed":
		result.Status = "awaiting_qc"
	}
	if openWork {
		result.Status = "rework_required"
	}
	if result.PendingHandover != nil {
		result.Status = "in_transit"
	}
	return result, nil
}
