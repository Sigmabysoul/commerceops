// This file coordinates the package's business rules and persistence operations behind a reusable API in the core company and user package.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/audit"
	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound     = errors.New("resource not found")
	ErrInvalidInput = errors.New("invalid input")
)

type Service struct {
	db         *pgxpool.Pool
	authorizer *authorization.Service
	audit      audit.Recorder
}

type Company struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Employee struct {
	ID          string    `json:"id"`
	UserID      *string   `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Permission struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

type Entitlement struct {
	ModuleKey string `json:"module_key"`
	Enabled   bool   `json:"enabled"`
}

type UserAccess struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Status string `json:"status"`
}

type AuditEntry struct {
	ID          string          `json:"id"`
	ActorUserID *string         `json:"actor_user_id"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    string          `json:"target_id"`
	Metadata    json.RawMessage `json:"metadata"`
	OccurredAt  time.Time       `json:"occurred_at"`
}

func NewService(db *pgxpool.Pool, authorizer *authorization.Service) *Service {
	return &Service{db: db, authorizer: authorizer}
}

func (s *Service) Company(ctx context.Context, principal auth.Principal) (Company, error) {
	var company Company
	err := s.db.QueryRow(ctx, `SELECT id, name, status, created_at, updated_at FROM companies WHERE id = $1`, principal.CompanyID).
		Scan(&company.ID, &company.Name, &company.Status, &company.CreatedAt, &company.UpdatedAt)
	return company, mapNotFound(err)
}

func (s *Service) ListEmployees(ctx context.Context, principal auth.Principal) ([]Employee, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "employees.view"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, display_name, status, created_at, updated_at
		FROM employees WHERE company_id = $1 ORDER BY display_name, id`, principal.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Employee, 0)
	for rows.Next() {
		var employee Employee
		if err := rows.Scan(&employee.ID, &employee.UserID, &employee.DisplayName, &employee.Status, &employee.CreatedAt, &employee.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, employee)
	}
	return result, rows.Err()
}

func (s *Service) CreateEmployee(ctx context.Context, principal auth.Principal, displayName string, userID *string) (Employee, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "employees.manage"); err != nil {
		return Employee{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return Employee{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Employee{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var employee Employee
	err = tx.QueryRow(ctx, `
		INSERT INTO employees (company_id, user_id, display_name)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, display_name, status, created_at, updated_at`, principal.CompanyID, userID, displayName,
	).Scan(&employee.ID, &employee.UserID, &employee.DisplayName, &employee.Status, &employee.CreatedAt, &employee.UpdatedAt)
	if err != nil {
		return Employee{}, err
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "employee.created", "employee", employee.ID, map[string]any{"display_name": employee.DisplayName}); err != nil {
		return Employee{}, err
	}
	return employee, tx.Commit(ctx)
}

func (s *Service) SetEmployeeStatus(ctx context.Context, principal auth.Principal, employeeID, status string) (Employee, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "employees.manage"); err != nil {
		return Employee{}, err
	}
	if status != "active" && status != "inactive" {
		return Employee{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Employee{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var employee Employee
	err = tx.QueryRow(ctx, `
		UPDATE employees SET status = $1, updated_at = now()
		WHERE company_id = $2 AND id = $3
		RETURNING id, user_id, display_name, status, created_at, updated_at`, status, principal.CompanyID, employeeID,
	).Scan(&employee.ID, &employee.UserID, &employee.DisplayName, &employee.Status, &employee.CreatedAt, &employee.UpdatedAt)
	if err != nil {
		return Employee{}, mapNotFound(err)
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "employee.status_changed", "employee", employee.ID, map[string]any{"status": status}); err != nil {
		return Employee{}, err
	}
	return employee, tx.Commit(ctx)
}

func (s *Service) SetUserAccessStatus(ctx context.Context, principal auth.Principal, userID, status string) error {
	if err := s.authorizer.RequirePermission(ctx, principal, "employees.manage"); err != nil {
		return err
	}
	if status != "active" && status != "disabled" {
		return ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	command, err := tx.Exec(ctx, `UPDATE company_users SET status = $1, updated_at = now() WHERE company_id = $2 AND user_id = $3`, status, principal.CompanyID, userID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	if status == "disabled" {
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE company_id = $1 AND user_id = $2 AND revoked_at IS NULL`, principal.CompanyID, userID); err != nil {
			return err
		}
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "user_access.status_changed", "user", userID, map[string]any{"status": status}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) CreateUserAccess(ctx context.Context, principal auth.Principal, email, password string) (UserAccess, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "employees.manage"); err != nil {
		return UserAccess{}, err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	passwordHash, err := auth.HashPassword(password)
	if err != nil || email == "" {
		return UserAccess{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return UserAccess{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var access UserAccess
	err = tx.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, email, status`, email, passwordHash).
		Scan(&access.UserID, &access.Email, &access.Status)
	if err != nil {
		return UserAccess{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO company_users (company_id, user_id) VALUES ($1, $2)`, principal.CompanyID, access.UserID); err != nil {
		return UserAccess{}, err
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "user_access.created", "user", access.UserID, map[string]any{"email": access.Email}); err != nil {
		return UserAccess{}, err
	}
	return access, tx.Commit(ctx)
}

func (s *Service) SetUserRoles(ctx context.Context, principal auth.Principal, userID string, roleIDs []string) error {
	if err := s.authorizer.RequirePermission(ctx, principal, "roles.manage"); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var accessExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM company_users WHERE company_id = $1 AND user_id = $2)`, principal.CompanyID, userID).Scan(&accessExists); err != nil {
		return err
	}
	if !accessExists {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM company_user_roles WHERE company_id = $1 AND user_id = $2`, principal.CompanyID, userID); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(roleIDs))
	for _, roleID := range roleIDs {
		if _, duplicate := seen[roleID]; duplicate {
			continue
		}
		seen[roleID] = struct{}{}
		if _, err := tx.Exec(ctx, `INSERT INTO company_user_roles (company_id, user_id, role_id) VALUES ($1, $2, $3)`, principal.CompanyID, userID, roleID); err != nil {
			return err
		}
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "user_access.roles_changed", "user", userID, map[string]any{"role_ids": roleIDs}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ListPermissions(ctx context.Context, principal auth.Principal) ([]Permission, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "roles.view"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT key, description FROM permissions ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Permission, 0)
	for rows.Next() {
		var permission Permission
		if err := rows.Scan(&permission.Key, &permission.Description); err != nil {
			return nil, err
		}
		result = append(result, permission)
	}
	return result, rows.Err()
}

