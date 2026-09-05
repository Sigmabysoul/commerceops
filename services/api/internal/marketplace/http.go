// This file implements the REST boundary: decoding input, calling services, and encoding consistent responses in the marketplace orchestration package.
package marketplace

import (
	"errors"
	"io"
	"net/http"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(s *Service) *HTTPHandler { return &HTTPHandler{s} }
func (h *HTTPHandler) Jobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpserver.WriteError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
		httpserver.WriteError(w, 400, "INVALID_UPLOAD", "Upload must be a supported file smaller than 20 MiB")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpserver.WriteError(w, 400, "INVALID_UPLOAD", "Source file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxUploadBytes+1))
	if err != nil || len(data) > MaxUploadBytes {
		httpserver.WriteError(w, 400, "INVALID_UPLOAD", "Upload must be a supported file smaller than 20 MiB")
		return
	}
	p, _ := auth.PrincipalFromContext(r.Context())
	result, err := h.service.UploadWithIdempotency(r.Context(), p, header.Filename, data, r.FormValue("idempotency_key"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusAccepted, result)
}
func (h *HTTPHandler) Job(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		p, _ := auth.PrincipalFromContext(r.Context())
		job, err := h.service.Retry(r.Context(), p, r.PathValue("job_id"))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
		return
	}
	if r.Method != http.MethodGet {
		httpserver.WriteError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	p, _ := auth.PrincipalFromContext(r.Context())
	result, err := h.service.Get(r.Context(), p, r.PathValue("job_id"))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, 200, result)
}
func writeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, authorization.ErrPermissionDenied), errors.Is(err, authorization.ErrModuleUnavailable):
		httpserver.WriteError(w, 403, "FORBIDDEN", "Permission or marketplace entitlement denied")
	case errors.Is(err, ErrInvalidFile):
		httpserver.WriteError(w, 400, "INVALID_UPLOAD", "File type, content, or idempotency key is invalid")
	case errors.Is(err, ErrIdempotencyConflict):
		httpserver.WriteError(w, 409, "IDEMPOTENCY_CONFLICT", "Idempotency key was already used with different content")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, 404, "NOT_FOUND", "Processing job not found")
	case errors.Is(err, ErrJobActive):
		httpserver.WriteError(w, 409, "JOB_ACTIVE", "Processing job is already queued or processing")
	default:
		httpserver.WriteError(w, 500, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}
