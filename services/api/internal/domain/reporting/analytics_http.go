// The analytics HTTP boundary parses report coordinates and delegates metric computation
// and permission checks to Reporting. Operational source data is never changed here.
package reporting

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

func (h *HTTPHandler) Analytics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	query := r.URL.Query()
	from, e1 := time.Parse(time.RFC3339, query.Get("from"))
	to, e2 := time.Parse(time.RFC3339, query.Get("to"))
	limit, offset := 20, 0
	var e3, e4 error
	if query.Has("limit") {
		limit, e3 = strconv.Atoi(query.Get("limit"))
	}
	if query.Has("offset") {
		offset, e4 = strconv.Atoi(query.Get("offset"))
	}
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_RANGE", "Reporting range or pagination is invalid")
		return
	}
	p, _ := auth.PrincipalFromContext(r.Context())
	report, err := h.service.Analytics(r.Context(), p, AnalyticsFilter{From: from, To: to, Timezone: query.Get("timezone"), Limit: limit, Offset: offset})
	switch {
	case err == nil:
		httpserver.WriteJSON(w, http.StatusOK, report)
	case errors.Is(err, ErrInvalidRange):
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_RANGE", "Use a range up to 366 days, a valid timezone, and valid pagination")
	case errors.Is(err, authorization.ErrPermissionDenied), errors.Is(err, authorization.ErrModuleUnavailable):
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Reporting and Traceability access are required")
	default:
		slog.ErrorContext(r.Context(), "operations analytics failed", "company_id", p.CompanyID, "error", err)
		httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to load operations analytics")
	}
}
