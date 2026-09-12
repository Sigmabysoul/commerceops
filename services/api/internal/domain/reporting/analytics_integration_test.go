// This file verifies operational analytics against immutable PostgreSQL history.
// Fixed event times make denominators, boundary inclusion, and cycle sources explicit.
package reporting

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

type analyticsFixture struct {
	db                  *pgxpool.Pool
	service             *Service
	principal, other    auth.Principal
	role                string
	products, employees []string
	otherProduct        string
	otherEmployee       string
	sequence            int
}

// Each test migrates its own schema because workflow history cannot be deleted and
// other packages may be running integration tests against the same database.
func setupAnalytics(t *testing.T) *analyticsFixture {
	t.Helper()
	connection := os.Getenv("TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("analytics_test_%d", time.Now().UnixNano())
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		base.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(connection)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		if _, err := base.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		base.Close()
	})
	migrations, err := filepath.Glob("../../../migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("migration files missing: %v", err)
	}
	for _, path := range migrations {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, string(contents)); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
	}
	f := &analyticsFixture{db: db, service: NewService(db, authorization.NewService(db))}
	for i := range 2 {
		var p auth.Principal
		var role string
		mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{fmt.Sprintf("Analytics company %d", i)}, &p.CompanyID)
		mustScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{fmt.Sprintf("analytics-%d@example.test", i)}, &p.UserID)
		mustExec(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$2)`, p.CompanyID, p.UserID)
		mustScan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Analyst') RETURNING id`, []any{p.CompanyID}, &role)
		mustExec(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'reports.view'),($1,$2,'traceability.view')`, p.CompanyID, role)
		mustExec(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, p.CompanyID, p.UserID, role)
		mustExec(t, db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'traceability',true)`, p.CompanyID)
		if i == 0 {
			f.principal, f.role = p, role
		} else {
			f.other = p
		}
	}
	for i := range 2 {
		var product string
		mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,$2,'Analytics product') RETURNING id`, []any{f.principal.CompanyID, fmt.Sprintf("ANA-%d", i)}, &product)
		f.products = append(f.products, product)
	}
	// Matching names deliberately exercise the stable employee-ID pagination key.
	for i := range 4 {
		var employee string
		status := "active"
		if i == 2 {
			status = "inactive"
		}
		mustScan(t, db, `INSERT INTO employees(company_id,display_name,status) VALUES($1,'Operator',$2) RETURNING id`, []any{f.principal.CompanyID, status}, &employee)
		f.employees = append(f.employees, employee)
	}
	mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'OTHER','Other product') RETURNING id`, []any{f.other.CompanyID}, &f.otherProduct)
	mustScan(t, db, `INSERT INTO employees(company_id,display_name) VALUES($1,'Other operator') RETURNING id`, []any{f.other.CompanyID}, &f.otherEmployee)
	return f
}

