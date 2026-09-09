package marketplaceaccount

import (
	"errors"
	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
	"log/slog"
	"net/http"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(s *Service) *HTTPHandler { return &HTTPHandler{service: s} }
func (h *HTTPHandler) Register(mux *http.ServeMux, session func(http.Handler) http.Handler) {
	for path, handler := range map[string]http.HandlerFunc{"/api/v1/business-identities": h.Identities, "/api/v1/business-identities/{identity_id}": h.Identity, "/api/v1/marketplace-accounts": h.Accounts, "/api/v1/marketplace-accounts/{account_id}": h.Account} {
		mux.Handle(path, session(handler))
	}
}
func principal(r *http.Request) auth.Principal {
	p, _ := auth.PrincipalFromContext(r.Context())
	return p
}
func respond(w http.ResponseWriter, key string, v any, err error) {
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, 200, map[string]any{key: v})
}
func method(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method == want {
		return true
	}
	httpserver.WriteError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
	return false
}
func writeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, authorization.ErrPermissionDenied):
		httpserver.WriteError(w, 403, "FORBIDDEN", "Permission denied")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, 404, "NOT_FOUND", "Account or identity not found")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, 400, "INVALID_REQUEST", "Account request is invalid; metadata supports only region and notes, never credentials")
	case errors.Is(err, ErrConflict), errors.Is(err, ErrUnavailable):
		httpserver.WriteError(w, 409, "CONFLICT", "Account identity or state conflicts with this operation")
	default:
		slog.Error("marketplace account request failed", "error", err)
		httpserver.WriteError(w, 500, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}
func (h *HTTPHandler) Identities(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, e := h.service.Identities(r.Context(), principal(r))
		respond(w, "business_identities", v, e)
		return
	}
	if !method(w, r, "POST") {
		return
	}
	h.saveIdentity(w, r, "")
}
func (h *HTTPHandler) Identity(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "PUT") {
		return
	}
	h.saveIdentity(w, r, r.PathValue("identity_id"))
}
func (h *HTTPHandler) saveIdentity(w http.ResponseWriter, r *http.Request, id string) {
	var in IdentityInput
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	v, e := h.service.SaveIdentity(r.Context(), principal(r), id, in)
	respond(w, "business_identity", v, e)
}
func (h *HTTPHandler) Accounts(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, e := h.service.Accounts(r.Context(), principal(r), r.URL.Query().Get("marketplace"), r.URL.Query().Get("status"))
		respond(w, "marketplace_accounts", v, e)
		return
	}
	if !method(w, r, "POST") {
		return
	}
	h.saveAccount(w, r, "")
}
func (h *HTTPHandler) Account(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, e := h.service.Account(r.Context(), principal(r), r.PathValue("account_id"))
		respond(w, "marketplace_account", v, e)
		return
	}
	if !method(w, r, "PUT") {
		return
	}
	h.saveAccount(w, r, r.PathValue("account_id"))
}
func (h *HTTPHandler) saveAccount(w http.ResponseWriter, r *http.Request, id string) {
	var in AccountInput
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	v, e := h.service.SaveAccount(r.Context(), principal(r), id, in)
	respond(w, "marketplace_account", v, e)
}
