// This file exposes the authenticated REST boundary for Trace Box operations.
package traceability

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(service *Service) *HTTPHandler { return &HTTPHandler{service: service} }

func (h *HTTPHandler) Boxes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.service.List(r.Context(), principal(r))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"trace_boxes": items})
	case http.MethodPost:
		var input CreateInput
		if !httpserver.DecodeJSON(w, r, &input) {
			return
		}
		item, replayed, err := h.service.Create(r.Context(), principal(r), input)
		writeResult(w, item, replayed, err)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) Box(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	item, err := h.service.Get(r.Context(), principal(r), r.PathValue("trace_box_id"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"trace_box": item})
}

func (h *HTTPHandler) Options(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.service.Options(r.Context(), principal(r))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"options": items})
}

func (h *HTTPHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	identifier, err := url.PathUnescape(strings.TrimSpace(r.PathValue("opaque_identifier")))
	if err != nil {
		writeError(w, ErrInvalidInput)
		return
	}
	item, err := h.service.Resolve(r.Context(), principal(r), identifier)
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"trace_box": item})
}

func (h *HTTPHandler) AddContent(w http.ResponseWriter, r *http.Request) {
	var input ContentInput
	if !postJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.AddContent(r.Context(), principal(r), r.PathValue("trace_box_id"), input)
	writeResult(w, item, replayed, err)
}

func (h *HTTPHandler) RemoveContent(w http.ResponseWriter, r *http.Request) {
	var input ContentInput
	if !postJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.RemoveContent(r.Context(), principal(r), r.PathValue("trace_box_id"), input)
	writeResult(w, item, replayed, err)
}

func (h *HTTPHandler) TransferCustody(w http.ResponseWriter, r *http.Request) {
	var input CustodyInput
	if !postJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.TransferCustody(r.Context(), principal(r), r.PathValue("trace_box_id"), input)
	writeResult(w, item, replayed, err)
}

func postJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return false
	}
	return httpserver.DecodeJSON(w, r, target)
}

func writeResult(w http.ResponseWriter, item TraceBox, replayed bool, err error) {
	if writeError(w, err) {
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	httpserver.WriteJSON(w, status, map[string]any{"trace_box": item, "idempotent_replay": replayed})
}

func writeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, authorization.ErrPermissionDenied), errors.Is(err, authorization.ErrModuleUnavailable):
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Traceability permission or entitlement denied")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Trace Box or related record was not found")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Traceability request is invalid")
	case errors.Is(err, ErrConflict):
		httpserver.WriteError(w, http.StatusConflict, "CONFLICT", "Traceability request conflicts with an existing idempotency key")
	case errors.Is(err, ErrQuantity):
		httpserver.WriteError(w, http.StatusConflict, "INSUFFICIENT_QUANTITY", "Trace Box does not contain that quantity")
	default:
		httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}

func principal(r *http.Request) auth.Principal {
	value, _ := auth.PrincipalFromContext(r.Context())
	return value
}

func methodNotAllowed(w http.ResponseWriter) {
	httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
}
