// This file defines read-only operational metrics. Inspections describe observed quality,
// not who caused a defect; every rate keeps its measured workload denominator visible.
package reporting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
)

type AnalyticsFilter struct {
	From     time.Time
	To       time.Time
	Timezone string
	Limit    int
	Offset   int
}

type QualityMetrics struct {
	Checks               int64    `json:"checks"`
	CheckedQuantity      int64    `json:"checked_quantity"`
	PassedQuantity       int64    `json:"passed_quantity"`
	RejectedQuantity     int64    `json:"rejected_quantity"`
	RejectionRatePercent *float64 `json:"rejection_rate_percent"`
}

type DailyQuality struct {
	Date string `json:"date"`
	QualityMetrics
}

type DefectTrend struct {
	Reason           string  `json:"reason"`
	RejectedQuantity int64   `json:"rejected_quantity"`
	SharePercent     float64 `json:"share_percent"`
}

type CycleTime struct {
	Kind         string   `json:"kind"`
	Samples      int64    `json:"samples"`
	AverageHours *float64 `json:"average_hours"`
	MedianHours  *float64 `json:"median_hours"`
	P95Hours     *float64 `json:"p95_hours"`
}

type WorkforceQuality struct {
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
	QualityMetrics
	CompletedWorkItems      int64    `json:"completed_work_items"`
	CompletedWorkQuantity   int64    `json:"completed_work_quantity"`
	FinalChecks             int64    `json:"final_checks"`
	FailedFinalChecks       int64    `json:"failed_final_checks"`
	FinalFailureRatePercent *float64 `json:"final_failure_rate_percent"`
}

type AnalyticsReport struct {
	From           time.Time          `json:"from"`
	To             time.Time          `json:"to"`
	Timezone       string             `json:"timezone"`
	GeneratedAt    time.Time          `json:"generated_at"`
	MetricVersion  string             `json:"metric_version"`
	Quality        QualityMetrics     `json:"quality"`
	Daily          []DailyQuality     `json:"daily"`
	Defects        []DefectTrend      `json:"defects"`
	Cycles         []CycleTime        `json:"cycles"`
	Workforce      []WorkforceQuality `json:"workforce"`
	WorkforceTotal int64              `json:"workforce_total"`
	Limit          int                `json:"limit"`
	Offset         int                `json:"offset"`
}

func validateAnalyticsFilter(f AnalyticsFilter) (AnalyticsFilter, error) {
	f.Timezone = strings.TrimSpace(f.Timezone)
	if f.Timezone == "" {
		f.Timezone = "UTC"
	}
	if f.From.IsZero() || f.To.IsZero() || !f.From.Before(f.To) || f.To.Sub(f.From) > 366*24*time.Hour ||
		f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 1000000 || len(f.Timezone) > 100 || f.Timezone == "Local" {
		return f, ErrInvalidRange
	}
	if _, err := time.LoadLocation(f.Timezone); err != nil {
		return f, ErrInvalidRange
	}
	return f, nil
}

// Analytics uses one read-only repeatable snapshot so pagination totals, rates and
// summaries cannot observe different concurrent workflow commits within a response.
func (s *Service) Analytics(ctx context.Context, p auth.Principal, f AnalyticsFilter) (AnalyticsReport, error) {
	for _, permission := range []string{"reports.view", "traceability.view"} {
		if err := s.authorizer.RequirePermission(ctx, p, permission); err != nil {
			return AnalyticsReport{}, err
		}
	}
	if err := s.authorizer.RequireModule(ctx, p, "traceability"); err != nil {
		return AnalyticsReport{}, err
	}
	f, err := validateAnalyticsFilter(f)
	if err != nil {
		return AnalyticsReport{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return AnalyticsReport{}, fmt.Errorf("begin analytics snapshot: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // A successful commit has already closed the transaction.
	r := AnalyticsReport{From: f.From, To: f.To, Timezone: f.Timezone, MetricVersion: "traceability-v1", Limit: f.Limit, Offset: f.Offset,
		Daily: []DailyQuality{}, Defects: []DefectTrend{}, Cycles: []CycleTime{}, Workforce: []WorkforceQuality{}}
	var timezoneExists bool
	if err = tx.QueryRow(ctx, `SELECT transaction_timestamp(),EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=$1)`, f.Timezone).Scan(&r.GeneratedAt, &timezoneExists); err != nil {
		return AnalyticsReport{}, fmt.Errorf("analytics timezone: %w", err)
	}
	if !timezoneExists {
		return AnalyticsReport{}, ErrInvalidRange
	}
	if err = loadQualityAnalytics(ctx, tx, p.CompanyID, f, &r); err != nil {
		return AnalyticsReport{}, err
	}
	if err = loadCycleAnalytics(ctx, tx, p.CompanyID, f, &r); err != nil {
		return AnalyticsReport{}, err
	}
	if err = loadWorkforceAnalytics(ctx, tx, p.CompanyID, f, &r); err != nil {
		return AnalyticsReport{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AnalyticsReport{}, fmt.Errorf("commit analytics snapshot: %w", err)
	}
	return r, nil
}

func percentage(numerator, denominator int64) *float64 {
	if denominator == 0 {
		return nil
	}
	value := 100 * float64(numerator) / float64(denominator)
	return &value
}
