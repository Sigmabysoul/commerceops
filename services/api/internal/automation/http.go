package automation

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(s *Service) *HTTPHandler { return &HTTPHandler{service: s} }
func (h *HTTPHandler) Register(mux *http.ServeMux, session func(http.Handler) http.Handler) {
	for path, handler := range map[string]http.HandlerFunc{
		"/api/v1/automations/rules": h.Rules, "/api/v1/automations/rules/{rule_id}": h.Rule,
		"/api/v1/automations/rules/{rule_id}/pause": h.Pause, "/api/v1/automations/rules/{rule_id}/test": h.Test,
		"/api/v1/automations/rules/{rule_id}/history": h.History, "/api/v1/automations/timezone": h.Timezone,
		"/api/v1/automations/preview": h.Preview, "/api/v1/automations/runs": h.Runs,
		"/api/v1/automations/runs/{execution_id}/retry": h.Retry, "/api/v1/automations/upcoming": h.Upcoming,
		"/api/v1/automations/options": h.Options,
		"/api/v1/automations/report":  h.Report,
	} {
		mux.Handle(path, session(handler))
	}
}
func principal(r *http.Request) auth.Principal {
	p, _ := auth.PrincipalFromContext(r.Context())
	return p
}
func respond(w http.ResponseWriter, value any, err error) {
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, value)
}
func method(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method == want {
		return true
	}
	httpserver.WriteError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
	return false
}
func writeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, authorization.ErrPermissionDenied):
		httpserver.WriteError(w, 403, "FORBIDDEN", "Permission denied")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, 400, "INVALID_REQUEST", "Automation request is invalid")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, 404, "NOT_FOUND", "Automation resource not found")
	case errors.Is(err, ErrConflict):
		httpserver.WriteError(w, 409, "CONFLICT", "Rule version or execution state changed; refresh before retrying")
	default:
		slog.Error("automation request failed", "error", err)
		httpserver.WriteError(w, 500, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}
func (h *HTTPHandler) Rules(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, e := h.service.Rules(r.Context(), principal(r))
		respond(w, map[string]any{"rules": v}, e)
		return
	}
	if !method(w, r, http.MethodPost) {
		return
	}
	var in RuleInput
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	v, e := h.service.SaveRule(r.Context(), principal(r), "", in)
	if writeError(w, e) {
		return
	}
	httpserver.WriteJSON(w, 201, map[string]any{"rule": v})
}
func (h *HTTPHandler) Rule(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, e := h.service.Rule(r.Context(), principal(r), r.PathValue("rule_id"))
		respond(w, map[string]any{"rule": v}, e)
		return
	}
	if !method(w, r, http.MethodPut) {
		return
	}
	var in RuleInput
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	v, e := h.service.SaveRule(r.Context(), principal(r), r.PathValue("rule_id"), in)
	respond(w, map[string]any{"rule": v}, e)
}
func (h *HTTPHandler) Pause(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var in struct {
		Version int  `json:"version"`
		Paused  bool `json:"paused"`
	}
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	v, e := h.service.Pause(r.Context(), principal(r), r.PathValue("rule_id"), in.Version, in.Paused)
	respond(w, map[string]any{"rule": v}, e)
}
func (h *HTTPHandler) Test(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var in struct {
		Key string `json:"idempotency_key"`
	}
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	v, e := h.service.TestRun(r.Context(), principal(r), r.PathValue("rule_id"), in.Key)
	respond(w, map[string]any{"execution_id": v}, e)
}
func (h *HTTPHandler) Timezone(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, e := h.service.Timezone(r.Context(), principal(r))
		respond(w, map[string]any{"timezone": v}, e)
		return
	}
	if !method(w, r, http.MethodPut) {
		return
	}
	var in struct {
		Timezone string `json:"timezone"`
	}
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	e := h.service.SetTimezone(r.Context(), principal(r), in.Timezone)
	respond(w, map[string]any{"timezone": in.Timezone}, e)
}
func (h *HTTPHandler) Preview(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var in Schedule
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	v, e := h.service.Preview(r.Context(), principal(r), in)
	respond(w, map[string]any{"occurrences": v}, e)
}
func (h *HTTPHandler) Runs(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	v, e := h.service.Runs(r.Context(), principal(r), r.URL.Query().Get("rule_id"), r.URL.Query().Get("failures") == "true")
	respond(w, map[string]any{"runs": v}, e)
}
func (h *HTTPHandler) Retry(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if writeError(w, h.service.Retry(r.Context(), principal(r), r.PathValue("execution_id"))) {
		return
	}
	w.WriteHeader(204)
}
func (h *HTTPHandler) Upcoming(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	v, e := h.service.Upcoming(r.Context(), principal(r))
	respond(w, map[string]any{"upcoming": v}, e)
}
func (h *HTTPHandler) Report(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	v, e := h.service.Report(r.Context(), principal(r))
	respond(w, map[string]any{"metrics": v}, e)
}
func (h *HTTPHandler) History(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	v, e := h.service.History(r.Context(), principal(r), r.PathValue("rule_id"))
	respond(w, map[string]any{"history": v}, e)
}

func (h *HTTPHandler) Options(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	v, e := h.service.Options(r.Context(), principal(r))
	respond(w, v, e)
}
