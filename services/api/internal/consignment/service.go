// This file coordinates the package's business rules and persistence operations behind a reusable API in the consignment package.
package consignment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/audit"
	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/inventory"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("consignment not found")
	ErrInvalidInput  = errors.New("invalid consignment input")
	ErrConflict      = errors.New("consignment conflict")
	ErrInvalidState  = errors.New("consignment state transition is not allowed")
	ErrIncomplete    = errors.New("consignment quantities are incomplete")
	ErrInsufficient  = errors.New("consignment stock is insufficient")
	uuidRE           = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	validStatuses    = map[string]bool{"created": true, "allocated": true, "picking": true, "ready": true, "packing": true, "packed": true, "outbound": true, "completed": true, "cancelled": true}
	validTransitions = map[string]string{"allocated:picking": "picking", "picking:ready": "ready", "ready:packing": "packing", "packing:packed": "packed", "outbound:completed": "completed"}
)

type Service struct {
	db         *pgxpool.Pool
	authorizer *authorization.Service
	inventory  *inventory.Service
	audit      audit.Recorder
}

type Department struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Members   []Member  `json:"members"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Member struct {
	EmployeeID string  `json:"employee_id"`
	Name       string  `json:"name"`
	UserID     *string `json:"user_id"`
}
type DepartmentInput struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}
type MembershipInput struct {
	EmployeeIDs []string `json:"employee_ids"`
}
type LineInput struct {
	ProductID        string `json:"product_id"`
	DepartmentID     string `json:"department_id"`
	RequiredQuantity int64  `json:"required_quantity"`
}
type CreateInput struct {
	OrderReference  string      `json:"order_reference"`
	DealerReference *string     `json:"dealer_reference"`
	PouchReference  *string     `json:"pouch_reference"`
	SourceType      string      `json:"source_type"`
	SourceReference *string     `json:"source_reference"`
	Notes           *string     `json:"notes"`
	Lines           []LineInput `json:"lines"`
	IdempotencyKey  string      `json:"idempotency_key"`
}
type ActionInput struct {
	Notes           *string `json:"notes"`
	IdempotencyKey  string  `json:"idempotency_key"`
	ExpectedVersion int     `json:"expected_version"`
}
type TransitionInput struct {
	TargetStatus    string  `json:"target_status"`
	Notes           *string `json:"notes"`
	IdempotencyKey  string  `json:"idempotency_key"`
	ExpectedVersion int     `json:"expected_version"`
}
type ProgressInput struct {
	ReadyQuantity   int64   `json:"ready_quantity"`
	PackedQuantity  int64   `json:"packed_quantity"`
	Notes           *string `json:"notes"`
	IdempotencyKey  string  `json:"idempotency_key"`
	ExpectedVersion int     `json:"expected_version"`
}
type Consignment struct {
	ID              string     `json:"id"`
	OrderReference  string     `json:"order_reference"`
	DealerReference *string    `json:"dealer_reference"`
	PouchReference  *string    `json:"pouch_reference"`
	SourceType      string     `json:"source_type"`
	SourceReference *string    `json:"source_reference"`
	Status          string     `json:"status"`
	Notes           *string    `json:"notes"`
	CreatedBy       string     `json:"created_by"`
	AllocatedBy     *string    `json:"allocated_by"`
	OutboundBy      *string    `json:"outbound_by"`
	CompletedBy     *string    `json:"completed_by"`
	CancelledBy     *string    `json:"cancelled_by"`
	AllocatedAt     *time.Time `json:"allocated_at"`
	OutboundAt      *time.Time `json:"outbound_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CancelledAt     *time.Time `json:"cancelled_at"`
	Version         int        `json:"version"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Lines           []Line     `json:"lines"`
	Events          []Event    `json:"events"`
}
type Line struct {
	ID               string    `json:"id"`
	ProductID        string    `json:"product_id"`
	InternalCode     string    `json:"internal_code"`
	ProductName      string    `json:"product_name"`
	DepartmentID     string    `json:"department_id"`
	DepartmentName   string    `json:"department_name"`
	RequiredQuantity int64     `json:"required_quantity"`
	ReadyQuantity    int64     `json:"ready_quantity"`
	PackedQuantity   int64     `json:"packed_quantity"`
	Progress         string    `json:"progress"`
	Version          int       `json:"version"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type Event struct {
	ID          string          `json:"id"`
	EventType   string          `json:"event_type"`
	ActorUserID string          `json:"actor_user_id"`
	Notes       *string         `json:"notes"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
}
type Filter struct {
	Status, DepartmentID, Query string
}

func NewService(db *pgxpool.Pool, authorizer *authorization.Service, inventoryService *inventory.Service) *Service {
	return &Service{db: db, authorizer: authorizer, inventory: inventoryService}
}

func (s *Service) ListDepartments(ctx context.Context, p auth.Principal) ([]Department, error) {
	broad, err := s.access(ctx, p)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT d.id,d.name,d.status,d.created_at,d.updated_at FROM consignment_departments d WHERE d.company_id=$1 AND ($2 OR EXISTS(SELECT 1 FROM consignment_department_members dm JOIN employees e ON e.company_id=dm.company_id AND e.id=dm.employee_id WHERE dm.company_id=d.company_id AND dm.department_id=d.id AND e.user_id=$3)) ORDER BY d.name,d.id`, p.CompanyID, broad, p.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Department{}
	for rows.Next() {
		var item Department
		if err = rows.Scan(&item.ID, &item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Members, err = s.loadMembers(ctx, p.CompanyID, item.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) CreateDepartment(ctx context.Context, p auth.Principal, input DepartmentInput) (Department, error) {
	if err := s.authorize(ctx, p, "consignments.manage"); err != nil {
		return Department{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Status = strings.TrimSpace(input.Status)
	if input.Status == "" {
		input.Status = "active"
	}
	if input.Name == "" || len(input.Name) > 100 || (input.Status != "active" && input.Status != "inactive") {
		return Department{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Department{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item Department
	err = tx.QueryRow(ctx, `INSERT INTO consignment_departments(company_id,name,status,created_by) VALUES($1,$2,$3,$4) RETURNING id,name,status,created_at,updated_at`, p.CompanyID, input.Name, input.Status, p.UserID).Scan(&item.ID, &item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Department{}, mapDBError(err)
	}
	item.Members = []Member{}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "consignment.department_created", "consignment_department", item.ID, map[string]any{"name": item.Name}); err != nil {
		return Department{}, err
	}
	return item, tx.Commit(ctx)
}

func (s *Service) UpdateDepartment(ctx context.Context, p auth.Principal, id string, input DepartmentInput) (Department, error) {
	if err := s.authorize(ctx, p, "consignments.manage"); err != nil {
		return Department{}, err
	}
	input.Name, input.Status = strings.TrimSpace(input.Name), strings.TrimSpace(input.Status)
	if !uuidRE.MatchString(id) || input.Name == "" || len(input.Name) > 100 || (input.Status != "active" && input.Status != "inactive") {
		return Department{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Department{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item Department
	err = tx.QueryRow(ctx, `UPDATE consignment_departments SET name=$1,status=$2,updated_at=now() WHERE company_id=$3 AND id=$4 RETURNING id,name,status,created_at,updated_at`, input.Name, input.Status, p.CompanyID, id).Scan(&item.ID, &item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Department{}, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "consignment.department_updated", "consignment_department", id, map[string]any{"name": item.Name, "status": item.Status}); err != nil {
		return Department{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Department{}, err
	}
	item.Members, err = s.loadMembers(ctx, p.CompanyID, id)
	return item, err
}

func (s *Service) SetDepartmentMembers(ctx context.Context, p auth.Principal, id string, input MembershipInput) (Department, error) {
	if err := s.authorize(ctx, p, "consignments.manage"); err != nil {
		return Department{}, err
	}
	if !uuidRE.MatchString(id) || len(input.EmployeeIDs) > 200 {
		return Department{}, ErrInvalidInput
	}
	seen := map[string]bool{}
	for _, employeeID := range input.EmployeeIDs {
		if !uuidRE.MatchString(employeeID) || seen[employeeID] {
			return Department{}, ErrInvalidInput
		}
		seen[employeeID] = true
	}
	sort.Strings(input.EmployeeIDs)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Department{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item Department
	if err = tx.QueryRow(ctx, `SELECT id,name,status,created_at,updated_at FROM consignment_departments WHERE company_id=$1 AND id=$2 FOR UPDATE`, p.CompanyID, id).Scan(&item.ID, &item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return Department{}, mapDBError(err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM consignment_department_members WHERE company_id=$1 AND department_id=$2`, p.CompanyID, id); err != nil {
		return Department{}, err
	}
	for _, employeeID := range input.EmployeeIDs {
		result, e := tx.Exec(ctx, `INSERT INTO consignment_department_members(company_id,department_id,employee_id,assigned_by) SELECT $1,$2,id,$3 FROM employees WHERE company_id=$1 AND id=$4 AND status='active'`, p.CompanyID, id, p.UserID, employeeID)
		if e != nil {
			return Department{}, mapDBError(e)
		}
		if result.RowsAffected() != 1 {
			return Department{}, ErrInvalidInput
		}
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "consignment.department_members_updated", "consignment_department", id, map[string]any{"employee_ids": input.EmployeeIDs}); err != nil {
		return Department{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Department{}, err
	}
	item.Members, err = s.loadMembers(ctx, p.CompanyID, id)
	return item, err
}

func (s *Service) Create(ctx context.Context, p auth.Principal, input CreateInput) (Consignment, bool, error) {
	if err := s.authorize(ctx, p, "consignments.manage"); err != nil {
		return Consignment{}, false, err
	}
	normalizeCreate(&input)
	if !validCreate(input) {
		return Consignment{}, false, ErrInvalidInput
	}
	sort.Slice(input.Lines, func(i, j int) bool {
		if input.Lines[i].ProductID == input.Lines[j].ProductID {
			return input.Lines[i].DepartmentID < input.Lines[j].DepartmentID
		}
		return input.Lines[i].ProductID < input.Lines[j].ProductID
	})
	hash := hashJSON(input)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Consignment{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, p.CompanyID+":"+input.IdempotencyKey); err != nil {
		return Consignment{}, false, err
	}
	var id, oldHash string
	err = tx.QueryRow(ctx, `SELECT id,request_hash FROM consignments WHERE company_id=$1 AND idempotency_key=$2`, p.CompanyID, input.IdempotencyKey).Scan(&id, &oldHash)
	if err == nil {
		if oldHash != hash {
			return Consignment{}, false, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Consignment{}, false, err
		}
		item, e := s.load(ctx, s.db, p.CompanyID, id)
		return item, true, e
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Consignment{}, false, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO consignments(company_id,order_reference,dealer_reference,pouch_reference,source_type,source_reference,notes,created_by,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, p.CompanyID, input.OrderReference, input.DealerReference, input.PouchReference, input.SourceType, input.SourceReference, input.Notes, p.UserID, input.IdempotencyKey, hash).Scan(&id)
	if err != nil {
		return Consignment{}, false, mapDBError(err)
	}
	for _, line := range input.Lines {
		result, e := tx.Exec(ctx, `INSERT INTO consignment_lines(company_id,consignment_id,product_id,department_id,required_quantity,updated_by) SELECT $1,$2,p.id,d.id,$5,$6 FROM products p JOIN consignment_departments d ON d.company_id=p.company_id WHERE p.company_id=$1 AND p.id=$3 AND p.status='active' AND d.id=$4 AND d.status='active'`, p.CompanyID, id, line.ProductID, line.DepartmentID, line.RequiredQuantity, p.UserID)
		if e != nil {
			return Consignment{}, false, mapDBError(e)
		}
		if result.RowsAffected() != 1 {
			return Consignment{}, false, ErrInvalidInput
		}
	}
	metadata, _ := json.Marshal(map[string]any{"source_type": input.SourceType, "line_count": len(input.Lines)})
	if _, err = tx.Exec(ctx, `INSERT INTO consignment_events(company_id,consignment_id,event_type,actor_user_id,notes,metadata,idempotency_key,request_hash) VALUES($1,$2,'created',$3,$4,$5,$6,$7)`, p.CompanyID, id, p.UserID, input.Notes, metadata, input.IdempotencyKey, hash); err != nil {
		return Consignment{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "consignment.created", "consignment", id, map[string]any{"order_reference": input.OrderReference, "source_type": input.SourceType}); err != nil {
		return Consignment{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Consignment{}, false, err
	}
	item, err := s.load(ctx, s.db, p.CompanyID, id)
	return item, false, err
}

func (s *Service) List(ctx context.Context, p auth.Principal, f Filter) ([]Consignment, error) {
	broad, err := s.access(ctx, p)
	if err != nil {
		return nil, err
	}
	f.Status = strings.TrimSpace(f.Status)
	f.DepartmentID = strings.TrimSpace(f.DepartmentID)
	f.Query = strings.TrimSpace(f.Query)
	if f.Status != "" && !validStatuses[f.Status] || f.DepartmentID != "" && !uuidRE.MatchString(f.DepartmentID) || len(f.Query) > 200 {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT c.id FROM consignments c JOIN consignment_lines l ON l.company_id=c.company_id AND l.consignment_id=c.id WHERE c.company_id=$1 AND ($2='' OR c.status=$2) AND ($3='' OR l.department_id::text=$3) AND ($4='' OR c.order_reference ILIKE '%'||$4||'%' OR COALESCE(c.dealer_reference,'') ILIKE '%'||$4||'%' OR COALESCE(c.pouch_reference,'') ILIKE '%'||$4||'%') AND ($5 OR EXISTS(SELECT 1 FROM consignment_department_members dm JOIN employees e ON e.company_id=dm.company_id AND e.id=dm.employee_id WHERE dm.company_id=l.company_id AND dm.department_id=l.department_id AND e.user_id=$6)) ORDER BY c.id LIMIT 500`, p.CompanyID, f.Status, f.DepartmentID, f.Query, broad, p.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
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
	items := make([]Consignment, 0, len(ids))
	for _, id := range ids {
		item, e := s.load(ctx, s.db, p.CompanyID, id)
		if e != nil {
			return nil, e
		}
		if !broad {
			item.Lines = filterLines(item.Lines, p.UserID, s.db, ctx, p.CompanyID)
			item.Events = []Event{}
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Service) Get(ctx context.Context, p auth.Principal, id string) (Consignment, error) {
	broad, err := s.access(ctx, p)
	if err != nil {
		return Consignment{}, err
	}
	if !uuidRE.MatchString(id) {
		return Consignment{}, ErrInvalidInput
	}
	if !broad {
		var allowed bool
		err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM consignment_lines l JOIN consignment_department_members dm ON dm.company_id=l.company_id AND dm.department_id=l.department_id JOIN employees e ON e.company_id=dm.company_id AND e.id=dm.employee_id WHERE l.company_id=$1 AND l.consignment_id=$2 AND e.user_id=$3)`, p.CompanyID, id, p.UserID).Scan(&allowed)
		if err != nil {
			return Consignment{}, err
		}
		if !allowed {
			return Consignment{}, ErrNotFound
		}
	}
	item, err := s.load(ctx, s.db, p.CompanyID, id)
	if err == nil && !broad {
		item.Lines = filterLines(item.Lines, p.UserID, s.db, ctx, p.CompanyID)
		item.Events = []Event{}
	}
	return item, err
}

func (s *Service) Allocate(ctx context.Context, p auth.Principal, id string, input ActionInput) (Consignment, bool, error) {
	if err := s.authorize(ctx, p, "consignments.manage"); err != nil {
		return Consignment{}, false, err
	}
	if input.ExpectedVersion <= 0 {
		return Consignment{}, false, ErrInvalidInput
	}
	return s.action(ctx, p, id, "allocated", input.IdempotencyKey, input.ExpectedVersion, input.Notes, func(ctx context.Context, tx pgx.Tx, item *Consignment, eventID string) error {
		if item.Status != "created" {
			return ErrInvalidState
		}
		movements := aggregate(item.Lines)
		if err := s.inventory.ReserveConsignment(ctx, tx, p, id, movements); err != nil {
			return mapInventoryError(err)
		}
		_, err := tx.Exec(ctx, `UPDATE consignments SET status='allocated',allocated_by=$1,allocated_at=now(),version=version+1,updated_at=now() WHERE company_id=$2 AND id=$3`, p.UserID, p.CompanyID, id)
		return err
	})
}

func (s *Service) Transition(ctx context.Context, p auth.Principal, id string, input TransitionInput) (Consignment, bool, error) {
	if err := s.authorize(ctx, p, "consignments.manage"); err != nil {
		return Consignment{}, false, err
	}
	input.TargetStatus = strings.TrimSpace(input.TargetStatus)
	if input.ExpectedVersion <= 0 || !validStatuses[input.TargetStatus] {
		return Consignment{}, false, ErrInvalidInput
	}
	return s.action(ctx, p, id, "status:"+input.TargetStatus, input.IdempotencyKey, input.ExpectedVersion, input.Notes, func(ctx context.Context, tx pgx.Tx, item *Consignment, eventID string) error {
		if validTransitions[item.Status+":"+input.TargetStatus] == "" {
			return ErrInvalidState
		}
		if input.TargetStatus == "ready" {
			for _, line := range item.Lines {
				if line.ReadyQuantity != line.RequiredQuantity {
					return ErrIncomplete
				}
			}
		}
		if input.TargetStatus == "packed" {
			for _, line := range item.Lines {
				if line.PackedQuantity != line.RequiredQuantity {
					return ErrIncomplete
				}
			}
		}
		columns := ""
		args := []any{input.TargetStatus, p.CompanyID, id}
		if input.TargetStatus == "completed" {
			columns = ",completed_by=$4,completed_at=now()"
			args = append(args, p.UserID)
		}
		_, err := tx.Exec(ctx, `UPDATE consignments SET status=$1,version=version+1,updated_at=now()`+columns+` WHERE company_id=$2 AND id=$3`, args...)
		return err
	})
}

func (s *Service) UpdateProgress(ctx context.Context, p auth.Principal, id, lineID string, input ProgressInput) (Consignment, bool, error) {
	if err := s.authorize(ctx, p, "consignments.work"); err != nil {
		if e := s.authorize(ctx, p, "consignments.manage"); e != nil {
			return Consignment{}, false, err
		}
	}
	if !uuidRE.MatchString(lineID) || input.ReadyQuantity < 0 || input.PackedQuantity < 0 || input.ExpectedVersion <= 0 {
		return Consignment{}, false, ErrInvalidInput
	}
	operation := fmt.Sprintf("progress:%s:%d:%d:%d", lineID, input.ReadyQuantity, input.PackedQuantity, input.ExpectedVersion)
	return s.action(ctx, p, id, operation, input.IdempotencyKey, 0, input.Notes, func(ctx context.Context, tx pgx.Tx, item *Consignment, eventID string) error {
		if item.Status != "allocated" && item.Status != "picking" && item.Status != "ready" && item.Status != "packing" {
			return ErrInvalidState
		}
		var required, currentPacked int64
		var currentVersion int
		var departmentID string
		if err := tx.QueryRow(ctx, `SELECT required_quantity,packed_quantity,version,department_id FROM consignment_lines WHERE company_id=$1 AND consignment_id=$2 AND id=$3 FOR UPDATE`, p.CompanyID, id, lineID).Scan(&required, &currentPacked, &currentVersion, &departmentID); err != nil {
			return mapDBError(err)
		}
		if input.ExpectedVersion != currentVersion || input.PackedQuantity > input.ReadyQuantity || input.ReadyQuantity > required {
			return ErrConflict
		}
		if err := s.requireDepartmentWork(ctx, p, departmentID); err != nil {
			return err
		}
		if (item.Status == "allocated" || item.Status == "picking") && input.PackedQuantity != currentPacked {
			return ErrInvalidState
		}
		if (item.Status == "ready" || item.Status == "packing") && input.ReadyQuantity != required {
			return ErrInvalidState
		}
		_, err := tx.Exec(ctx, `UPDATE consignment_lines SET ready_quantity=$1,packed_quantity=$2,version=version+1,updated_by=$3,updated_at=now() WHERE company_id=$4 AND id=$5`, input.ReadyQuantity, input.PackedQuantity, p.UserID, p.CompanyID, lineID)
		return err
	})
}

func (s *Service) ConfirmOutbound(ctx context.Context, p auth.Principal, id string, input ActionInput) (Consignment, bool, error) {
	if err := s.authorize(ctx, p, "consignments.outbound"); err != nil {
		return Consignment{}, false, err
	}
	if input.ExpectedVersion <= 0 {
		return Consignment{}, false, ErrInvalidInput
	}
	return s.action(ctx, p, id, "outbound", input.IdempotencyKey, input.ExpectedVersion, input.Notes, func(ctx context.Context, tx pgx.Tx, item *Consignment, eventID string) error {
		if item.Status != "packed" {
			return ErrInvalidState
		}
		for _, line := range item.Lines {
			if line.PackedQuantity != line.RequiredQuantity {
				return ErrIncomplete
			}
		}
		if _, err := s.inventory.ConfirmConsignmentOutbound(ctx, tx, p, id, eventID); err != nil {
			return mapInventoryError(err)
		}
		_, err := tx.Exec(ctx, `UPDATE consignments SET status='outbound',outbound_by=$1,outbound_at=now(),version=version+1,updated_at=now() WHERE company_id=$2 AND id=$3`, p.UserID, p.CompanyID, id)
		return err
	})
}

func (s *Service) Cancel(ctx context.Context, p auth.Principal, id string, input ActionInput) (Consignment, bool, error) {
	if err := s.authorize(ctx, p, "consignments.manage"); err != nil {
		return Consignment{}, false, err
	}
	if input.ExpectedVersion <= 0 {
		return Consignment{}, false, ErrInvalidInput
	}
	return s.action(ctx, p, id, "cancelled", input.IdempotencyKey, input.ExpectedVersion, input.Notes, func(ctx context.Context, tx pgx.Tx, item *Consignment, eventID string) error {
		if item.Status == "outbound" || item.Status == "completed" || item.Status == "cancelled" {
			return ErrInvalidState
		}
		if item.Status != "created" {
			if err := s.inventory.ReleaseConsignment(ctx, tx, p, id, eventID, "Consignment cancelled"); err != nil {
				return mapInventoryError(err)
			}
		}
		_, err := tx.Exec(ctx, `UPDATE consignments SET status='cancelled',cancelled_by=$1,cancelled_at=now(),version=version+1,updated_at=now() WHERE company_id=$2 AND id=$3`, p.UserID, p.CompanyID, id)
		return err
	})
}

type actionFn func(context.Context, pgx.Tx, *Consignment, string) error

func (s *Service) action(ctx context.Context, p auth.Principal, id, eventType, key string, expectedVersion int, notes *string, fn actionFn) (Consignment, bool, error) {
	id = strings.TrimSpace(id)
	key = strings.TrimSpace(key)
	notes = trim(notes)
	if !uuidRE.MatchString(id) || key == "" || len(key) > 128 || eventType == "status:" || notes != nil && len(*notes) > 2000 {
		return Consignment{}, false, ErrInvalidInput
	}
	hash := hashJSON(struct {
		ID, Event string
		Expected  int
		Notes     *string
	}{id, eventType, expectedVersion, notes})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Consignment{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, p.CompanyID+":"+key); err != nil {
		return Consignment{}, false, err
	}
	var existingID, oldHash string
	err = tx.QueryRow(ctx, `SELECT consignment_id,request_hash FROM consignment_events WHERE company_id=$1 AND idempotency_key=$2`, p.CompanyID, key).Scan(&existingID, &oldHash)
	if err == nil {
		if existingID != id || oldHash != hash {
			return Consignment{}, false, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Consignment{}, false, err
		}
		item, e := s.load(ctx, s.db, p.CompanyID, id)
		return item, true, e
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Consignment{}, false, err
	}
	item, err := s.loadForUpdate(ctx, tx, p.CompanyID, id)
	if err != nil {
		return Consignment{}, false, err
	}
	if expectedVersion > 0 && item.Version != expectedVersion {
		return Consignment{}, false, ErrConflict
	}
	dbEventType := eventType
	if strings.HasPrefix(eventType, "status:") {
		dbEventType = "status_changed"
	}
	if eventType == "status:completed" {
		dbEventType = "completed"
	}
	if strings.HasPrefix(eventType, "progress:") {
		dbEventType = "line_progress"
	}
	metadata, _ := json.Marshal(map[string]any{"operation": eventType, "from_status": item.Status})
	var eventID string
	if err = tx.QueryRow(ctx, `INSERT INTO consignment_events(company_id,consignment_id,event_type,actor_user_id,notes,metadata,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, p.CompanyID, id, dbEventType, p.UserID, notes, metadata, key, hash).Scan(&eventID); err != nil {
		return Consignment{}, false, mapDBError(err)
	}
	if err = fn(ctx, tx, &item, eventID); err != nil {
		return Consignment{}, false, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "consignment."+strings.ReplaceAll(eventType, ":", "_"), "consignment", id, map[string]any{"event_id": eventID, "from_status": item.Status}); err != nil {
		return Consignment{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Consignment{}, false, mapDBError(err)
	}
	item, err = s.load(ctx, s.db, p.CompanyID, id)
	return item, false, err
}

func (s *Service) load(ctx context.Context, q querier, companyID, id string) (Consignment, error) {
	var item Consignment
	err := q.QueryRow(ctx, `SELECT id,order_reference,dealer_reference,pouch_reference,source_type,source_reference,status,notes,created_by,allocated_by,outbound_by,completed_by,cancelled_by,allocated_at,outbound_at,completed_at,cancelled_at,version,created_at,updated_at FROM consignments WHERE company_id=$1 AND id=$2`, companyID, id).Scan(&item.ID, &item.OrderReference, &item.DealerReference, &item.PouchReference, &item.SourceType, &item.SourceReference, &item.Status, &item.Notes, &item.CreatedBy, &item.AllocatedBy, &item.OutboundBy, &item.CompletedBy, &item.CancelledBy, &item.AllocatedAt, &item.OutboundAt, &item.CompletedAt, &item.CancelledAt, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Consignment{}, mapDBError(err)
	}
	item.Lines, err = loadLines(ctx, q, companyID, id)
	if err != nil {
		return Consignment{}, err
	}
	item.Events, err = loadEvents(ctx, q, companyID, id)
	return item, err
}
func (s *Service) loadForUpdate(ctx context.Context, tx pgx.Tx, companyID, id string) (Consignment, error) {
	var lock string
	if err := tx.QueryRow(ctx, `SELECT id FROM consignments WHERE company_id=$1 AND id=$2 FOR UPDATE`, companyID, id).Scan(&lock); err != nil {
		return Consignment{}, mapDBError(err)
	}
	return s.load(ctx, tx, companyID, id)
}

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadLines(ctx context.Context, q querier, companyID, id string) ([]Line, error) {
	rows, err := q.Query(ctx, `SELECT l.id,l.product_id,p.internal_code,p.name,l.department_id,d.name,l.required_quantity,l.ready_quantity,l.packed_quantity,CASE WHEN l.packed_quantity=l.required_quantity THEN 'packed' WHEN l.ready_quantity=l.required_quantity THEN 'ready' ELSE 'pending' END,l.version,l.updated_at FROM consignment_lines l JOIN products p ON p.company_id=l.company_id AND p.id=l.product_id JOIN consignment_departments d ON d.company_id=l.company_id AND d.id=l.department_id WHERE l.company_id=$1 AND l.consignment_id=$2 ORDER BY d.name,p.name,p.internal_code,l.id`, companyID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Line{}
	for rows.Next() {
		var x Line
		if err = rows.Scan(&x.ID, &x.ProductID, &x.InternalCode, &x.ProductName, &x.DepartmentID, &x.DepartmentName, &x.RequiredQuantity, &x.ReadyQuantity, &x.PackedQuantity, &x.Progress, &x.Version, &x.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}
func loadEvents(ctx context.Context, q querier, companyID, id string) ([]Event, error) {
	rows, err := q.Query(ctx, `SELECT id,event_type,actor_user_id,notes,metadata,created_at FROM consignment_events WHERE company_id=$1 AND consignment_id=$2 ORDER BY created_at,id`, companyID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Event{}
	for rows.Next() {
		var x Event
		if err = rows.Scan(&x.ID, &x.EventType, &x.ActorUserID, &x.Notes, &x.Metadata, &x.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}
func (s *Service) loadMembers(ctx context.Context, companyID, id string) ([]Member, error) {
	rows, err := s.db.Query(ctx, `SELECT e.id,e.display_name,e.user_id FROM consignment_department_members dm JOIN employees e ON e.company_id=dm.company_id AND e.id=dm.employee_id WHERE dm.company_id=$1 AND dm.department_id=$2 ORDER BY e.display_name,e.id`, companyID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Member{}
	for rows.Next() {
		var x Member
		if err = rows.Scan(&x.EmployeeID, &x.Name, &x.UserID); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}
func (s *Service) authorize(ctx context.Context, p auth.Principal, permission string) error {
	if err := s.authorizer.RequireModule(ctx, p, "consignments"); err != nil {
		return err
	}
	return s.authorizer.RequirePermission(ctx, p, permission)
}
func (s *Service) access(ctx context.Context, p auth.Principal) (bool, error) {
	if err := s.authorizer.RequireModule(ctx, p, "consignments"); err != nil {
		return false, err
	}
	if err := s.authorizer.RequirePermission(ctx, p, "consignments.view"); err == nil {
		return true, nil
	}
	if err := s.authorizer.RequirePermission(ctx, p, "consignments.manage"); err == nil {
		return true, nil
	}
	if err := s.authorizer.RequirePermission(ctx, p, "consignments.work"); err != nil {
		return false, err
	}
	return false, nil
}
func (s *Service) requireDepartmentWork(ctx context.Context, p auth.Principal, departmentID string) error {
	if err := s.authorizer.RequirePermission(ctx, p, "consignments.manage"); err == nil {
		return nil
	}
	if err := s.authorizer.RequirePermission(ctx, p, "consignments.work"); err != nil {
		return err
	}
	var allowed bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM consignment_department_members dm JOIN employees e ON e.company_id=dm.company_id AND e.id=dm.employee_id WHERE dm.company_id=$1 AND dm.department_id=$2 AND e.user_id=$3 AND e.status='active')`, p.CompanyID, departmentID, p.UserID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return authorization.ErrPermissionDenied
	}
	return nil
}
func normalizeCreate(i *CreateInput) {
	i.OrderReference = strings.TrimSpace(i.OrderReference)
	i.DealerReference = trim(i.DealerReference)
	i.PouchReference = trim(i.PouchReference)
	i.SourceType = strings.TrimSpace(i.SourceType)
	i.SourceReference = trim(i.SourceReference)
	i.Notes = trim(i.Notes)
	i.IdempotencyKey = strings.TrimSpace(i.IdempotencyKey)
	for n := range i.Lines {
		i.Lines[n].ProductID = strings.TrimSpace(i.Lines[n].ProductID)
		i.Lines[n].DepartmentID = strings.TrimSpace(i.Lines[n].DepartmentID)
	}
}
func validCreate(i CreateInput) bool {
	if i.OrderReference == "" || len(i.OrderReference) > 200 || (i.SourceType != "manual" && i.SourceType != "import") || i.SourceType == "import" && i.SourceReference == nil || i.IdempotencyKey == "" || len(i.IdempotencyKey) > 128 || len(i.Lines) == 0 || len(i.Lines) > 200 {
		return false
	}
	seen := map[string]bool{}
	for _, l := range i.Lines {
		k := l.ProductID + ":" + l.DepartmentID
		if !uuidRE.MatchString(l.ProductID) || !uuidRE.MatchString(l.DepartmentID) || l.RequiredQuantity <= 0 || seen[k] {
			return false
		}
		seen[k] = true
	}
	return true
}
func trim(v *string) *string {
	if v == nil {
		return nil
	}
	x := strings.TrimSpace(*v)
	if x == "" {
		return nil
	}
	return &x
}
func hashJSON(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func aggregate(lines []Line) []inventory.ConsignmentMovement {
	m := map[string]int64{}
	for _, l := range lines {
		m[l.ProductID] += l.RequiredQuantity
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]inventory.ConsignmentMovement, 0, len(keys))
	for _, k := range keys {
		out = append(out, inventory.ConsignmentMovement{ProductID: k, Quantity: m[k]})
	}
	return out
}
func filterLines(lines []Line, userID string, db *pgxpool.Pool, ctx context.Context, companyID string) []Line {
	out := []Line{}
	for _, l := range lines {
		var ok bool
		if db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM consignment_department_members dm JOIN employees e ON e.company_id=dm.company_id AND e.id=dm.employee_id WHERE dm.company_id=$1 AND dm.department_id=$2 AND e.user_id=$3)`, companyID, l.DepartmentID, userID).Scan(&ok) == nil && ok {
			out = append(out, l)
		}
	}
	return out
}
func mapDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var e *pgconn.PgError
	if errors.As(err, &e) {
		if e.Code == "23505" {
			return ErrConflict
		}
		if e.Code == "23503" || e.Code == "23514" || e.Code == "22P02" {
			return ErrInvalidInput
		}
	}
	return err
}

func mapInventoryError(err error) error {
	switch {
	case errors.Is(err, inventory.ErrInsufficientStock):
		return ErrInsufficient
	case errors.Is(err, inventory.ErrNotFound), errors.Is(err, inventory.ErrInvalidInput):
		return ErrInvalidInput
	case errors.Is(err, inventory.ErrConflict):
		return ErrConflict
	default:
		return err
	}
}
