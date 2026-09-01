package inventory

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

func (h *HTTPHandler) Balances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.service.ListBalances(r.Context(), principal(r))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"inventory": items})
}

func (h *HTTPHandler) Transactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.service.ListTransactions(r.Context(), principal(r), r.URL.Query().Get("product_id"), r.URL.Query().Get("type"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"transactions": items})
}

func (h *HTTPHandler) StockIn(w http.ResponseWriter, r *http.Request) {
	h.command(w, r, h.service.StockIn)
}
func (h *HTTPHandler) Adjust(w http.ResponseWriter, r *http.Request) {
	h.command(w, r, h.service.Adjust)
}
func (h *HTTPHandler) Correct(w http.ResponseWriter, r *http.Request) {
	h.command(w, r, h.service.Correct)
}
func (h *HTTPHandler) EcommerceOutbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input OutboundInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	items, replayed, err := h.service.ConfirmEcommerceOutbound(r.Context(), principal(r), r.PathValue("batch_id"), input)
	if writeError(w, err) {
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	httpserver.WriteJSON(w, status, map[string]any{"transactions": items, "idempotent_replay": replayed})
}
func (h *HTTPHandler) Reservations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.service.ListReservations(r.Context(), principal(r), r.URL.Query().Get("status"))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"reservations": items})
	case http.MethodPost:
		var input ReserveInput
		if !httpserver.DecodeJSON(w, r, &input) {
			return
		}
		item, replayed, err := h.service.Reserve(r.Context(), principal(r), input)
		if writeError(w, err) {
			return
		}
		status := http.StatusCreated
		if replayed {
			status = http.StatusOK
		}
		httpserver.WriteJSON(w, status, map[string]any{"reservation": item, "idempotent_replay": replayed})
	default:
		methodNotAllowed(w)
	}
}
func (h *HTTPHandler) ReleaseReservation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input ReleaseInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, replayed, err := h.service.ReleaseReservation(r.Context(), principal(r), r.PathValue("reservation_id"), input)
	if writeError(w, err) {
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	httpserver.WriteJSON(w, status, map[string]any{"reservation": item, "idempotent_replay": replayed})
}

func (h *HTTPHandler) command(w http.ResponseWriter, r *http.Request, command func(context.Context, auth.Principal, CommandInput) (Transaction, bool, error)) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input CommandInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, replayed, err := command(r.Context(), principal(r), input)
	if writeError(w, err) {
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	httpserver.WriteJSON(w, status, map[string]any{"transaction": item, "idempotent_replay": replayed})
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
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Permission or inventory entitlement denied")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Inventory product not found")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Inventory request is invalid")
	case errors.Is(err, ErrConflict):
		httpserver.WriteError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency key conflicts with an existing inventory transaction")
	case errors.Is(err, ErrInsufficientStock):
		httpserver.WriteError(w, http.StatusConflict, "INSUFFICIENT_STOCK", "Inventory cannot become negative or fall below reserved stock")
	case errors.Is(err, ErrCancelledOrder):
		httpserver.WriteError(w, http.StatusConflict, "CANCELLED_ORDER", "A cancelled order cannot be confirmed outbound")
	default:
		httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}
