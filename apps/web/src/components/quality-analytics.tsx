"use client";

import { useEffect, useState } from "react";
import { AnalyticsReport, analyticsAPI } from "@/api/analytics";

type Preset = "today" | "yesterday" | "custom";

// Read-only reporting keeps all eligibility, attribution and metric calculations in Go.
export function QualityAnalytics() {
  const [preset, setPreset] = useState<Preset>("today");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [timezone, setTimezone] = useState("");
  const [offset, setOffset] = useState(0);
  const [refresh, setRefresh] = useState(0);
  const [report, setReport] = useState<AnalyticsReport | null>(null);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    const now = new Date();
    setFromDate(localDate(now));
    setToDate(localDate(addDays(now, 1)));
    setTimezone(Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC");
  }, []);

  useEffect(() => {
    if (!timezone) return;
    const controller = new AbortController();
    async function load() {
      setBusy(true);
      setError("");
      setReport(null);
      try {
        const [from, to] = dateRange(preset, fromDate, toDate);
        const result = await analyticsAPI.report(from, to, timezone, offset, controller.signal);
        if (!controller.signal.aborted) setReport(result);
      } catch (cause) {
        if (!controller.signal.aborted) {
          setError(cause instanceof Error ? cause.message : "Unable to load quality analytics.");
        }
      } finally {
        if (!controller.signal.aborted) setBusy(false);
      }
    }
    void load();
    return () => controller.abort();
  }, [preset, fromDate, toDate, timezone, offset, refresh]);

  return <section className="reporting" aria-labelledby="quality-analytics-heading">
    <div className="product-heading">
      <div><p className="eyebrow">Phase 23</p><h2 id="quality-analytics-heading">Quality and workforce analytics</h2>
        <p className="muted">Inspection volume, observed defects, completed work and elapsed workflow time.</p>
      </div>
      <button type="button" className="secondary" disabled={busy} onClick={() => setRefresh(value => value + 1)}>Refresh analytics</button>
    </div>
    <div className="report-filters panel">
      <label>Range<select value={preset} onChange={event => { setPreset(event.target.value as Preset); setOffset(0); }}>
        <option value="today">Today</option><option value="yesterday">Yesterday</option><option value="custom">Custom</option>
      </select></label>
      {preset === "custom" && <>
        <label>From<input type="date" value={fromDate} onChange={event => { setFromDate(event.target.value); setOffset(0); }} /></label>
        <label>To (exclusive)<input type="date" value={toDate} onChange={event => { setToDate(event.target.value); setOffset(0); }} /></label>
      </>}
      <p className="muted">Local dates in {timezone || "your browser timezone"}. Custom ranges can span up to 366 days; the end date is excluded.</p>
    </div>
    {busy && <p className="muted" role="status">Loading quality analytics…</p>}
    {error && <p className="error" role="alert">{error}</p>}
    {report && <>
      <p className="muted">{reportRange(report)} · Updated {new Date(report.generated_at).toLocaleString(undefined, { timeZone: report.timezone })} · Metric definition {report.metric_version}</p>
      <div className="metric-grid">
        <Metric label="QC inspections" value={number(report.quality.checks)} />
        <Metric label="Units inspected" value={number(report.quality.checked_quantity)} />
        <Metric label="Units rejected" value={number(report.quality.rejected_quantity)} />
        <Metric label="Rejection rate" value={percent(report.quality.rejection_rate_percent)} />
      </div>
      <p className="notice">Rejection rate measures defects detected during inspection, not who caused them. Repeated inspections count again in both the inspected and rejected quantities. Rates with no observations show “No sample”.</p>
      <div className="report-columns">
        <ReportTable title="Daily QC trend" headers={["Local date", "Inspections", "Inspected units", "Passed units", "Rejected units", "Rejection rate"]}
          rows={report.daily.map(day => ({ key: day.date, cells: [day.date, day.checks, day.checked_quantity, day.passed_quantity, day.rejected_quantity, percent(day.rejection_rate_percent)] }))}
          description="Dates with no inspections are omitted. Daily totals use the selected local timezone."
          empty="No inspections in this period." />
        <ReportTable title="Defects detected" headers={["Reason", "Rejected units", "Share of rejected units"]}
          rows={report.defects.map(defect => ({ key: defect.reason, cells: [defectLabel(defect.reason), defect.rejected_quantity, percent(defect.share_percent)] }))}
          empty="No rejected units in this period." />
      </div>
      <section className="panel report-block">
        <h3>Workflow cycle times</h3>
        <p className="muted">Workflows that finished in this period are included, even if they started earlier. Elapsed hours include waiting time. Each summary shows its sample count; these are workflow durations, not employee speed scores.</p>
        <div className="table-scroll"><table>
          <thead><tr><th scope="col">Workflow</th><th scope="col">Samples</th><th scope="col">Average hours</th><th scope="col">Median hours</th><th scope="col">95th percentile hours</th></tr></thead>
          <tbody>{report.cycles.map(cycle => <tr key={cycle.kind}>
            <th scope="row">{cycleLabel(cycle.kind)}<small>{cycleDescription(cycle.kind)}</small></th><td>{number(cycle.samples)}</td><td>{hours(cycle.average_hours)}</td><td>{hours(cycle.median_hours)}</td><td>{hours(cycle.p95_hours)}</td>
          </tr>)}</tbody>
        </table>{report.cycles.length === 0 && <p className="empty-state">No completed workflow samples in this period.</p>}</div>
      </section>
      <section className="panel report-block">
        <h3>Workforce activity</h3>
        <p className="muted">Inspection results belong to the employee who recorded the check and show detection workload, not fault attribution. Completed work and final checks are separate activities. Employees appear in a stable order, with no quality rankings.</p>
        <div className="table-scroll"><table>
          <thead><tr>
            <th scope="col">Employee</th><th scope="col">QC inspections</th><th scope="col">Inspected units</th><th scope="col">Rejected units detected</th><th scope="col">Rejection rate</th>
            <th scope="col">Completed work items</th><th scope="col">Completed work units</th><th scope="col">Final checks</th><th scope="col">Failed final checks</th><th scope="col">Final check failure rate</th>
          </tr></thead>
          <tbody>{report.workforce.map(employee => <tr key={employee.employee_id}>
            <th scope="row">{employee.employee_name}</th><td>{number(employee.checks)}</td><td>{number(employee.checked_quantity)}</td><td>{number(employee.rejected_quantity)}</td><td>{percent(employee.rejection_rate_percent)}</td>
            <td>{number(employee.completed_work_items)}</td><td>{number(employee.completed_work_quantity)}</td><td>{number(employee.final_checks)}</td><td>{number(employee.failed_final_checks)}</td><td>{percent(employee.final_failure_rate_percent)}</td>
          </tr>)}</tbody>
        </table>{report.workforce.length === 0 && <p className="empty-state">No employee activity on this page for the selected period.</p>}</div>
        <nav className="panel-actions" aria-label="Workforce report pages">
          <button type="button" className="secondary" disabled={report.offset === 0} onClick={() => setOffset(Math.max(0, report.offset - report.limit))}>Previous employees</button>
          <span>{report.workforce.length ? `${number(report.offset + 1)}–${number(report.offset + report.workforce.length)} of ${number(report.workforce_total)} employees` : `${number(report.workforce_total)} employees in this period`}</span>
          <button type="button" className="secondary" disabled={report.offset + report.limit >= report.workforce_total} onClick={() => setOffset(report.offset + report.limit)}>Next employees</button>
        </nav>
      </section>
    </>}
  </section>;
}

