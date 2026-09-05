package automation

import (
	"sort"
	"strings"
	"time"
	_ "time/tzdata"
)

type Schedule struct {
	Mode      string   `json:"mode,omitempty"`
	Times     []string `json:"times,omitempty"`
	Weekdays  []int    `json:"weekdays,omitempty"`
	StartDate string   `json:"start_date,omitempty"`
	EndDate   string   `json:"end_date,omitempty"`
}

func location(zone string) (*time.Location, error) {
	if zone == "" || zone == "Local" || len(zone) > 100 || strings.TrimSpace(zone) != zone {
		return nil, ErrInvalidInput
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, ErrInvalidInput
	}
	return loc, nil
}
func (s Schedule) validate() error {
	if s.Mode != "daily" && s.Mode != "weekdays" && s.Mode != "selected_weekdays" {
		return ErrInvalidInput
	}
	if len(s.Times) == 0 || len(s.Times) > 24 {
		return ErrInvalidInput
	}
	seen := map[string]bool{}
	for _, clock := range s.Times {
		if t, e := time.Parse("15:04", clock); e != nil || t.Format("15:04") != clock || seen[clock] {
			return ErrInvalidInput
		}
		seen[clock] = true
	}
	days := map[int]bool{}
	for _, d := range s.Weekdays {
		if d < 1 || d > 7 || days[d] {
			return ErrInvalidInput
		}
		days[d] = true
	}
	if s.Mode == "selected_weekdays" && len(days) == 0 || s.Mode != "selected_weekdays" && len(days) > 0 {
		return ErrInvalidInput
	}
	for _, d := range []string{s.StartDate, s.EndDate} {
		if d != "" {
			if t, e := time.Parse("2006-01-02", d); e != nil || t.Format("2006-01-02") != d {
				return ErrInvalidInput
			}
		}
	}
	if s.StartDate != "" && s.EndDate != "" && s.StartDate > s.EndDate {
		return ErrInvalidInput
	}
	return nil
}

// NextRun returns the earliest logical wall-clock occurrence strictly after
// after. Missing DST minutes are skipped; repeated minutes use their first
// instant only, even when after lies between the two physical instants.
func NextRun(s Schedule, zone string, after time.Time) (*time.Time, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	loc, err := location(zone)
	if err != nil {
		return nil, err
	}
	local := after.In(loc)
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if s.StartDate != "" && date.Format("2006-01-02") < s.StartDate {
		date, _ = time.Parse("2006-01-02", s.StartDate)
	}
	clocks := append([]string(nil), s.Times...)
	sort.Strings(clocks)
	for n := 0; n < 370; n++ { // every allowed weekly schedule has a candidate within this bound
		day := date.AddDate(0, 0, n)
		ds := day.Format("2006-01-02")
		if s.EndDate != "" && ds > s.EndDate {
			return nil, nil
		}
		wd := int(day.Weekday())
		if wd == 0 {
			wd = 7
		}
		allowed := s.Mode == "daily" || s.Mode == "weekdays" && wd <= 5
		if s.Mode == "selected_weekdays" {
			for _, d := range s.Weekdays {
				if d == wd {
					allowed = true
				}
			}
		}
		if !allowed {
			continue
		}
		for _, clock := range clocks {
			ct, _ := time.Parse("15:04", clock)
			wall := time.Date(day.Year(), day.Month(), day.Day(), ct.Hour(), ct.Minute(), 0, 0, time.UTC)
			// Discover offsets around the date rather than assuming a one-hour DST
			// shift (Lord Howe, for example, shifts by 30 minutes).
			offsets := map[int]bool{}
			for h := -36; h <= 36; h += 6 {
				_, offset := wall.Add(time.Duration(h) * time.Hour).In(loc).Zone()
				offsets[offset] = true
			}
			var earliest *time.Time
			for offset := range offsets {
				candidate := wall.Add(-time.Duration(offset) * time.Second)
				if candidate.In(loc).Format("2006-01-02 15:04") != ds+" "+clock {
					continue
				}
				if earliest == nil || candidate.Before(*earliest) {
					v := candidate
					earliest = &v
				}
			}
			if earliest != nil && earliest.After(after) {
				return earliest, nil
			}
		}
	}
	return nil, nil
}
