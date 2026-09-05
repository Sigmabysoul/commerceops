// This file coordinates the package's business rules and persistence operations behind a reusable API in the authorization package.
package authorization

import (
	"context"
	"errors"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPermissionDenied  = errors.New("permission denied")
	ErrModuleUnavailable = errors.New("module unavailable")
)

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

func (s *Service) RequirePermission(ctx context.Context, principal auth.Principal, permission string) error {
	var allowed bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM company_user_roles cur
			JOIN roles r ON r.company_id = cur.company_id AND r.id = cur.role_id
			JOIN role_permissions rp ON rp.company_id = r.company_id AND rp.role_id = r.id
			WHERE cur.company_id = $1 AND cur.user_id = $2
			  AND r.status = 'active' AND rp.permission_key = $3
		)`, principal.CompanyID, principal.UserID, permission,
	).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}

func (s *Service) RequireModule(ctx context.Context, principal auth.Principal, module string) error {
	if module == "core" {
		return nil
	}
	var allowed bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM module_entitlements
			WHERE company_id = $1 AND module_key = $2 AND enabled
		)`, principal.CompanyID, module,
	).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrModuleUnavailable
	}
	return nil
}
