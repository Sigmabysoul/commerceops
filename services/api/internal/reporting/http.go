package reporting

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(service *Service) *HTTPHandler { return &HTTPHandler{service: service} }

func (h *HTTPHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	from, e1 := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, e2 := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	limit := 50
	offset := 0
	var e3, e4 error
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, e3 = strconv.Atoi(value)
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, e4 = strconv.Atoi(value)
	}
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_RANGE", "Reporting range or pagination is invalid")
		return
	}
	p, _ := auth.PrincipalFromContext(r.Context())
	report, err := h.service.Dashboard(r.Context(), p, Filter{From: from, To: to, Marketplace: r.URL.Query().Get("marketplace"), ProductID: r.URL.Query().Get("product_id"), Limit: limit, Offset: offset})
	if err == nil {
		httpserver.WriteJSON(w, http.StatusOK, report)
		return
	}
	if errors.Is(err, ErrInvalidRange) {
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_RANGE", "Reporting range, filters, or pagination are invalid")
		return
	}
	if errors.Is(err, authorization.ErrPermissionDenied) || errors.Is(err, authorization.ErrModuleUnavailable) {
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Reporting permission denied")
		return
	}
	httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
}
