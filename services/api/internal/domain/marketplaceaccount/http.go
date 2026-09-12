// This file exposes the marketplace-account domain through authenticated REST endpoints.
package marketplaceaccount

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(service *Service) *HTTPHandler { return &HTTPHandler{service: service} }

func (h *HTTPHandler) Identities(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	if r.Method == http.MethodGet {
		value, err := h.service.Identities(r.Context(), p)
		respond(w, "business_identities", value, err)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input IdentityInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	value, err := h.service.SaveIdentity(r.Context(), p, "", input)
	respond(w, "business_identity", value, err)
}

func (h *HTTPHandler) Identity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	p, _ := auth.PrincipalFromContext(r.Context())
	var input IdentityInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	value, err := h.service.SaveIdentity(r.Context(), p, r.PathValue("identity_id"), input)
	respond(w, "business_identity", value, err)
}

func (h *HTTPHandler) Accounts(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	if r.Method == http.MethodGet {
		value, err := h.service.Accounts(r.Context(), p, r.URL.Query().Get("marketplace"))
		respond(w, "marketplace_accounts", value, err)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input AccountInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	value, err := h.service.SaveAccount(r.Context(), p, "", input)
	respond(w, "marketplace_account", value, err)
}

func (h *HTTPHandler) Account(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	p, _ := auth.PrincipalFromContext(r.Context())
	var input AccountInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	value, err := h.service.SaveAccount(r.Context(), p, r.PathValue("account_id"), input)
	respond(w, "marketplace_account", value, err)
}

func methodNotAllowed(w http.ResponseWriter) {
	httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
}
func respond(w http.ResponseWriter, key string, value any, err error) {
	if err == nil {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{key: value})
		return
	}
	switch {
	case errors.Is(err, authorization.ErrPermissionDenied):
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Permission denied")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Business identity or marketplace account not found")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Marketplace account request is invalid")
	case errors.Is(err, ErrConflict):
		httpserver.WriteError(w, http.StatusConflict, "CONFLICT", "Marketplace account identity conflicts with this operation")
	default:
		slog.Error("marketplace account request failed", "error", err)
		httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	}
}
