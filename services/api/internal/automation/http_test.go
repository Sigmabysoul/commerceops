package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/auth"
)

func TestHTTPPermissionTenantAndInputBoundaries(t *testing.T) {
	f := setup(t)
	r := f.rule(t, "scheduled")
	request := func(p auth.Principal, method, path, body string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		NewHTTPHandler(f.s).Register(mux, func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
			})
		})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return rec
	}
	for _, path := range []string{"/rules", "/rules/" + r.ID, "/rules/" + r.ID + "/history", "/timezone", "/runs", "/upcoming", "/report", "/options"} {
		rec := request(f.p, "GET", "/api/v1/automations"+path, "")
		if rec.Code != 200 {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body)
		}
	}
	if rec := request(f.other, "GET", "/api/v1/automations/rules/"+r.ID, ""); rec.Code != 404 {
		t.Fatal(rec.Code)
	}
	if rec := request(f.p, "PUT", "/api/v1/automations/timezone", `{"timezone":"UTC","company_id":"`+f.other.CompanyID+`"}`); rec.Code != 400 {
		t.Fatal("accepted frontend company_id", rec.Code)
	}
	if rec := request(f.p, "POST", "/api/v1/automations/preview", `{"mode":"daily","times":["25:00"]}`); rec.Code != 400 {
		t.Fatal(rec.Code)
	}
	if rec := request(f.p, "POST", "/api/v1/automations/rules/"+r.ID+"/pause", `{"version":999,"paused":true}`); rec.Code != 409 {
		t.Fatal(rec.Code)
	}
	sql(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND permission_key LIKE 'automations.%'`, f.p.CompanyID)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/rules", ""}, {"GET", "/runs", ""}, {"GET", "/report", ""}, {"GET", "/options", ""},
		{"POST", "/rules/" + r.ID + "/test", `{"idempotency_key":"denied"}`}, {"PUT", "/timezone", `{"timezone":"UTC"}`}, {"POST", "/rules/" + r.ID + "/pause", `{"version":1,"paused":true}`},
	} {
		rec := request(f.p, c.method, "/api/v1/automations"+c.path, c.body)
		if rec.Code != 403 {
			t.Fatalf("denied %s=%d %s", c.path, rec.Code, rec.Body)
		}
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		check(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
		if envelope.Error.Code != "FORBIDDEN" {
			t.Fatal(envelope)
		}
	}
	if f.count(t, "automation_executions") != 0 {
		t.Fatal("unauthorized test persisted")
	}
	// An automation manager needs no unrelated printer-admin/read grants to
	// select friendly resources for a rule.
	sql(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1`, f.p.CompanyID)
	sql(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'automations.manage')`, f.p.CompanyID, f.role)
	options, err := f.s.Options(context.Background(), f.p)
	check(t, err)
	if len(options.Assets) != 1 || len(options.Printers) != 1 {
		t.Fatal(options)
	}
}
