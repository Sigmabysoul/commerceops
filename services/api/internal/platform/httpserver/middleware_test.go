// This file verifies the production HTTP boundary: browser origins, request correlation,
// response metadata, security headers, and panic containment.
package httpserver

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareAddsSecurityAndRequestMetadata(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := Middleware(logger, []string{"https://ops.example.test"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestID(r.Context()) != "known-request" {
			t.Fatalf("request ID missing from context")
		}
		WriteJSON(w, http.StatusCreated, map[string]string{"status": "created"})
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/example", strings.NewReader(`{}`))
	request.Header.Set("Origin", "https://ops.example.test")
	request.Header.Set("X-Request-ID", "known-request")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated || recorder.Header().Get("X-Request-ID") != "known-request" || recorder.Header().Get("Access-Control-Allow-Origin") != "https://ops.example.test" {
		t.Fatalf("response = %d headers=%v", recorder.Code, recorder.Header())
	}
	for key, value := range map[string]string{
		"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff",
		"X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer",
		"Strict-Transport-Security": "max-age=31536000",
	} {
		if got := recorder.Header().Get(key); got != value {
			t.Errorf("%s=%q want %q", key, got, value)
		}
	}
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["request_id"] != "known-request" || entry["status"] != float64(http.StatusCreated) || entry["bytes"] == float64(0) {
		t.Fatalf("request log = %#v", entry)
	}
}

func TestMiddlewareRejectsUntrustedOriginBeforeHandler(t *testing.T) {
	called := false
	handler := Middleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), []string{"https://ops.example.test"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/example", nil)
	request.Header.Set("Origin", "https://attacker.example.test")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if called || recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "ORIGIN_NOT_ALLOWED") {
		t.Fatalf("called=%v response=%d %s", called, recorder.Code, recorder.Body.String())
	}
}

func TestMiddlewareGeneratesRequestIDAndRecoversPanic(t *testing.T) {
	var logs bytes.Buffer
	handler := Middleware(slog.New(slog.NewJSONHandler(&logs, nil)), []string{"http://localhost:3000"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("test panic") }))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/example", nil)
	request.Header.Set("X-Request-ID", "contains spaces")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError || !validRequestID.MatchString(recorder.Header().Get("X-Request-ID")) {
		t.Fatalf("response=%d request_id=%q", recorder.Code, recorder.Header().Get("X-Request-ID"))
	}
	if strings.Contains(recorder.Body.String(), "test panic") || !strings.Contains(logs.String(), `"status":500`) {
		t.Fatalf("panic response/logs = %s / %s", recorder.Body.String(), logs.String())
	}
}
