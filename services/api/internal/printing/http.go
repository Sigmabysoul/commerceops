// This file implements the REST boundary: decoding input, calling services, and encoding consistent responses in the printing package.
package printing

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(s *Service) *HTTPHandler { return &HTTPHandler{service: s} }

func (h *HTTPHandler) RequireAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, ErrInvalidCredential)
			return
		}
		p, err := h.service.AuthenticateAgent(r.Context(), strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			writeError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(withAgent(r.Context(), p)))
	})
}

func (h *HTTPHandler) Agents(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		items, err := h.service.ListAgents(r.Context(), userPrincipal(r))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"agents": items})
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		FriendlyName string `json:"friendly_name"`
		Platform     string `json:"platform"`
	}
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	x, err := h.service.CreateAgent(r.Context(), userPrincipal(r), in.FriendlyName, in.Platform)
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, map[string]any{"agent": x.Agent, "credential": x.Credential})
}
func (h *HTTPHandler) Agent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if writeError(w, h.service.RevokeAgent(r.Context(), userPrincipal(r), r.PathValue("agent_id"))) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *HTTPHandler) Printers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	x, err := h.service.ListPrinters(r.Context(), userPrincipal(r))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"printers": x})
}
func (h *HTTPHandler) Printer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	var input UpdatePrinterInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, err := h.service.UpdatePrinter(r.Context(), userPrincipal(r), r.PathValue("printer_id"), input)
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"printer": item})
}
func (h *HTTPHandler) Assets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		x, err := h.service.ListAssets(r.Context(), userPrincipal(r), r.URL.Query().Get("search"), r.URL.Query().Get("category"))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"assets": x})
	case http.MethodPost:
		h.uploadAsset(w, r)
	default:
		methodNotAllowed(w)
	}
}
func (h *HTTPHandler) Asset(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var input UpdateAssetInput
		if !httpserver.DecodeJSON(w, r, &input) {
			return
		}
		item, err := h.service.UpdateAsset(r.Context(), userPrincipal(r), r.PathValue("asset_id"), input)
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"asset": item})
		return
	}
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if writeError(w, h.service.ArchiveAsset(r.Context(), userPrincipal(r), r.PathValue("asset_id"))) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *HTTPHandler) uploadAsset(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxPDFBytes+1<<20)
	if err := r.ParseMultipartForm(MaxPDFBytes); err != nil {
		writeError(w, ErrInvalidInput)
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, ErrInvalidInput)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxPDFBytes+1))
	if err != nil || len(data) > MaxPDFBytes {
		writeError(w, ErrInvalidInput)
		return
	}
	copies, err := strconv.Atoi(r.FormValue("default_copies"))
	if err != nil {
		writeError(w, ErrInvalidInput)
		return
	}
	var printer, product *string
	if v := strings.TrimSpace(r.FormValue("default_printer_id")); v != "" {
		printer = &v
	}
	if v := strings.TrimSpace(r.FormValue("product_id")); v != "" {
		product = &v
	}
	favorite, _ := strconv.ParseBool(r.FormValue("favorite"))
	a, err := h.service.CreateAsset(r.Context(), userPrincipal(r), r.FormValue("name"), r.FormValue("category"), r.FormValue("description"), printer, copies, product, favorite, header.Filename, data)
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, map[string]any{"asset": a})
}
func (h *HTTPHandler) Jobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		x, err := h.service.ListJobs(r.Context(), userPrincipal(r))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"printer_jobs": x})
	case http.MethodPost:
		var in CreateJobInput
		if !httpserver.DecodeJSON(w, r, &in) {
			return
		}
		x, replay, err := h.service.CreateQuickJob(r.Context(), userPrincipal(r), in)
		if writeError(w, err) {
			return
		}
		status := http.StatusCreated
		if replay {
			status = http.StatusOK
		}
		httpserver.WriteJSON(w, status, map[string]any{"printer_job": x, "idempotent_replay": replay})
	default:
		methodNotAllowed(w)
	}
}
func (h *HTTPHandler) ArtifactJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in QueueArtifactInput
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	in.ArtifactID = r.PathValue("artifact_id")
	x, replay, err := h.service.QueueArtifact(r.Context(), userPrincipal(r), in)
	if writeError(w, err) {
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	httpserver.WriteJSON(w, status, map[string]any{"printer_job": x, "idempotent_replay": replay})
}
func (h *HTTPHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	x, err := h.service.Cancel(r.Context(), userPrincipal(r), r.PathValue("printer_job_id"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"printer_job": x})
}
func (h *HTTPHandler) Retry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	x, replay, err := h.service.Retry(r.Context(), userPrincipal(r), r.PathValue("printer_job_id"), in.IdempotencyKey)
	if writeError(w, err) {
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	httpserver.WriteJSON(w, status, map[string]any{"printer_job": x, "idempotent_replay": replay})
}
func (h *HTTPHandler) AgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		Printers []LocalPrinter `json:"printers"`
	}
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	x, err := h.service.Heartbeat(r.Context(), requestAgent(r), in.Printers)
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"printers": x})
}
func (h *HTTPHandler) AgentClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	x, err := h.service.Claim(r.Context(), requestAgent(r))
	if errors.Is(err, ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"claim": x})
}
func (h *HTTPHandler) AgentArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	data, err := h.service.AgentDownload(r.Context(), requestAgent(r), r.PathValue("printer_job_id"), r.Header.Get("X-Print-Lease"))
	if writeError(w, err) {
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="print-job.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
func (h *HTTPHandler) AgentReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		LeaseToken     string `json:"lease_token"`
		Status         string `json:"status"`
		FailureCode    string `json:"failure_code"`
		FailureMessage string `json:"failure_message"`
	}
	if !httpserver.DecodeJSON(w, r, &in) {
		return
	}
	x, err := h.service.Report(r.Context(), requestAgent(r), r.PathValue("printer_job_id"), in.LeaseToken, in.Status, in.FailureCode, in.FailureMessage)
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"printer_job": x})
}

func userPrincipal(r *http.Request) auth.Principal {
	p, _ := auth.PrincipalFromContext(r.Context())
	return p
}
func requestAgent(r *http.Request) AgentPrincipal {
	p, _ := agentPrincipalFromContext(r.Context())
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
	case errors.Is(err, ErrInvalidCredential):
		httpserver.WriteError(w, http.StatusUnauthorized, "INVALID_AGENT_CREDENTIAL", "Agent credential is invalid")
	case errors.Is(err, authorization.ErrPermissionDenied):
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Permission denied")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Printing resource not found")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Printing request is invalid")
	case errors.Is(err, ErrConflict):
		httpserver.WriteError(w, http.StatusConflict, "CONFLICT", "Printing request conflicts with existing data")
	case errors.Is(err, ErrInvalidState):
		httpserver.WriteError(w, http.StatusConflict, "INVALID_STATE", "Print job state transition is not allowed")
	default:
		httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}