func (f *analyticsFixture) box(t *testing.T, p auth.Principal, at time.Time) string {
	t.Helper()
	f.sequence++
	key := fmt.Sprintf("analytics-box-%d", f.sequence)
	hash := sha256.Sum256([]byte(key))
	opaque := "TBX_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hash[:16])
	var box string
	mustScan(t, f.db, `INSERT INTO trace_boxes(company_id,opaque_identifier,created_by,idempotency_key,request_hash,created_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, []any{p.CompanyID, opaque, p.UserID, key, fmt.Sprintf("%064x", f.sequence), at}, &box)
	f.event(t, p, box, "box_created", at)
	return box
}

func (f *analyticsFixture) event(t *testing.T, p auth.Principal, box, kind string, at time.Time) string {
	t.Helper()
	f.sequence++
	var event string
	mustScan(t, f.db, `INSERT INTO trace_box_events(company_id,trace_box_id,event_type,actor_user_id,idempotency_key,request_hash,created_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, []any{p.CompanyID, box, kind, p.UserID, fmt.Sprintf("analytics-event-%d", f.sequence), fmt.Sprintf("%064x", f.sequence), at}, &event)
	return event
}

type analyticsQCLine struct {
	product           string
	checked, rejected int64
	reason            string
}

func (f *analyticsFixture) qc(t *testing.T, p auth.Principal, box, employee string, at time.Time, lines ...analyticsQCLine) string {
	t.Helper()
	event := f.event(t, p, box, "qc_completed", at)
	mustExec(t, f.db, `INSERT INTO trace_box_qc_checks(company_id,event_id,trace_box_id,checked_by_employee_id) VALUES($1,$2,$3,$4)`, p.CompanyID, event, box, employee)
	for _, line := range lines {
		var reason, work any
		if line.rejected > 0 {
			reason, work = line.reason, "repair"
		}
		mustExec(t, f.db, `INSERT INTO trace_box_qc_lines(company_id,qc_event_id,trace_box_id,product_id,checked_quantity,passed_quantity,rejected_quantity,rejection_reason,required_work) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, p.CompanyID, event, box, line.product, line.checked, line.checked-line.rejected, line.rejected, reason, work)
	}
	return event
}

func (f *analyticsFixture) requirement(t *testing.T, box, qc, product string, at time.Time) string {
	t.Helper()
	var requirement string
	mustScan(t, f.db, `INSERT INTO trace_box_work_requirements(company_id,trace_box_id,qc_event_id,product_id,quantity,work_type,rejection_reason,created_at) SELECT company_id,trace_box_id,qc_event_id,product_id,rejected_quantity,required_work,rejection_reason,$5 FROM trace_box_qc_lines WHERE company_id=$1 AND trace_box_id=$2 AND qc_event_id=$3 AND product_id=$4 RETURNING id`, []any{f.principal.CompanyID, box, qc, product, at}, &requirement)
	return requirement
}

func (f *analyticsFixture) complete(t *testing.T, box, requirement, employee string, at time.Time) {
	t.Helper()
	event := f.event(t, f.principal, box, "work_completed", at)
	mustExec(t, f.db, `INSERT INTO trace_box_work_completions(company_id,event_id,trace_box_id,work_requirement_id,completed_by_employee_id) VALUES($1,$2,$3,$4,$5)`, f.principal.CompanyID, event, box, requirement, employee)
}

func (f *analyticsFixture) gate(t *testing.T, box, employee, kind string, at time.Time, passed *bool) {
	t.Helper()
	event := f.event(t, f.principal, box, kind, at)
	mustExec(t, f.db, `INSERT INTO trace_box_gate_records(company_id,event_id,event_type,trace_box_id,employee_id,passed) VALUES($1,$2,$3,$4,$5,$6)`, f.principal.CompanyID, event, kind, box, employee, passed)
}

func (f *analyticsFixture) handover(t *testing.T, box string, sent, recordedAt time.Time, received *time.Time) {
	t.Helper()
	event := f.event(t, f.principal, box, "handover_sent", sent)
	var handover string
	mustScan(t, f.db, `INSERT INTO trace_box_handovers(company_id,trace_box_id,sent_event_id,target_employee_id,sent_at) VALUES($1,$2,$3,$4,$5) RETURNING id`, []any{f.principal.CompanyID, box, event, f.employees[0], recordedAt}, &handover)
	if received != nil {
		receipt := f.event(t, f.principal, box, "handover_received", *received)
		mustExec(t, f.db, `INSERT INTO trace_box_handover_receipts(company_id,event_id,trace_box_id,handover_id,received_by_employee_id) VALUES($1,$2,$3,$4,$5)`, f.principal.CompanyID, receipt, box, handover, f.employees[0])
	}
}

func analyticsTime(value string) time.Time {
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err) // Only fixed test literals call this helper.
	}
	return at
}

func assertAnalyticsRate(t *testing.T, got *float64, want float64) {
	t.Helper()
	if got == nil || math.Abs(*got-want) > 0.000001 {
		t.Fatalf("rate=%v, want %.8f", got, want)
	}
}

// Fingerprint all operational rows to guard the reporting boundary against writes,
// including inventory and audit side effects rather than checking event counts alone.
func (f *analyticsFixture) fingerprint(t *testing.T) []string {
	t.Helper()
	tables := []string{"trace_boxes", "trace_box_events", "trace_box_qc_checks", "trace_box_qc_lines", "trace_box_work_requirements", "trace_box_work_completions", "trace_box_handovers", "trace_box_handover_receipts", "trace_box_gate_records", "inventory_transactions", "audit_logs"}
	result := make([]string, 0, len(tables))
	for _, table := range tables {
		var fingerprint string
		mustScan(t, f.db, `SELECT md5(COALESCE(string_agg(row_to_json(t)::text,'' ORDER BY row_to_json(t)::text),'')) FROM `+table+` t`, nil, &fingerprint)
		result = append(result, fingerprint)
	}
	return result
}

func TestAnalyticsExactMetricsIsolationAndWorkforcePagination(t *testing.T) {
	f := setupAnalytics(t)
	from := analyticsTime("2026-09-01T00:00:00Z")
	filter := AnalyticsFilter{From: from, To: from.Add(24 * time.Hour), Timezone: "UTC", Limit: 100}
	box := f.box(t, f.principal, from.Add(-24*time.Hour))
	first := f.qc(t, f.principal, box, f.employees[0], from,
		analyticsQCLine{f.products[0], 10, 3, "damaged"}, analyticsQCLine{f.products[1], 4, 1, "dirty"})
	f.qc(t, f.principal, box, f.employees[0], from.Add(time.Hour),
		analyticsQCLine{f.products[0], 10, 0, ""}, analyticsQCLine{f.products[1], 4, 0, ""})
	f.qc(t, f.principal, box, f.employees[1], from.Add(2*time.Hour), analyticsQCLine{f.products[0], 5, 2, "damaged"})
	f.qc(t, f.principal, box, f.employees[0], from.Add(-time.Microsecond), analyticsQCLine{f.products[0], 100, 100, "other"})
	f.qc(t, f.principal, box, f.employees[0], filter.To, analyticsQCLine{f.products[0], 100, 100, "other"})
	otherBox := f.box(t, f.other, from.Add(-time.Hour))
	f.qc(t, f.other, otherBox, f.otherEmployee, from, analyticsQCLine{f.otherProduct, 999, 999, "other"})
	for i, product := range f.products {
		requirement := f.requirement(t, box, first, product, from)
		f.complete(t, box, requirement, f.employees[2], from.Add(time.Duration(i+3)*time.Hour))
	}
	pass, fail := true, false
	f.gate(t, box, f.employees[0], "final_check_completed", from.Add(5*time.Hour), &pass)
	f.gate(t, box, f.employees[1], "final_check_completed", from.Add(6*time.Hour), &fail)
	f.gate(t, box, f.employees[1], "final_check_completed", from.Add(7*time.Hour), &pass)
	before := f.fingerprint(t)
	report, err := f.service.Analytics(context.Background(), f.principal, filter)
	if err != nil {
		t.Fatal(err)
	}
	if report.Quality.Checks != 3 || report.Quality.CheckedQuantity != 33 || report.Quality.PassedQuantity != 27 || report.Quality.RejectedQuantity != 6 {
		t.Fatalf("multi-product/repeated QC totals=%#v", report.Quality)
	}
	assertAnalyticsRate(t, report.Quality.RejectionRatePercent, 600.0/33)
	if len(report.Daily) != 1 || report.Daily[0].Date != "2026-09-01" || !reflect.DeepEqual(report.Daily[0].QualityMetrics, report.Quality) {
		t.Fatalf("daily=%#v", report.Daily)
	}
	if len(report.Defects) != 2 {
		t.Fatalf("defects=%#v", report.Defects)
	}
	defects := map[string]DefectTrend{}
	for _, item := range report.Defects {
		defects[item.Reason] = item
	}
	if defects["damaged"].RejectedQuantity != 5 || defects["dirty"].RejectedQuantity != 1 || math.Abs(defects["damaged"].SharePercent-500.0/6) > 0.000001 {
		t.Fatalf("defects=%#v", defects)
	}
	if report.WorkforceTotal != 3 || len(report.Workforce) != 3 {
		t.Fatalf("workforce total=%d rows=%#v", report.WorkforceTotal, report.Workforce)
	}
	workers := map[string]WorkforceQuality{}
	var ids []string
	for _, worker := range report.Workforce {
		workers[worker.EmployeeID] = worker
		ids = append(ids, worker.EmployeeID)
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("unstable workforce order: %v", ids)
	}
	a, b, c := workers[f.employees[0]], workers[f.employees[1]], workers[f.employees[2]]
	if a.Checks != 2 || a.CheckedQuantity != 28 || a.RejectedQuantity != 4 || a.FinalChecks != 1 || a.FailedFinalChecks != 0 || a.CompletedWorkItems != 0 {
		t.Fatalf("first inspector=%#v", a)
	}
	assertAnalyticsRate(t, a.RejectionRatePercent, 400.0/28)
	assertAnalyticsRate(t, a.FinalFailureRatePercent, 0)
	if b.Checks != 1 || b.CheckedQuantity != 5 || b.RejectedQuantity != 2 || b.FinalChecks != 2 || b.FailedFinalChecks != 1 {
		t.Fatalf("second inspector=%#v", b)
	}
	assertAnalyticsRate(t, b.FinalFailureRatePercent, 50)
	if c.Checks != 0 || c.CompletedWorkItems != 2 || c.CompletedWorkQuantity != 4 || c.RejectionRatePercent != nil || c.FinalFailureRatePercent != nil {
		t.Fatalf("inactive worker with no inspections=%#v", c)
	}
	filter.Limit = 1
	for offset, want := range ids {
		filter.Offset = offset
		page, err := f.service.Analytics(context.Background(), f.principal, filter)
		if err != nil || page.WorkforceTotal != 3 || len(page.Workforce) != 1 || page.Workforce[0].EmployeeID != want || page.Quality.CheckedQuantity != 33 {
			t.Fatalf("page %d=%#v err=%v", offset, page, err)
		}
	}
	filter.Offset = 50
	page, err := f.service.Analytics(context.Background(), f.principal, filter)
	if err != nil || page.WorkforceTotal != 3 || len(page.Workforce) != 0 {
		t.Fatalf("offset beyond total=%#v err=%v", page, err)
	}
	filter.Limit, filter.Offset = 100, 0
	other, err := f.service.Analytics(context.Background(), f.other, filter)
	if err != nil || other.Quality.Checks != 1 || other.Quality.CheckedQuantity != 999 || len(other.Workforce) != 1 || other.Workforce[0].EmployeeID != f.otherEmployee {
		t.Fatalf("authorized other-company report=%#v err=%v", other, err)
	}
	if !reflect.DeepEqual(before, f.fingerprint(t)) {
		t.Fatal("analytics changed authoritative records")
	}
}

func TestAnalyticsCyclesUseSourceEventsAndFirstEverReadiness(t *testing.T) {
	f := setupAnalytics(t)
	from := analyticsTime("2026-09-01T00:00:00Z")
	filter := AnalyticsFilter{From: from, To: from.Add(24 * time.Hour), Timezone: "UTC", Limit: 100}
	box := f.box(t, f.principal, from.Add(-10*time.Hour))
	qc := f.qc(t, f.principal, box, f.employees[0], from.Add(-4*time.Hour), analyticsQCLine{f.products[0], 5, 2, "damaged"}, analyticsQCLine{f.products[1], 5, 3, "dirty"})
	first := f.requirement(t, box, qc, f.products[0], from.Add(-time.Hour))
	second := f.requirement(t, box, qc, f.products[1], from.Add(-time.Hour))
	f.complete(t, box, first, f.employees[0], from)
	f.complete(t, box, second, f.employees[0], from.Add(4*time.Hour))
	pendingQC := f.qc(t, f.principal, box, f.employees[0], from.Add(time.Hour), analyticsQCLine{f.products[0], 5, 5, "dirty"})
	f.requirement(t, box, pendingQC, f.products[0], from.Add(time.Hour))
	endQC := f.qc(t, f.principal, box, f.employees[0], from.Add(2*time.Hour), analyticsQCLine{f.products[0], 1, 1, "dirty"})
	endWork := f.requirement(t, box, endQC, f.products[0], from.Add(2*time.Hour))
	f.complete(t, box, endWork, f.employees[0], filter.To)
	received := from.Add(time.Hour)
	f.handover(t, box, from.Add(-time.Hour), from, &received)
	received = from.Add(6 * time.Hour)
	f.handover(t, box, from.Add(2*time.Hour), from.Add(3*time.Hour), &received)
	f.handover(t, box, from.Add(7*time.Hour), from.Add(7*time.Hour), nil)
	received = filter.To
	f.handover(t, box, from.Add(8*time.Hour), from.Add(8*time.Hour), &received)
	f.gate(t, box, f.employees[0], "ready_for_shipment", from.Add(2*time.Hour), nil)
	f.gate(t, box, f.employees[0], "ready_for_shipment", from.Add(5*time.Hour), nil)
	secondBox := f.box(t, f.principal, from.Add(-16*time.Hour))
	f.gate(t, secondBox, f.employees[0], "ready_for_shipment", from.Add(4*time.Hour), nil)
	priorReady := f.box(t, f.principal, from.Add(-48*time.Hour))
	f.gate(t, priorReady, f.employees[0], "ready_for_shipment", from.Add(-time.Hour), nil)
	f.gate(t, priorReady, f.employees[0], "ready_for_shipment", from.Add(3*time.Hour), nil)
	toReady := f.box(t, f.principal, from.Add(-time.Hour))
	f.gate(t, toReady, f.employees[0], "ready_for_shipment", filter.To, nil)
	f.box(t, f.principal, from) // An unfinished box contributes no completed cycle.
	report, err := f.service.Analytics(context.Background(), f.principal, filter)
	if err != nil {
		t.Fatal(err)
	}
	cycles := map[string]CycleTime{}
	for _, cycle := range report.Cycles {
		cycles[cycle.Kind] = cycle
	}
	for kind, want := range map[string][3]float64{"rework": {6, 6, 7.8}, "handover": {3, 3, 3.9}, "first_shipment_readiness": {16, 16, 19.6}} {
		cycle, ok := cycles[kind]
		if !ok || cycle.Samples != 2 {
			t.Fatalf("%s samples=%#v", kind, cycle)
		}
		assertAnalyticsRate(t, cycle.AverageHours, want[0])
		assertAnalyticsRate(t, cycle.MedianHours, want[1])
		assertAnalyticsRate(t, cycle.P95Hours, want[2])
	}
}

func TestAnalyticsTimezoneBoundariesAndDaylightSaving(t *testing.T) {
	f := setupAnalytics(t)
	box := f.box(t, f.principal, analyticsTime("2026-01-01T00:00:00Z"))
	cases := []struct {
		name, zone, from, to string
		times                []string
		quantities           []int64
		wantDays             map[string]int64
	}{
		{"offset midnight", "Asia/Kolkata", "2026-09-01T00:00:00+05:30", "2026-09-03T00:00:00+05:30",
			[]string{"2026-08-31T18:29:59Z", "2026-08-31T18:30:00Z", "2026-09-01T18:29:59Z", "2026-09-01T18:30:00Z", "2026-09-02T18:30:00Z"},
			[]int64{100, 2, 3, 5, 100}, map[string]int64{"2026-09-01": 5, "2026-09-02": 5}},
		{"spring daylight saving", "America/New_York", "2026-03-08T00:00:00-05:00", "2026-03-09T00:00:00-04:00",
			[]string{"2026-03-08T04:59:59Z", "2026-03-08T05:00:00Z", "2026-03-08T06:59:59Z", "2026-03-08T07:00:00Z", "2026-03-09T04:00:00Z"},
			[]int64{100, 2, 3, 5, 100}, map[string]int64{"2026-03-08": 10}},
		{"fall repeated hour", "America/New_York", "2026-11-01T00:00:00-04:00", "2026-11-02T00:00:00-05:00",
			[]string{"2026-11-01T03:59:59Z", "2026-11-01T04:00:00Z", "2026-11-01T05:30:00Z", "2026-11-01T06:30:00Z", "2026-11-02T05:00:00Z"},
			[]int64{100, 2, 3, 5, 100}, map[string]int64{"2026-11-01": 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i, at := range tc.times {
				f.qc(t, f.principal, box, f.employees[0], analyticsTime(at), analyticsQCLine{f.products[0], tc.quantities[i], 0, ""})
			}
			report, err := f.service.Analytics(context.Background(), f.principal, AnalyticsFilter{From: analyticsTime(tc.from), To: analyticsTime(tc.to), Timezone: tc.zone, Limit: 100})
			if err != nil || report.Quality.Checks != 3 || report.Quality.CheckedQuantity != 10 {
				t.Fatalf("boundary report=%#v err=%v", report, err)
			}
			days := map[string]int64{}
			for _, day := range report.Daily {
				days[day.Date] = day.CheckedQuantity
			}
			if !reflect.DeepEqual(days, tc.wantDays) {
				t.Fatalf("days=%v want=%v", days, tc.wantDays)
			}
		})
	}
}

func TestAnalyticsEmptyDenominatorsPermissionsAndHTTPBoundary(t *testing.T) {
	f := setupAnalytics(t)
	filter := AnalyticsFilter{From: analyticsTime("2026-09-01T00:00:00Z"), To: analyticsTime("2026-09-02T00:00:00Z"), Limit: 50}
	report, err := f.service.Analytics(context.Background(), f.principal, filter)
	if err != nil || report.Quality.Checks != 0 || report.Quality.RejectionRatePercent != nil || report.WorkforceTotal != 0 || len(report.Workforce) != 0 || len(report.Daily) != 0 || len(report.Defects) != 0 || report.Timezone != "UTC" || report.GeneratedAt.IsZero() || report.MetricVersion == "" {
		t.Fatalf("empty report=%#v err=%v", report, err)
	}
	if len(report.Cycles) != 3 {
		t.Fatalf("empty cycles=%#v", report.Cycles)
	}
	for _, cycle := range report.Cycles {
		if cycle.Samples != 0 || cycle.AverageHours != nil || cycle.MedianHours != nil || cycle.P95Hours != nil {
			t.Fatalf("empty cycle=%#v", cycle)
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(encoded), `"rejection_rate_percent":null`) || !strings.Contains(string(encoded), `"workforce":[]`) {
		t.Fatalf("empty JSON=%s err=%v", encoded, err)
	}
	for _, permission := range []string{"reports.view", "traceability.view"} {
		mustExec(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key=$3`, f.principal.CompanyID, f.role, permission)
		if _, err := f.service.Analytics(context.Background(), f.principal, filter); !errors.Is(err, authorization.ErrPermissionDenied) {
			t.Fatalf("missing %s: %v", permission, err)
		}
		mustExec(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,$3)`, f.principal.CompanyID, f.role, permission)
	}
	mustExec(t, f.db, `UPDATE module_entitlements SET enabled=false WHERE company_id=$1 AND module_key='traceability'`, f.principal.CompanyID)
	if _, err := f.service.Analytics(context.Background(), f.principal, filter); !errors.Is(err, authorization.ErrModuleUnavailable) {
		t.Fatalf("disabled traceability: %v", err)
	}
	mustExec(t, f.db, `UPDATE module_entitlements SET enabled=true WHERE company_id=$1 AND module_key='traceability'`, f.principal.CompanyID)
	handler := NewHTTPHandler(f.service)
	valid := url.Values{"from": {filter.From.Format(time.RFC3339)}, "to": {filter.To.Format(time.RFC3339)}}
	for _, tc := range []struct{ key, value string }{
		{"from", ""}, {"from", "yesterday"}, {"to", "2026-08-01T00:00:00Z"},
		{"limit", "NaN"}, {"limit", "0"}, {"limit", "101"}, {"offset", "-1"}, {"offset", "1000001"},
		{"timezone", "Local"}, {"timezone", "America/Not_A_Place"}, {"timezone", "UTC'); SELECT pg_sleep(20);--"},
	} {
		query := valid.Encode()
		values, _ := url.ParseQuery(query)
		values.Set(tc.key, tc.value)
		request := httptest.NewRequest(http.MethodGet, "/api/v1/reports/operations-analytics?"+values.Encode(), nil)
		request = request.WithContext(auth.WithPrincipal(request.Context(), f.principal))
		response := httptest.NewRecorder()
		handler.Analytics(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid %s=%q: status=%d body=%s", tc.key, tc.value, response.Code, response.Body.String())
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "/api/v1/reports/operations-analytics?"+valid.Encode(), nil)
		request = request.WithContext(auth.WithPrincipal(request.Context(), f.principal))
		response := httptest.NewRecorder()
		handler.Analytics(response, request)
		want := http.StatusOK
		if method != http.MethodGet {
			want = http.StatusMethodNotAllowed
		}
		if response.Code != want {
			t.Fatalf("%s status=%d body=%s", method, response.Code, response.Body.String())
		}
	}
	mustExec(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='reports.view'`, f.principal.CompanyID, f.role)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/reports/operations-analytics?"+valid.Encode(), nil)
	request = request.WithContext(auth.WithPrincipal(request.Context(), f.principal))
	response := httptest.NewRecorder()
	handler.Analytics(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("permission status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAnalyticsRepresentativeQueryPerformanceAndIndexRoundTrip(t *testing.T) {
	f := setupAnalytics(t)
	start := analyticsTime("2026-01-01T00:00:00Z")
	box := f.box(t, f.principal, start)
	// Twelve thousand two-product inspections span 125 days. A single-day report
	// exercises selective event access and aggregation over a realistic history size.
	mustExec(t, f.db, `INSERT INTO trace_box_events(company_id,trace_box_id,event_type,actor_user_id,idempotency_key,request_hash,created_at)
		SELECT $1,$2,'qc_completed',$3,'analytics-load-'||n,repeat('a',64),$4::timestamptz+n*interval '15 minutes' FROM generate_series(1,12000) n`, f.principal.CompanyID, box, f.principal.UserID, start)
	mustExec(t, f.db, `INSERT INTO trace_box_qc_checks(company_id,event_id,trace_box_id,checked_by_employee_id)
		SELECT company_id,id,trace_box_id,$2 FROM trace_box_events WHERE company_id=$1 AND event_type='qc_completed'`, f.principal.CompanyID, f.employees[0])
	for i, product := range f.products {
		checked, rejected := 10, 2
		if i == 1 {
			checked, rejected = 4, 1
		}
		mustExec(t, f.db, `INSERT INTO trace_box_qc_lines(company_id,qc_event_id,trace_box_id,product_id,checked_quantity,passed_quantity,rejected_quantity,rejection_reason,required_work)
			SELECT company_id,event_id,trace_box_id,$2,$3,$3::bigint-$4::bigint,$4,'damaged','repair' FROM trace_box_qc_checks WHERE company_id=$1`, f.principal.CompanyID, product, checked, rejected)
	}
	mustExec(t, f.db, `ANALYZE trace_box_events`)
	mustExec(t, f.db, `ANALYZE trace_box_qc_checks`)
	mustExec(t, f.db, `ANALYZE trace_box_qc_lines`)
	filter := AnalyticsFilter{From: analyticsTime("2026-04-01T00:00:00Z"), To: analyticsTime("2026-04-02T00:00:00Z"), Timezone: "UTC", Limit: 100}
	before := f.fingerprint(t)
	started := time.Now()
	report, err := f.service.Analytics(context.Background(), f.principal, filter)
	elapsed := time.Since(started)
	t.Logf("12,000 two-product QC events, selective daily report: %s", elapsed)
	if err != nil || elapsed >= 15*time.Second || report.Quality.Checks != 96 || report.Quality.CheckedQuantity != 1344 || report.Quality.RejectedQuantity != 288 || report.WorkforceTotal != 1 {
		t.Fatalf("representative report quality=%#v elapsed=%s err=%v", report.Quality, elapsed, err)
	}
	rows, err := f.db.Query(context.Background(), `EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM trace_box_events WHERE company_id=$1 AND created_at >= $2 AND created_at < $3 AND event_type='qc_completed'`, f.principal.CompanyID, filter.From, filter.To)
	if err != nil {
		t.Fatal(err)
	}
	var plan []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		plan = append(plan, line)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("selective source-event plan:\n%s", strings.Join(plan, "\n"))
	if !strings.Contains(strings.Join(plan, "\n"), "trace_box_events_company_time_idx") {
		t.Fatal("selective event range did not use the Phase 23 index")
	}
	if !reflect.DeepEqual(before, f.fingerprint(t)) {
		t.Fatal("analytics or query plan changed authoritative history")
	}
	for _, direction := range []string{"down", "up"} {
		path := "../../../migrations/000030_operations_analytics_indexes." + direction + ".sql"
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		mustExec(t, f.db, string(contents))
		var exists bool
		mustScan(t, f.db, `SELECT to_regclass('trace_box_events_company_time_idx') IS NOT NULL`, nil, &exists)
		if exists != (direction == "up") {
			t.Fatalf("index after %s: exists=%v", direction, exists)
		}
	}
	if !reflect.DeepEqual(before, f.fingerprint(t)) {
		t.Fatal("index migration roundtrip changed operational rows")
	}
}