function Metric({ label, value }: { label: string; value: string }) {
  return <article><span>{label}</span><strong>{value}</strong></article>;
}

function ReportTable({ title, headers, rows, empty, description }: {
  title: string; headers: string[]; rows: { key: string; cells: (string | number)[] }[]; empty: string; description?: string;
}) {
  return <section className="panel report-block"><h3>{title}</h3>{description && <p className="muted">{description}</p>}<div className="table-scroll"><table>
    <thead><tr>{headers.map(header => <th scope="col" key={header}>{header}</th>)}</tr></thead>
    <tbody>{rows.map(row => <tr key={row.key}>{row.cells.map((cell, index) => <td key={headers[index]}>{typeof cell === "number" ? number(cell) : cell}</td>)}</tr>)}</tbody>
  </table>{rows.length === 0 && <p className="empty-state">{empty}</p>}</div></section>;
}

function number(value: number) { return value.toLocaleString(); }
function percent(value: number | null) { return value === null ? "No sample" : `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })}%`; }
function hours(value: number | null) { return value === null ? "No sample" : value.toLocaleString(undefined, { maximumFractionDigits: 2 }); }
function cycleLabel(kind: string) {
  return ({ rework: "Rework", handover: "Handover", first_shipment_readiness: "First shipment readiness" } as Record<string, string>)[kind] ?? kind;
}
function cycleDescription(kind: string) {
  return ({ rework: "Originating QC inspection to completed rework", handover: "Sent to received", first_shipment_readiness: "Box creation to its first ready-for-shipment event" } as Record<string, string>)[kind];
}
function defectLabel(reason: string) {
  return reason.replaceAll("_", " ").replace(/^./, character => character.toUpperCase());
}
function reportRange(report: AnalyticsReport) {
  const format = (value: string) => new Date(value).toLocaleString(undefined, { timeZone: report.timezone, dateStyle: "medium", timeStyle: "short" });
  return `${format(report.from)} to ${format(report.to)} (exclusive), ${report.timezone}`;
}
function localDate(value: Date) {
  return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, "0")}-${String(value.getDate()).padStart(2, "0")}`;
}
function addDays(value: Date, days: number) { const result = new Date(value); result.setDate(result.getDate() + days); return result; }
function dateRange(preset: Preset, fromDate: string, toDate: string): [Date, Date] {
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const [from, to] = preset === "today" ? [today, addDays(today, 1)]
    : preset === "yesterday" ? [addDays(today, -1), today]
    : [new Date(`${fromDate}T00:00:00`), new Date(`${toDate}T00:00:00`)];
  if (!Number.isFinite(from.getTime()) || !Number.isFinite(to.getTime()) || from >= to) {
    throw new Error("Choose a valid start date and a later end date. The end date is excluded.");
  }
  if (to.getTime() - from.getTime() > 366 * 24 * 60 * 60 * 1000) {
    throw new Error("Choose a date range of no more than 366 days.");
  }
  return [from, to];
}
