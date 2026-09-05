// This file covers health endpoint status and response behavior without starting the full server in the health-check package.
package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakePinger struct{ err error }

func (p fakePinger) Ping(context.Context) error { return p.err }

func TestHealthReturnsOK(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHandler(fakePinger{}, time.Second).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"database":"ok"`) {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestHealthReturnsUnavailableWithoutInternalError(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHandler(fakePinger{err: errors.New("password secret-host")}, time.Second).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(recorder.Body.String(), "secret-host") {
		t.Fatal("response exposed internal database error")
	}
}
