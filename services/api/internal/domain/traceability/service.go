// This file owns Trace Box identity, contents, custody, and immutable history without owning inventory.
package traceability

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/audit"
	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/authorization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound     = errors.New("trace box or related record not found")
	ErrInvalidInput = errors.New("invalid traceability input")
	ErrConflict     = errors.New("traceability request conflicts with an existing request")
	ErrQuantity     = errors.New("trace box quantity is insufficient")
	uuidRE          = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	identifierRE    = regexp.MustCompile(`^TBX_[A-Z2-7]{26}$`)
)

type Service struct {
	db         *pgxpool.Pool
	authorizer *authorization.Service
	audit      audit.Recorder
}

type TraceBox struct {
	ID               string    `json:"id"`
	OpaqueIdentifier string    `json:"opaque_identifier"`
	Label            *string   `json:"label"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	Contents         []Content `json:"contents"`
	CurrentCustody   *Custody  `json:"current_custody"`
	Workflow         Workflow  `json:"workflow"`
	Events           []Event   `json:"events"`
}

type Content struct {
	ProductID    string `json:"product_id"`
	InternalCode string `json:"internal_code"`
	ProductName  string `json:"product_name"`
	Quantity     int64  `json:"quantity"`
}

type Custody struct {
	EventID       string    `json:"event_id"`
	EmployeeID    *string   `json:"employee_id"`
	DepartmentID  *string   `json:"department_id"`
	CustodianName string    `json:"custodian_name"`
	ActorUserID   string    `json:"actor_user_id"`
	TransferredAt time.Time `json:"transferred_at"`
}

type Event struct {
	ID             string          `json:"id"`
	EventType      string          `json:"event_type"`
	ActorUserID    string          `json:"actor_user_id"`
	Notes          *string         `json:"notes"`
	Metadata       json.RawMessage `json:"metadata"`
	IdempotencyKey string          `json:"idempotency_key"`
	CreatedAt      time.Time       `json:"created_at"`
}

type CreateInput struct {
	Label          *string `json:"label"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type ContentInput struct {
	ProductID      string  `json:"product_id"`
	Quantity       int64   `json:"quantity"`
	Notes          *string `json:"notes"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type CustodyInput struct {
	EmployeeID     *string `json:"employee_id"`
	DepartmentID   *string `json:"department_id"`
	Notes          *string `json:"notes"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type Options struct {
	Products    []ProductOption    `json:"products"`
	Employees   []EmployeeOption   `json:"employees"`
	Departments []DepartmentOption `json:"departments"`
}

type ProductOption struct {
	ID           string `json:"id"`
	InternalCode string `json:"internal_code"`
	Name         string `json:"name"`
}

type EmployeeOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DepartmentOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewService(db *pgxpool.Pool, authorizer *authorization.Service) *Service {
	return &Service{db: db, authorizer: authorizer}
}

func (s *Service) List(ctx context.Context, principal auth.Principal) ([]TraceBox, error) {
	if err := s.authorize(ctx, principal, "traceability.view"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,opaque_identifier,label,created_by,created_at FROM trace_boxes WHERE company_id=$1 ORDER BY created_at DESC,id DESC LIMIT 200`, principal.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TraceBox, 0)
	for rows.Next() {
		var item TraceBox
		if err = rows.Scan(&item.ID, &item.OpaqueIdentifier, &item.Label, &item.CreatedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Contents = []Content{}
		item.Events = []Event{}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Options(ctx context.Context, principal auth.Principal) (Options, error) {
	if err := s.authorize(ctx, principal, "traceability.view"); err != nil {
		return Options{}, err
	}
	result := Options{Products: []ProductOption{}, Employees: []EmployeeOption{}, Departments: []DepartmentOption{}}
	productRows, err := s.db.Query(ctx, `SELECT id,internal_code,name FROM products WHERE company_id=$1 AND status='active' ORDER BY internal_code,id`, principal.CompanyID)
	if err != nil {
		return Options{}, err
	}
	for productRows.Next() {
		var item ProductOption
		if err = productRows.Scan(&item.ID, &item.InternalCode, &item.Name); err != nil {
			productRows.Close()
			return Options{}, err
		}
		result.Products = append(result.Products, item)
	}
	if err = productRows.Err(); err != nil {
		productRows.Close()
		return Options{}, err
	}
	productRows.Close()
	employeeRows, err := s.db.Query(ctx, `SELECT id,display_name FROM employees WHERE company_id=$1 AND status='active' ORDER BY display_name,id`, principal.CompanyID)
	if err != nil {
		return Options{}, err
	}
	for employeeRows.Next() {
		var item EmployeeOption
		if err = employeeRows.Scan(&item.ID, &item.Name); err != nil {
			employeeRows.Close()
			return Options{}, err
		}
		result.Employees = append(result.Employees, item)
	}
	if err = employeeRows.Err(); err != nil {
		employeeRows.Close()
		return Options{}, err
	}
	employeeRows.Close()
	departmentRows, err := s.db.Query(ctx, `SELECT id,name FROM consignment_departments WHERE company_id=$1 AND status='active' ORDER BY name,id`, principal.CompanyID)
	if err != nil {
		return Options{}, err
	}
	defer departmentRows.Close()
	for departmentRows.Next() {
		var item DepartmentOption
		if err = departmentRows.Scan(&item.ID, &item.Name); err != nil {
			return Options{}, err
		}
		result.Departments = append(result.Departments, item)
	}
	return result, departmentRows.Err()
}

func (s *Service) Get(ctx context.Context, principal auth.Principal, id string) (TraceBox, error) {
	if err := s.authorize(ctx, principal, "traceability.view"); err != nil {
		return TraceBox{}, err
	}
	if !uuidRE.MatchString(strings.TrimSpace(id)) {
		return TraceBox{}, ErrInvalidInput
	}
	return s.load(ctx, principal.CompanyID, id)
}

func (s *Service) Resolve(ctx context.Context, principal auth.Principal, identifier string) (TraceBox, error) {
	if err := s.authorize(ctx, principal, "traceability.view"); err != nil {
		return TraceBox{}, err
	}
	identifier = strings.ToUpper(strings.TrimSpace(identifier))
	if !identifierRE.MatchString(identifier) {
		return TraceBox{}, ErrInvalidInput
	}
	var id string
	if err := s.db.QueryRow(ctx, `SELECT id FROM trace_boxes WHERE company_id=$1 AND opaque_identifier=$2`, principal.CompanyID, identifier).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return TraceBox{}, ErrNotFound
	} else if err != nil {
		return TraceBox{}, err
	}
	return s.load(ctx, principal.CompanyID, id)
}

func (s *Service) Create(ctx context.Context, principal auth.Principal, input CreateInput) (TraceBox, bool, error) {
	if err := s.authorize(ctx, principal, "traceability.manage"); err != nil {
		return TraceBox{}, false, err
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Label = cleanOptional(input.Label)
	if !validKey(input.IdempotencyKey) || input.Label != nil && len(*input.Label) > 200 {
		return TraceBox{}, false, ErrInvalidInput
	}
	hash := requestHash(input)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TraceBox{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err = lockRequest(ctx, tx, principal.CompanyID, input.IdempotencyKey); err != nil {
		return TraceBox{}, false, err
	}
	var id, oldHash string
	err = tx.QueryRow(ctx, `SELECT id,request_hash FROM trace_boxes WHERE company_id=$1 AND idempotency_key=$2`, principal.CompanyID, input.IdempotencyKey).Scan(&id, &oldHash)
	if err == nil {
		if oldHash != hash {
			return TraceBox{}, false, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return TraceBox{}, false, err
		}
		item, err := s.load(ctx, principal.CompanyID, id)
		return item, true, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return TraceBox{}, false, err
	}
	identifier, err := opaqueIdentifier()
	if err != nil {
		return TraceBox{}, false, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO trace_boxes(company_id,opaque_identifier,label,created_by,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, principal.CompanyID, identifier, input.Label, principal.UserID, input.IdempotencyKey, hash).Scan(&id); err != nil {
		return TraceBox{}, false, mapDBError(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO trace_box_events(company_id,trace_box_id,event_type,actor_user_id,metadata,idempotency_key,request_hash) VALUES($1,$2,'box_created',$3,jsonb_build_object('opaque_identifier',$4::text),$5,$6)`, principal.CompanyID, id, principal.UserID, identifier, input.IdempotencyKey, hash); err != nil {
		return TraceBox{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "trace_box.created", "trace_box", id, map[string]any{"opaque_identifier": identifier}); err != nil {
		return TraceBox{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return TraceBox{}, false, err
	}
	item, err := s.load(ctx, principal.CompanyID, id)
	return item, false, err
}

func (s *Service) AddContent(ctx context.Context, principal auth.Principal, boxID string, input ContentInput) (TraceBox, bool, error) {
	return s.changeContent(ctx, principal, boxID, input, "content_added")
}

func (s *Service) RemoveContent(ctx context.Context, principal auth.Principal, boxID string, input ContentInput) (TraceBox, bool, error) {
	return s.changeContent(ctx, principal, boxID, input, "content_removed")
}

func (s *Service) changeContent(ctx context.Context, principal auth.Principal, boxID string, input ContentInput, eventType string) (TraceBox, bool, error) {
	if err := s.authorize(ctx, principal, "traceability.manage"); err != nil {
		return TraceBox{}, false, err
	}
	boxID, input.ProductID, input.IdempotencyKey = strings.TrimSpace(boxID), strings.TrimSpace(input.ProductID), strings.TrimSpace(input.IdempotencyKey)
	input.Notes = cleanOptional(input.Notes)
	if !uuidRE.MatchString(boxID) || !uuidRE.MatchString(input.ProductID) || input.Quantity <= 0 || !validKey(input.IdempotencyKey) || input.Notes != nil && len(*input.Notes) > 2000 {
		return TraceBox{}, false, ErrInvalidInput
	}
	hash := requestHash(struct {
		BoxID, EventType string
		Input            ContentInput
	}{boxID, eventType, input})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TraceBox{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	replayed, err := existingEvent(ctx, tx, principal.CompanyID, boxID, eventType, input.IdempotencyKey, hash)
	if err != nil || replayed {
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			return TraceBox{}, false, err
		}
		item, loadErr := s.load(ctx, principal.CompanyID, boxID)
		return item, true, loadErr
	}
	if err = lockBox(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	if err = assertNoPendingHandover(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	var productExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE company_id=$1 AND id=$2)`, principal.CompanyID, input.ProductID).Scan(&productExists); err != nil {
		return TraceBox{}, false, err
	}
	if !productExists {
		return TraceBox{}, false, ErrNotFound
	}
	if eventType == "content_removed" {
		var available int64
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity_delta),0) FROM trace_box_content_changes WHERE company_id=$1 AND trace_box_id=$2 AND product_id=$3`, principal.CompanyID, boxID, input.ProductID).Scan(&available); err != nil {
			return TraceBox{}, false, err
		}
		if input.Quantity > available {
			return TraceBox{}, false, ErrQuantity
		}
	}
	metadata := map[string]any{"product_id": input.ProductID, "quantity": input.Quantity}
	eventID, err := insertEvent(ctx, tx, principal, boxID, eventType, input.Notes, input.IdempotencyKey, hash, metadata)
	if err != nil {
		return TraceBox{}, false, err
	}
	delta := input.Quantity
	if eventType == "content_removed" {
		delta = -delta
	}
	if _, err = tx.Exec(ctx, `INSERT INTO trace_box_content_changes(company_id,event_id,event_type,trace_box_id,product_id,quantity_delta) VALUES($1,$2,$3,$4,$5,$6)`, principal.CompanyID, eventID, eventType, boxID, input.ProductID, delta); err != nil {
		return TraceBox{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "trace_box."+eventType, "trace_box", boxID, metadata); err != nil {
		return TraceBox{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return TraceBox{}, false, err
	}
	item, err := s.load(ctx, principal.CompanyID, boxID)
	return item, false, err
}

func (s *Service) TransferCustody(ctx context.Context, principal auth.Principal, boxID string, input CustodyInput) (TraceBox, bool, error) {
	if err := s.authorize(ctx, principal, "traceability.manage"); err != nil {
		return TraceBox{}, false, err
	}
	boxID, input.IdempotencyKey = strings.TrimSpace(boxID), strings.TrimSpace(input.IdempotencyKey)
	input.EmployeeID, input.DepartmentID, input.Notes = cleanOptional(input.EmployeeID), cleanOptional(input.DepartmentID), cleanOptional(input.Notes)
	if !uuidRE.MatchString(boxID) || (input.EmployeeID == nil) == (input.DepartmentID == nil) || input.EmployeeID != nil && !uuidRE.MatchString(*input.EmployeeID) || input.DepartmentID != nil && !uuidRE.MatchString(*input.DepartmentID) || !validKey(input.IdempotencyKey) || input.Notes != nil && len(*input.Notes) > 2000 {
		return TraceBox{}, false, ErrInvalidInput
	}
	hash := requestHash(struct {
		BoxID string
		Input CustodyInput
	}{boxID, input})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TraceBox{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	replayed, err := existingEvent(ctx, tx, principal.CompanyID, boxID, "custody_transferred", input.IdempotencyKey, hash)
	if err != nil || replayed {
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			return TraceBox{}, false, err
		}
		item, loadErr := s.load(ctx, principal.CompanyID, boxID)
		return item, true, loadErr
	}
	if err = lockBox(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	if err = assertNoPendingHandover(ctx, tx, principal.CompanyID, boxID); err != nil {
		return TraceBox{}, false, err
	}
	metadata := map[string]any{}
	if input.EmployeeID != nil {
		metadata["employee_id"] = *input.EmployeeID
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM employees WHERE company_id=$1 AND id=$2 AND status='active')`, principal.CompanyID, *input.EmployeeID).Scan(&exists); err != nil || !exists {
			if err == nil {
				err = ErrNotFound
			}
			return TraceBox{}, false, err
		}
	} else {
		metadata["department_id"] = *input.DepartmentID
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM consignment_departments WHERE company_id=$1 AND id=$2 AND status='active')`, principal.CompanyID, *input.DepartmentID).Scan(&exists); err != nil || !exists {
			if err == nil {
				err = ErrNotFound
			}
			return TraceBox{}, false, err
		}
	}
	eventID, err := insertEvent(ctx, tx, principal, boxID, "custody_transferred", input.Notes, input.IdempotencyKey, hash, metadata)
	if err != nil {
		return TraceBox{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO trace_box_custody_changes(company_id,event_id,trace_box_id,employee_id,department_id) VALUES($1,$2,$3,$4,$5)`, principal.CompanyID, eventID, boxID, input.EmployeeID, input.DepartmentID); err != nil {
		return TraceBox{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "trace_box.custody_transferred", "trace_box", boxID, metadata); err != nil {
		return TraceBox{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return TraceBox{}, false, err
	}
	item, err := s.load(ctx, principal.CompanyID, boxID)
	return item, false, err
}

func (s *Service) load(ctx context.Context, companyID, id string) (TraceBox, error) {
	var item TraceBox
	if err := s.db.QueryRow(ctx, `SELECT id,opaque_identifier,label,created_by,created_at FROM trace_boxes WHERE company_id=$1 AND id=$2`, companyID, id).Scan(&item.ID, &item.OpaqueIdentifier, &item.Label, &item.CreatedBy, &item.CreatedAt); errors.Is(err, pgx.ErrNoRows) {
		return TraceBox{}, ErrNotFound
	} else if err != nil {
		return TraceBox{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT c.product_id,p.internal_code,p.name,sum(c.quantity_delta) FROM trace_box_content_changes c JOIN products p ON p.company_id=c.company_id AND p.id=c.product_id WHERE c.company_id=$1 AND c.trace_box_id=$2 GROUP BY c.product_id,p.internal_code,p.name HAVING sum(c.quantity_delta)>0 ORDER BY p.internal_code,c.product_id`, companyID, id)
	if err != nil {
		return TraceBox{}, err
	}
	item.Contents = make([]Content, 0)
	for rows.Next() {
		var content Content
		if err = rows.Scan(&content.ProductID, &content.InternalCode, &content.ProductName, &content.Quantity); err != nil {
			rows.Close()
			return TraceBox{}, err
		}
		item.Contents = append(item.Contents, content)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return TraceBox{}, err
	}
	rows.Close()
	var custody Custody
	err = s.db.QueryRow(ctx, `SELECT c.event_id,c.employee_id,c.department_id,COALESCE(e.display_name,d.name),v.actor_user_id,v.created_at FROM trace_box_custody_changes c JOIN trace_box_events v ON v.company_id=c.company_id AND v.id=c.event_id LEFT JOIN employees e ON e.company_id=c.company_id AND e.id=c.employee_id LEFT JOIN consignment_departments d ON d.company_id=c.company_id AND d.id=c.department_id WHERE c.company_id=$1 AND c.trace_box_id=$2 ORDER BY v.created_at DESC,v.id DESC LIMIT 1`, companyID, id).Scan(&custody.EventID, &custody.EmployeeID, &custody.DepartmentID, &custody.CustodianName, &custody.ActorUserID, &custody.TransferredAt)
	if err == nil {
		item.CurrentCustody = &custody
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return TraceBox{}, err
	}
	item.Workflow, err = s.loadWorkflow(ctx, companyID, id, len(item.Contents) > 0)
	if err != nil {
		return TraceBox{}, err
	}
	rows, err = s.db.Query(ctx, `SELECT id,event_type,actor_user_id,notes,metadata,idempotency_key,created_at FROM trace_box_events WHERE company_id=$1 AND trace_box_id=$2 ORDER BY created_at,id`, companyID, id)
	if err != nil {
		return TraceBox{}, err
	}
	defer rows.Close()
	item.Events = make([]Event, 0)
	for rows.Next() {
		var event Event
		if err = rows.Scan(&event.ID, &event.EventType, &event.ActorUserID, &event.Notes, &event.Metadata, &event.IdempotencyKey, &event.CreatedAt); err != nil {
			return TraceBox{}, err
		}
		item.Events = append(item.Events, event)
	}
	return item, rows.Err()
}

func (s *Service) authorize(ctx context.Context, principal auth.Principal, permission string) error {
	if err := s.authorizer.RequireModule(ctx, principal, "traceability"); err != nil {
		return err
	}
	return s.authorizer.RequirePermission(ctx, principal, permission)
}

func existingEvent(ctx context.Context, tx pgx.Tx, companyID, boxID, eventType, key, hash string) (bool, error) {
	if err := lockRequest(ctx, tx, companyID, key); err != nil {
		return false, err
	}
	var existingBox, existingType, existingHash string
	err := tx.QueryRow(ctx, `SELECT trace_box_id,event_type,request_hash FROM trace_box_events WHERE company_id=$1 AND idempotency_key=$2`, companyID, key).Scan(&existingBox, &existingType, &existingHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if existingBox != boxID || existingType != eventType || existingHash != hash {
		return false, ErrConflict
	}
	return true, nil
}

func insertEvent(ctx context.Context, tx pgx.Tx, principal auth.Principal, boxID, eventType string, notes *string, key, hash string, metadata map[string]any) (string, error) {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO trace_box_events(company_id,trace_box_id,event_type,actor_user_id,notes,metadata,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, principal.CompanyID, boxID, eventType, principal.UserID, notes, encoded, key, hash).Scan(&id)
	return id, mapDBError(err)
}

func lockBox(ctx context.Context, tx pgx.Tx, companyID, id string) error {
	var found string
	if err := tx.QueryRow(ctx, `SELECT id FROM trace_boxes WHERE company_id=$1 AND id=$2 FOR UPDATE`, companyID, id).Scan(&found); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else {
		return err
	}
}

func lockRequest(ctx context.Context, tx pgx.Tx, companyID, key string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, companyID+":"+key)
	return err
}

func opaqueIdentifier() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "TBX_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value), nil
}

func requestHash(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func validKey(value string) bool { return value != "" && len(value) <= 128 }

func cleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	clean := strings.TrimSpace(*value)
	if clean == "" {
		return nil
	}
	return &clean
}

func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}
