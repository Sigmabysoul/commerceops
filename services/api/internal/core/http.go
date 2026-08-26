package core

import (
	"errors"
	"net/http"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(service *Service) *HTTPHandler { return &HTTPHandler{service: service} }

func (h *HTTPHandler) Company(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	company, err := h.service.Company(r.Context(), principal(r))
	if writeServiceError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"company": company})
}

func (h *HTTPHandler) Employees(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		employees, err := h.service.ListEmployees(r.Context(), principal(r))
		if writeServiceError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"employees": employees})
	case http.MethodPost:
		var request struct {
			DisplayName string  `json:"display_name"`
			UserID      *string `json:"user_id"`
		}
		if !httpserver.DecodeJSON(w, r, &request) {
			return
		}
		employee, err := h.service.CreateEmployee(r.Context(), principal(r), request.DisplayName, request.UserID)
		if writeServiceError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusCreated, map[string]any{"employee": employee})
	default:
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
	}
}

func (h *HTTPHandler) Employee(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	var request struct {
		Status string `json:"status"`
	}
	if !httpserver.DecodeJSON(w, r, &request) {
		return
	}
	employee, err := h.service.SetEmployeeStatus(r.Context(), principal(r), r.PathValue("employee_id"), request.Status)
	if writeServiceError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"employee": employee})
}

func (h *HTTPHandler) UserAccesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !httpserver.DecodeJSON(w, r, &request) {
		return
	}
	access, err := h.service.CreateUserAccess(r.Context(), principal(r), request.Email, request.Password)
	if writeServiceError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, map[string]any{"user_access": access})
}

func (h *HTTPHandler) UserAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	var request struct {
		Status string `json:"status"`
	}
	if !httpserver.DecodeJSON(w, r, &request) {
		return
	}
	if writeServiceError(w, h.service.SetUserAccessStatus(r.Context(), principal(r), r.PathValue("user_id"), request.Status)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) UserRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	var request struct {
		RoleIDs []string `json:"role_ids"`
	}
	if !httpserver.DecodeJSON(w, r, &request) {
		return
	}
	if writeServiceError(w, h.service.SetUserRoles(r.Context(), principal(r), r.PathValue("user_id"), request.RoleIDs)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) Roles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		roles, err := h.service.ListRoles(r.Context(), principal(r))
		if writeServiceError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"roles": roles})
	case http.MethodPost:
		var request struct {
			Name string `json:"name"`
		}
		if !httpserver.DecodeJSON(w, r, &request) {
			return
		}
		role, err := h.service.CreateRole(r.Context(), principal(r), request.Name)
		if writeServiceError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusCreated, map[string]any{"role": role})
	default:
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
	}
}

func (h *HTTPHandler) RolePermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	var request struct {
		Permissions []string `json:"permissions"`
	}
	if !httpserver.DecodeJSON(w, r, &request) {
		return
	}
	if writeServiceError(w, h.service.SetRolePermissions(r.Context(), principal(r), r.PathValue("role_id"), request.Permissions)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) Permissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	permissions, err := h.service.ListPermissions(r.Context(), principal(r))
	if writeServiceError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"permissions": permissions})
}

func (h *HTTPHandler) Entitlements(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	entitlements, err := h.service.ListEntitlements(r.Context(), principal(r))
	if writeServiceError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"module_entitlements": entitlements})
}

func (h *HTTPHandler) Entitlement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if !httpserver.DecodeJSON(w, r, &request) {
		return
	}
	if writeServiceError(w, h.service.SetEntitlement(r.Context(), principal(r), r.PathValue("module_key"), request.Enabled)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) AuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	entries, err := h.service.ListAudit(r.Context(), principal(r))
	if writeServiceError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"audit_logs": entries})
}

func principal(r *http.Request) auth.Principal {
	value, _ := auth.PrincipalFromContext(r.Context())
	return value
}

func writeServiceError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, authorization.ErrPermissionDenied), errors.Is(err, authorization.ErrModuleUnavailable):
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Permission denied")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Request is invalid")
	default:
		httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}
