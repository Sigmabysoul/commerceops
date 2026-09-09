// This file coordinates the package's business rules and persistence operations behind a reusable API in the authentication package.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveAccess     = errors.New("inactive user or company access")
	ErrAmbiguousAccess    = errors.New("multiple active company accesses require an operating-company policy")
	ErrInvalidSession     = errors.New("invalid session")
)

type Principal struct {
	SessionID string `json:"-"`
	UserID    string `json:"user_id"`
	CompanyID string `json:"company_id"`
	Email     string `json:"email"`
}

type Service struct {
	db       *pgxpool.Pool
	lifetime time.Duration
	now      func() time.Time
}

func NewService(db *pgxpool.Pool, lifetime time.Duration) *Service {
	return &Service{db: db, lifetime: lifetime, now: time.Now}
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", errors.New("password must be between 12 and 1024 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func (s *Service) Login(ctx context.Context, email, password string) (string, Principal, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return "", Principal{}, ErrInvalidCredentials
	}

	var principal Principal
	var passwordHash, userStatus string
	err := s.db.QueryRow(ctx, `
		SELECT u.id, u.email, u.password_hash, u.status
		FROM users u
		WHERE u.email = $1`, email,
	).Scan(&principal.UserID, &principal.Email, &passwordHash, &userStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Principal{}, ErrInvalidCredentials
	}
	if err != nil {
		return "", Principal{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return "", Principal{}, ErrInvalidCredentials
	}
	if userStatus != "active" {
		return "", Principal{}, ErrInactiveAccess
	}
	rows, err := s.db.Query(ctx, `
		SELECT c.id
		FROM company_users cu
		JOIN companies c ON c.id = cu.company_id
		WHERE cu.user_id = $1 AND cu.status = 'active' AND c.status = 'active'
		ORDER BY c.id`, principal.UserID)
	if err != nil {
		return "", Principal{}, err
	}
	defer rows.Close()
	companyIDs := []string{}
	for rows.Next() {
		var companyID string
		if err = rows.Scan(&companyID); err != nil {
			return "", Principal{}, err
		}
		companyIDs = append(companyIDs, companyID)
	}
	if err = rows.Err(); err != nil {
		return "", Principal{}, err
	}
	switch len(companyIDs) {
	case 0:
		return "", Principal{}, ErrInactiveAccess
	case 1:
		principal.CompanyID = companyIDs[0]
	default:
		return "", Principal{}, ErrAmbiguousAccess
	}

	token, tokenHash, err := newToken()
	if err != nil {
		return "", Principal{}, err
	}
	expiresAt := s.now().Add(s.lifetime)
	err = s.db.QueryRow(ctx, `
		INSERT INTO sessions (user_id, company_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id`, principal.UserID, principal.CompanyID, tokenHash, expiresAt,
	).Scan(&principal.SessionID)
	if err != nil {
		return "", Principal{}, err
	}
	return token, principal, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrInvalidSession
	}
	hash := sha256.Sum256([]byte(token))
	var principal Principal
	err := s.db.QueryRow(ctx, `
		SELECT s.id, u.id, s.company_id, u.email
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN company_users cu ON cu.company_id = s.company_id AND cu.user_id = s.user_id
		JOIN companies c ON c.id = s.company_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > $2
		  AND u.status = 'active'
		  AND cu.status = 'active'
		  AND c.status = 'active'`, hash[:], s.now(),
	).Scan(&principal.SessionID, &principal.UserID, &principal.CompanyID, &principal.Email)
	if err != nil {
		return Principal{}, ErrInvalidSession
	}
	return principal, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	hash := sha256.Sum256([]byte(token))
	_, err := s.db.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, hash[:])
	return err
}

func newToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}
