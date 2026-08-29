package batch

import (
	"context"
	"errors"
	"net/http"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(service *Service) *HTTPHandler { return &HTTPHandler{service: service} }

func (h *HTTPHandler) Batches(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.service.List(r.Context(), principal(r))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"batches": items})
	case http.MethodPost:
		var input CreateInput
		if !httpserver.DecodeJSON(w, r, &input) {
			return
		}
		item, replayed, err := h.service.Create(r.Context(), principal(r), input)
		if writeError(w, err) {
			return
		}
		status := http.StatusCreated
		if replayed {
			status = http.StatusOK
		}
		httpserver.WriteJSON(w, status, map[string]any{"batch": item, "idempotent_replay": replayed})
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) Batch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	item, err := h.service.Get(r.Context(), principal(r), r.PathValue("batch_id"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"batch": item})
}

func (h *HTTPHandler) EligibleOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.service.EligibleOrders(r.Context(), principal(r), r.URL.Query().Get("marketplace"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"orders": items})
}

func (h *HTTPHandler) Ready(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.service.Ready)
}

func (h *HTTPHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.service.Cancel)
}

func (h *HTTPHandler) transition(w http.ResponseWriter, r *http.Request, transition func(context.Context, auth.Principal, string) (Batch, error)) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	item, err := transition(r.Context(), principal(r), r.PathValue("batch_id"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"batch": item})
}

func principal(r *http.Request) auth.Principal {
	value, _ := auth.PrincipalFromContext(r.Context())
	return value
}

func methodNotAllowed(w http.ResponseWriter) {
	httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
}

func writeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, authorization.ErrPermissionDenied), errors.Is(err, authorization.ErrModuleUnavailable):
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Permission or Flipkart entitlement denied")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Batch not found")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Batch request is invalid")
	case errors.Is(err, ErrIneligible):
		httpserver.WriteError(w, http.StatusConflict, "INELIGIBLE_ORDER", "One or more orders are not eligible for this batch")
	case errors.Is(err, ErrConflict):
		httpserver.WriteError(w, http.StatusConflict, "BATCH_CONFLICT", "Batch request conflicts with existing data")
	case errors.Is(err, ErrInvalidState):
		httpserver.WriteError(w, http.StatusConflict, "INVALID_BATCH_STATE", "Batch state transition is not allowed")
	case errors.Is(err, ErrUnresolved):
		httpserver.WriteError(w, http.StatusConflict, "UNRESOLVED_BATCH", "Batch contains unresolved products or quantities")
	default:
		httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}
