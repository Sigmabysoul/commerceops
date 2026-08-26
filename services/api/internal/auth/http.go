package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

const SessionCookieName = "commerceops_session"

type HTTPHandler struct {
	service      *Service
	secureCookie bool
	lifetime     time.Duration
}

func NewHTTPHandler(service *Service, secureCookie bool, lifetime time.Duration) *HTTPHandler {
	return &HTTPHandler{service: service, secureCookie: secureCookie, lifetime: lifetime}
}

func (h *HTTPHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	var request struct {
		Email     string `json:"email"`
		Password  string `json:"password"`
		CompanyID string `json:"company_id"`
	}
	if !httpserver.DecodeJSON(w, r, &request) {
		return
	}
	token, principal, err := h.service.Login(r.Context(), request.Email, request.Password, request.CompanyID)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			httpserver.WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email, password, or company is invalid")
		case errors.Is(err, ErrInactiveAccess):
			httpserver.WriteError(w, http.StatusForbidden, "ACCESS_INACTIVE", "User or company access is inactive")
		default:
			httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
		}
		return
	}
	h.setCookie(w, token, h.lifetime)
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"user": principal})
}

func (h *HTTPHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	cookie, _ := r.Cookie(SessionCookieName)
	if cookie != nil {
		if err := h.service.Logout(r.Context(), cookie.Value); err != nil {
			httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
			return
		}
	}
	h.setCookie(w, "", -time.Hour)
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) Session(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		httpserver.WriteError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"user": principal})
}

func (h *HTTPHandler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil {
			httpserver.WriteError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
			return
		}
		principal, err := h.service.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			httpserver.WriteError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}

func (h *HTTPHandler) setCookie(w http.ResponseWriter, value string, duration time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: value, Path: "/api/v1", MaxAge: int(duration.Seconds()),
		HttpOnly: true, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode,
	})
}
