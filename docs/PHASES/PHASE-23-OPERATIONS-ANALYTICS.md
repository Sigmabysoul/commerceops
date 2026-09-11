# Phase 23 — Operations and workforce analytics

Status: Implementation active — owner authorized continuation after Phase 22 completion on 2026-09-11.
Revision: 2026-09-11.

## Goal and work

Derive defect trends, cycle times and workload-normalized workforce quality measures from authoritative operational records.

## Boundaries

Permission-scoped reproducible metrics; no unrelated mutable counters. Gamification is deferred until reliable data and a separately approved design exist.

## Acceptance and verification

Metric fixtures, timezone boundaries, pagination, authorization and representative query performance.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.

## Implementation contract

- Reporting owns read-only queries over immutable Traceability events and typed QC, rework,
  handover and gate records. `GET /api/v1/reports/operations-analytics` requires `reports.view`,
  `traceability.view` and the Traceability entitlement.
- Inspection workload is full-check count and checked/passed/rejected units. Rejection rates
  use inspected-unit denominators. Repeated QC counts as repeated inspection work; the inspector
  records observed defects and is not assumed responsible for causing them.
- Daily QC trend buckets use a validated IANA timezone. Reason shares use all rejected
  inspection units in the selected period as their denominator.
- Elapsed rework and handover times pair their recorded start and completion events. First
  shipment readiness pairs box creation with its first-ever readiness. Only completions in the
  selected period contribute; starts can precede it. Counts, mean, median and p95 are returned.
- Workforce pages retain recorded employee IDs, including inactive employees, and independently
  aggregate inspection workload, completed requirement units and final-check outcomes. These are
  activity measures, not a blended ranking or productivity-per-hour score.
- One read-only repeatable-read transaction provides a consistent response snapshot. Ranges
  use `[from,to)` instants, at most 366 days; employee-ID pagination is stable within a fixed
  population. Null rates/durations distinguish missing observations from measured zero.
- Migration `000030` adds only a company/time event index. No mutable counters, source-record
  writes, framework/dependency changes or Inventory behavior changes are introduced.

Definitions and known data limits are in [the reporting workflow](../workflows/reporting.md).
