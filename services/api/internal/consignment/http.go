// This file implements the REST boundary: decoding input, calling services, and encoding consistent responses in the consignment package.
package consignment

import (
	"errors"
	"net/http"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(service *Service) *HTTPHandler { return &HTTPHandler{service: service} }

func (h *HTTPHandler) Departments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.service.ListDepartments(r.Context(), principal(r))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"departments": items})
	case http.MethodPost:
		var input DepartmentInput
		if !httpserver.DecodeJSON(w, r, &input) {
			return
		}
		item, err := h.service.CreateDepartment(r.Context(), principal(r), input)
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusCreated, map[string]any{"department": item})
	default:
		methodNotAllowed(w)
	}
}
func (h *HTTPHandler) DepartmentMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	var input MembershipInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, err := h.service.SetDepartmentMembers(r.Context(), principal(r), r.PathValue("department_id"), input)
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"department": item})
}
func (h *HTTPHandler) Department(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	var input DepartmentInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, err := h.service.UpdateDepartment(r.Context(), principal(r), r.PathValue("department_id"), input)
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"department": item})
}
func (h *HTTPHandler) Consignments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.service.List(r.Context(), principal(r), Filter{Status: r.URL.Query().Get("status"), DepartmentID: r.URL.Query().Get("department_id"), Query: r.URL.Query().Get("q")})
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"consignments": items})
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
func (h *HTTPHandler) Consignment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	item, err := h.service.Get(r.Context(), principal(r), r.PathValue("consignment_id"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"consignment": item})
}
func (h *HTTPHandler) Allocate(w http.ResponseWriter, r *http.Request) {
	var input ActionInput
	if !postJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.Allocate(r.Context(), principal(r), r.PathValue("consignment_id"), input)
	writeResult(w, item, replayed, err)
}
func (h *HTTPHandler) Transition(w http.ResponseWriter, r *http.Request) {
	var input TransitionInput
	if !postJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.Transition(r.Context(), principal(r), r.PathValue("consignment_id"), input)
	writeResult(w, item, replayed, err)
}
func (h *HTTPHandler) Progress(w http.ResponseWriter, r *http.Request) {
	var input ProgressInput
	if !postJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.UpdateProgress(r.Context(), principal(r), r.PathValue("consignment_id"), r.PathValue("line_id"), input)
	writeResult(w, item, replayed, err)
}
func (h *HTTPHandler) Outbound(w http.ResponseWriter, r *http.Request) {
	var input ActionInput
	if !postJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.ConfirmOutbound(r.Context(), principal(r), r.PathValue("consignment_id"), input)
	writeResult(w, item, replayed, err)
}
func (h *HTTPHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	var input ActionInput
	if !postJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.Cancel(r.Context(), principal(r), r.PathValue("consignment_id"), input)
	writeResult(w, item, replayed, err)
}
func postJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return false
	}
	return httpserver.DecodeJSON(w, r, target)
}
func writeResult(w http.ResponseWriter, item Consignment, replayed bool, err error) {
	if writeError(w, err) {
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	httpserver.WriteJSON(w, status, map[string]any{"consignment": item, "idempotent_replay": replayed})
}
func principal(r *http.Request) auth.Principal {
	p, _ := auth.PrincipalFromContext(r.Context())
	return p
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
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Consignment permission or entitlement denied")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Consignment or related record was not found")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Consignment request is invalid")
	case errors.Is(err, ErrConflict):
		httpserver.WriteError(w, http.StatusConflict, "CONFLICT", "Consignment request conflicts with current or idempotent state")
	case errors.Is(err, ErrInvalidState):
		httpserver.WriteError(w, http.StatusConflict, "INVALID_STATE", "Consignment state transition is not allowed")
	case errors.Is(err, ErrIncomplete):
		httpserver.WriteError(w, http.StatusConflict, "INCOMPLETE", "Required consignment quantities are incomplete")
	case errors.Is(err, ErrInsufficient):
		httpserver.WriteError(w, http.StatusConflict, "INSUFFICIENT_STOCK", "Available inventory is insufficient")
	default:
		httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}
