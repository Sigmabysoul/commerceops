// This file implements the REST boundary: decoding input, calling services, and encoding consistent responses in the returns package.
package returns

import (
	"errors"
	"net/http"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(service *Service) *HTTPHandler { return &HTTPHandler{service: service} }

func (h *HTTPHandler) Cancellations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.service.ListCancellations(r.Context(), principal(r), r.URL.Query().Get("status"), r.URL.Query().Get("marketplace"))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"cancellations": items})
	case http.MethodPost:
		var input CreateCancellationInput
		if !httpserver.DecodeJSON(w, r, &input) {
			return
		}
		item, replayed, err := h.service.CreateCancellation(r.Context(), principal(r), input)
		if writeError(w, err) {
			return
		}
		status := http.StatusCreated
		if replayed {
			status = http.StatusOK
		}
		httpserver.WriteJSON(w, status, map[string]any{"cancellation": item, "idempotent_replay": replayed})
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) Cancellation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	item, err := h.service.GetCancellation(r.Context(), principal(r), r.PathValue("cancellation_id"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"cancellation": item})
}

func (h *HTTPHandler) CloseCancellation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input CloseInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.CloseCancellation(r.Context(), principal(r), r.PathValue("cancellation_id"), input)
	if writeError(w, err) {
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	httpserver.WriteJSON(w, status, map[string]any{"cancellation": item, "idempotent_replay": replayed})
}

func (h *HTTPHandler) Returns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.service.ListReturns(r.Context(), principal(r), r.URL.Query().Get("status"), r.URL.Query().Get("marketplace"))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"returns": items})
	case http.MethodPost:
		var input CreateReturnInput
		if !httpserver.DecodeJSON(w, r, &input) {
			return
		}
		item, replayed, err := h.service.CreateReturn(r.Context(), principal(r), input)
		if writeError(w, err) {
			return
		}
		status := http.StatusCreated
		if replayed {
			status = http.StatusOK
		}
		httpserver.WriteJSON(w, status, map[string]any{"return": item, "idempotent_replay": replayed})
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) Return(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	item, err := h.service.GetReturn(r.Context(), principal(r), r.PathValue("return_id"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"return": item})
}

func (h *HTTPHandler) Receive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input ReceiveReturnInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.ReceiveReturn(r.Context(), principal(r), r.PathValue("return_id"), input)
	if writeError(w, err) {
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	httpserver.WriteJSON(w, status, map[string]any{"return": item, "idempotent_replay": replayed})
}

func (h *HTTPHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input InspectReturnInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.InspectReturn(r.Context(), principal(r), r.PathValue("return_id"), input)
	writeLifecycleResult(w, item, replayed, err)
}

func (h *HTTPHandler) Restock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input RestockReturnInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.RestockReturn(r.Context(), principal(r), r.PathValue("return_id"), input)
	writeLifecycleResult(w, item, replayed, err)
}

func (h *HTTPHandler) CorrectRestock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input CorrectRestockInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.CorrectRestock(r.Context(), principal(r), r.PathValue("return_id"), input)
	writeLifecycleResult(w, item, replayed, err)
}

func (h *HTTPHandler) CloseReturn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input CloseInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.CloseReturn(r.Context(), principal(r), r.PathValue("return_id"), input)
	writeLifecycleResult(w, item, replayed, err)
}

func writeLifecycleResult(w http.ResponseWriter, item ReturnCase, replayed bool, err error) {
	if writeError(w, err) {
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	httpserver.WriteJSON(w, status, map[string]any{"return": item, "idempotent_replay": replayed})
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
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Permission or returns entitlement denied")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Return, cancellation, or source order was not found")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Return or cancellation request is invalid")
	case errors.Is(err, ErrConflict):
		httpserver.WriteError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Request conflicts with an existing return or cancellation")
	case errors.Is(err, ErrInvalidState):
		httpserver.WriteError(w, http.StatusConflict, "INVALID_RETURN_STATE", "Return state transition is not allowed")
	case errors.Is(err, ErrQuantity):
		httpserver.WriteError(w, http.StatusConflict, "RETURN_QUANTITY_EXCEEDED", "Return quantity exceeds the eligible order quantity")
	case errors.Is(err, ErrInventoryState):
		httpserver.WriteError(w, http.StatusConflict, "INVENTORY_CONSTRAINT", "Inventory cannot apply the return movement")
	default:
		httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}
