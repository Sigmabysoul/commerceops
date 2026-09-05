// This file groups the responsibilities named by the file so related domain logic stays discoverable in the health-check package.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type DatabasePinger interface{ Ping(context.Context) error }

type Handler struct {
	database DatabasePinger
	timeout  time.Duration
}

type response struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

func NewHandler(database DatabasePinger, timeout time.Duration) *Handler {
	return &Handler{database: database, timeout: timeout}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.database.Ping(ctx); err != nil {
		httpserver.WriteJSON(w, http.StatusServiceUnavailable, response{Status: "unavailable", Database: "unavailable"})
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, response{Status: "ok", Database: "ok"})
}