func (s *Service) ListRoles(ctx context.Context, principal auth.Principal) ([]Role, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "roles.view"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT r.id, r.name, r.status, r.created_at, r.updated_at,
		       COALESCE(array_agg(rp.permission_key ORDER BY rp.permission_key) FILTER (WHERE rp.permission_key IS NOT NULL), '{}')
		FROM roles r LEFT JOIN role_permissions rp ON rp.company_id = r.company_id AND rp.role_id = r.id
		WHERE r.company_id = $1
		GROUP BY r.id ORDER BY r.name, r.id`, principal.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Role, 0)
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Status, &role.CreatedAt, &role.UpdatedAt, &role.Permissions); err != nil {
			return nil, err
		}
		result = append(result, role)
	}
	return result, rows.Err()
}

func (s *Service) CreateRole(ctx context.Context, principal auth.Principal, name string) (Role, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "roles.manage"); err != nil {
		return Role{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Role{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var role Role
	err = tx.QueryRow(ctx, `INSERT INTO roles (company_id, name) VALUES ($1, $2) RETURNING id, name, status, created_at, updated_at`, principal.CompanyID, name).
		Scan(&role.ID, &role.Name, &role.Status, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		return Role{}, err
	}
	role.Permissions = []string{}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "role.created", "role", role.ID, map[string]any{"name": role.Name}); err != nil {
		return Role{}, err
	}
	return role, tx.Commit(ctx)
}

func (s *Service) SetRolePermissions(ctx context.Context, principal auth.Principal, roleID string, permissions []string) error {
	if err := s.authorizer.RequirePermission(ctx, principal, "roles.manage"); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	command, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE company_id = $1 AND role_id = $2`, principal.CompanyID, roleID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM roles WHERE company_id = $1 AND id = $2)`, principal.CompanyID, roleID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	seen := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		if _, duplicate := seen[permission]; duplicate {
			continue
		}
		seen[permission] = struct{}{}
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (company_id, role_id, permission_key) VALUES ($1, $2, $3)`, principal.CompanyID, roleID, permission); err != nil {
			return err
		}
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "role.permissions_changed", "role", roleID, map[string]any{"permissions": permissions}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ListEntitlements(ctx context.Context, principal auth.Principal) ([]Entitlement, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "settings.manage"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT module_key, enabled FROM module_entitlements WHERE company_id = $1 ORDER BY module_key`, principal.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Entitlement{{ModuleKey: "core", Enabled: true}}
	for rows.Next() {
		var entitlement Entitlement
		if err := rows.Scan(&entitlement.ModuleKey, &entitlement.Enabled); err != nil {
			return nil, err
		}
		result = append(result, entitlement)
	}
	return result, rows.Err()
}

func (s *Service) SetEntitlement(ctx context.Context, principal auth.Principal, module string, enabled bool) error {
	if err := s.authorizer.RequirePermission(ctx, principal, "settings.manage"); err != nil {
		return err
	}
	module = strings.TrimSpace(module)
	if module == "" || module == "core" {
		return ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	_, err = tx.Exec(ctx, `
		INSERT INTO module_entitlements (company_id, module_key, enabled) VALUES ($1, $2, $3)
		ON CONFLICT (company_id, module_key) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = now()`, principal.CompanyID, module, enabled)
	if err != nil {
		return err
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "module_entitlement.changed", "module", module, map[string]any{"enabled": enabled}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ListAudit(ctx context.Context, principal auth.Principal) ([]AuditEntry, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "settings.manage"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, actor_user_id, action, target_type, target_id, metadata, occurred_at
		FROM audit_logs WHERE company_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 200`, principal.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AuditEntry, 0)
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(&entry.ID, &entry.ActorUserID, &entry.Action, &entry.TargetType, &entry.TargetID, &entry.Metadata, &entry.OccurredAt); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
