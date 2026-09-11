// These projections read immutable Traceability facts without changing domain state.
// Each activity is aggregated independently before workforce joins to avoid multiplying units.
package reporting

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const qualityCohort = `WITH qc AS (
 SELECT e.id,e.created_at,q.checked_by_employee_id,l.checked_quantity,l.passed_quantity,l.rejected_quantity,l.rejection_reason
 FROM trace_box_events e
 JOIN trace_box_qc_checks q ON q.company_id=e.company_id AND q.event_id=e.id
 JOIN trace_box_qc_lines l ON l.company_id=q.company_id AND l.qc_event_id=q.event_id
 WHERE e.company_id=$1 AND e.event_type='qc_completed' AND e.created_at >= $2 AND e.created_at < $3
) `

func loadQualityAnalytics(ctx context.Context, tx pgx.Tx, company string, f AnalyticsFilter, r *AnalyticsReport) error {
	err := tx.QueryRow(ctx, qualityCohort+`SELECT count(DISTINCT id),COALESCE(sum(checked_quantity),0),COALESCE(sum(passed_quantity),0),COALESCE(sum(rejected_quantity),0) FROM qc`, company, f.From, f.To).
		Scan(&r.Quality.Checks, &r.Quality.CheckedQuantity, &r.Quality.PassedQuantity, &r.Quality.RejectedQuantity)
	if err != nil {
		return fmt.Errorf("analytics QC summary: %w", err)
	}
	r.Quality.RejectionRatePercent = percentage(r.Quality.RejectedQuantity, r.Quality.CheckedQuantity)
	rows, err := tx.Query(ctx, qualityCohort+`SELECT (created_at AT TIME ZONE $4)::date::text,count(DISTINCT id),sum(checked_quantity),sum(passed_quantity),sum(rejected_quantity) FROM qc GROUP BY 1 ORDER BY 1`, company, f.From, f.To, f.Timezone)
	if err != nil {
		return fmt.Errorf("analytics daily quality: %w", err)
	}
	r.Daily, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (DailyQuality, error) {
		var day DailyQuality
		e := row.Scan(&day.Date, &day.Checks, &day.CheckedQuantity, &day.PassedQuantity, &day.RejectedQuantity)
		day.RejectionRatePercent = percentage(day.RejectedQuantity, day.CheckedQuantity)
		return day, e
	})
	if err != nil {
		return fmt.Errorf("read analytics daily quality: %w", err)
	}
	rows, err = tx.Query(ctx, qualityCohort+`SELECT rejection_reason,sum(rejected_quantity) FROM qc WHERE rejected_quantity>0 GROUP BY rejection_reason ORDER BY sum(rejected_quantity) DESC,rejection_reason`, company, f.From, f.To)
	if err != nil {
		return fmt.Errorf("analytics defect reasons: %w", err)
	}
	r.Defects, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (DefectTrend, error) {
		var defect DefectTrend
		e := row.Scan(&defect.Reason, &defect.RejectedQuantity)
		if share := percentage(defect.RejectedQuantity, r.Quality.RejectedQuantity); share != nil {
			defect.SharePercent = *share
		}
		return defect, e
	})
	if err != nil {
		return fmt.Errorf("read analytics defect reasons: %w", err)
	}
	return nil
}

// End events define the cohort; source timestamps are read from the complete history,
// including starts before the selected range. A box contributes only its first readiness.
const cycleCohort = `WITH durations AS (
 SELECT 'rework'::text kind,extract(epoch FROM (finish.created_at-start.created_at))/3600.0 hours
 FROM trace_box_events finish
 JOIN trace_box_work_completions c ON c.company_id=finish.company_id AND c.event_id=finish.id
 JOIN trace_box_work_requirements w ON w.company_id=c.company_id AND w.id=c.work_requirement_id
 JOIN trace_box_events start ON start.company_id=w.company_id AND start.id=w.qc_event_id
 WHERE finish.company_id=$1 AND finish.created_at >= $2 AND finish.created_at < $3
 UNION ALL
 SELECT 'handover',extract(epoch FROM (finish.created_at-start.created_at))/3600.0
 FROM trace_box_events finish
 JOIN trace_box_handover_receipts receipt ON receipt.company_id=finish.company_id AND receipt.event_id=finish.id
 JOIN trace_box_handovers h ON h.company_id=receipt.company_id AND h.id=receipt.handover_id
 JOIN trace_box_events start ON start.company_id=h.company_id AND start.id=h.sent_event_id
 WHERE finish.company_id=$1 AND finish.created_at >= $2 AND finish.created_at < $3
 UNION ALL
 SELECT 'first_shipment_readiness',extract(epoch FROM (finish.created_at-box.created_at))/3600.0
 FROM trace_box_events finish
 JOIN trace_boxes box ON box.company_id=finish.company_id AND box.id=finish.trace_box_id
 WHERE finish.company_id=$1 AND finish.event_type='ready_for_shipment'
 AND finish.created_at >= $2 AND finish.created_at < $3
 AND NOT EXISTS (
   SELECT 1 FROM trace_box_events earlier
   WHERE earlier.company_id=finish.company_id AND earlier.trace_box_id=finish.trace_box_id
   AND earlier.event_type='ready_for_shipment' AND (earlier.created_at,earlier.id)<(finish.created_at,finish.id)
 )
)
SELECT kinds.kind,count(d.hours),avg(d.hours)::double precision,
 percentile_cont(0.5) WITHIN GROUP (ORDER BY d.hours)::double precision,
 percentile_cont(0.95) WITHIN GROUP (ORDER BY d.hours)::double precision
FROM (VALUES ('rework',1),('handover',2),('first_shipment_readiness',3)) kinds(kind,position)
LEFT JOIN durations d ON d.kind=kinds.kind
GROUP BY kinds.kind,kinds.position ORDER BY kinds.position`

