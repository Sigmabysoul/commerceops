# Phase 19 — Myntra print-capture completion

Status: IMPLEMENTATION_IN_PROGRESS — PRINT_PAYLOAD_EVIDENCE_REQUIRED.
Revision: 2026-09-10.

## Goal and work

Capture actual browser/OS print payload; establish label/invoice structure, deterministic association and authoritative quantity before parsing or enrichment.

The owner authorized Phase 19 after deferring Phase 18's evidence-blocked work. The supplied
private Myntra packed-orders CSV is valid representative CSV evidence, but it contains no
quantity column and is not a browser/OS print payload. It can verify the existing Batch A
parser only; it cannot authorize PDF parsing, association, geometry or enrichment.

## Boundaries

Preserve CSV foundation and review-required behavior where quantity evidence is absent. Photos do not establish geometry.

## Acceptance and verification

Existing CSV regressions, sanitized real print fixtures, quantity/association conflicts and artifact traceability.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.
