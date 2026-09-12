package automation

import (
	"testing"
	"time"
)

func TestScheduleBoundaries(t *testing.T) {
	cases := []struct {
		name, zone, after, want string
		s                       Schedule
	}{
		{"India day boundary", "Asia/Kolkata", "2026-09-04T23:00:00Z", "2026-09-05T03:30:00Z", Schedule{Mode: "daily", Times: []string{"09:00"}}},
		{"weekend", "UTC", "2026-09-04T10:00:00Z", "2026-09-07T09:00:00Z", Schedule{Mode: "weekdays", Times: []string{"09:00"}}},
		{"selected days and sorted times", "UTC", "2026-09-04T10:00:00Z", "2026-09-06T08:00:00Z", Schedule{Mode: "selected_weekdays", Weekdays: []int{7}, Times: []string{"17:00", "08:00"}}},
		{"spring gap skipped", "America/New_York", "2026-03-08T00:00:00Z", "2026-03-09T06:30:00Z", Schedule{Mode: "daily", Times: []string{"02:30"}}},
		{"fall first instant", "America/New_York", "2026-11-01T00:00:00Z", "2026-11-01T05:30:00Z", Schedule{Mode: "daily", Times: []string{"01:30"}}},
		{"fall repeated instant excluded", "America/New_York", "2026-11-01T05:45:00Z", "2026-11-02T06:30:00Z", Schedule{Mode: "daily", Times: []string{"01:30"}}},
		{"half-hour spring gap", "Australia/Lord_Howe", "2026-10-03T13:00:00Z", "2026-10-04T15:15:00Z", Schedule{Mode: "daily", Times: []string{"02:15"}}},
		{"inclusive future start", "UTC", "2026-01-01T00:00:00Z", "2030-01-01T09:00:00Z", Schedule{Mode: "daily", Times: []string{"09:00"}, StartDate: "2030-01-01", EndDate: "2030-01-01"}},
		{"expired", "UTC", "2026-09-05T10:00:00Z", "", Schedule{Mode: "daily", Times: []string{"09:00"}, EndDate: "2026-09-05"}},
		{"exact minute not repeated", "UTC", "2026-09-05T09:00:00Z", "2026-09-05T10:00:00Z", Schedule{Mode: "daily", Times: []string{"09:00", "10:00"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			after, e := time.Parse(time.RFC3339, c.after)
			if e != nil {
				t.Fatal(e)
			}
			got, e := NextRun(c.s, c.zone, after)
			if e != nil {
				t.Fatal(e)
			}
			if c.want == "" {
				if got != nil {
					t.Fatalf("want no occurrence, got %v", got)
				}
				return
			}
			if got == nil || got.Format(time.RFC3339) != c.want {
				t.Fatalf("got %v want %s", got, c.want)
			}
		})
	}
}
func TestScheduleValidation(t *testing.T) {
	for _, s := range []Schedule{{}, {Mode: "daily", Times: []string{"9:00"}}, {Mode: "daily", Times: []string{"24:00"}}, {Mode: "daily", Times: []string{"09:00", "09:00"}}, {Mode: "selected_weekdays", Times: []string{"09:00"}}, {Mode: "selected_weekdays", Times: []string{"09:00"}, Weekdays: []int{0}}, {Mode: "daily", Times: []string{"09:00"}, StartDate: "2026-02-30"}, {Mode: "daily", Times: []string{"09:00"}, StartDate: "2026-09-06", EndDate: "2026-09-05"}} {
		if s.validate() == nil {
			t.Fatalf("accepted %#v", s)
		}
	}
	for _, zone := range []string{"", "Local", "Wrong/Zone", " UTC"} {
		if _, err := location(zone); err == nil {
			t.Fatalf("accepted timezone %q", zone)
		}
	}
}