func loadCycleAnalytics(ctx context.Context, tx pgx.Tx, company string, f AnalyticsFilter, r *AnalyticsReport) error {
	rows, err := tx.Query(ctx, cycleCohort, company, f.From, f.To)
	if err != nil {
		return fmt.Errorf("analytics cycle times: %w", err)
	}
	r.Cycles, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (CycleTime, error) {
		var cycle CycleTime
		e := row.Scan(&cycle.Kind, &cycle.Samples, &cycle.AverageHours, &cycle.MedianHours, &cycle.P95Hours)
		return cycle, e
	})
	if err != nil {
		return fmt.Errorf("read analytics cycle times: %w", err)
	}
	return nil
}

const workforceCohort = qualityCohort + `, inspections AS (
 SELECT checked_by_employee_id employee_id,count(DISTINCT id) checks,sum(checked_quantity) checked,
 sum(passed_quantity) passed,sum(rejected_quantity) rejected FROM qc GROUP BY checked_by_employee_id
), work AS (
 SELECT c.completed_by_employee_id employee_id,count(*) items,sum(w.quantity) quantity
 FROM trace_box_events e
 JOIN trace_box_work_completions c ON c.company_id=e.company_id AND c.event_id=e.id
 JOIN trace_box_work_requirements w ON w.company_id=c.company_id AND w.id=c.work_requirement_id
 WHERE e.company_id=$1 AND e.created_at >= $2 AND e.created_at < $3 GROUP BY c.completed_by_employee_id
), finals AS (
 SELECT g.employee_id,count(*) checks,count(*) FILTER (WHERE NOT g.passed) failed
 FROM trace_box_events e JOIN trace_box_gate_records g ON g.company_id=e.company_id AND g.event_id=e.id
 WHERE e.company_id=$1 AND e.event_type='final_check_completed' AND e.created_at >= $2 AND e.created_at < $3
 GROUP BY g.employee_id
), workers AS (
 SELECT employee_id FROM inspections UNION SELECT employee_id FROM work UNION SELECT employee_id FROM finals
) `

func loadWorkforceAnalytics(ctx context.Context, tx pgx.Tx, company string, f AnalyticsFilter, r *AnalyticsReport) error {
	if err := tx.QueryRow(ctx, workforceCohort+`SELECT count(*) FROM workers`, company, f.From, f.To).Scan(&r.WorkforceTotal); err != nil {
		return fmt.Errorf("analytics workforce count: %w", err)
	}
	rows, err := tx.Query(ctx, workforceCohort+`
 SELECT employee.id,employee.display_name,COALESCE(i.checks,0),COALESCE(i.checked,0),COALESCE(i.passed,0),COALESCE(i.rejected,0),
 COALESCE(w.items,0),COALESCE(w.quantity,0),COALESCE(finals.checks,0),COALESCE(finals.failed,0)
 FROM workers JOIN employees employee ON employee.company_id=$1 AND employee.id=workers.employee_id
 LEFT JOIN inspections i ON i.employee_id=workers.employee_id
 LEFT JOIN work w ON w.employee_id=workers.employee_id
 LEFT JOIN finals ON finals.employee_id=workers.employee_id
 ORDER BY employee.id LIMIT $4 OFFSET $5`, company, f.From, f.To, f.Limit, f.Offset)
	if err != nil {
		return fmt.Errorf("analytics workforce: %w", err)
	}
	r.Workforce, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (WorkforceQuality, error) {
		var worker WorkforceQuality
		e := row.Scan(&worker.EmployeeID, &worker.EmployeeName, &worker.Checks, &worker.CheckedQuantity, &worker.PassedQuantity, &worker.RejectedQuantity,
			&worker.CompletedWorkItems, &worker.CompletedWorkQuantity, &worker.FinalChecks, &worker.FailedFinalChecks)
		worker.RejectionRatePercent = percentage(worker.RejectedQuantity, worker.CheckedQuantity)
		worker.FinalFailureRatePercent = percentage(worker.FailedFinalChecks, worker.FinalChecks)
		return worker, e
	})
	if err != nil {
		return fmt.Errorf("read analytics workforce: %w", err)
	}
	return nil
}
