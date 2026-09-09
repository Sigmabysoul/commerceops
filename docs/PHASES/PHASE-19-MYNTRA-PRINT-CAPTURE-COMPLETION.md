# Phase 19 — Myntra print-capture completion

Status: EVIDENCE_PREPARATION_VERIFIED — PRINT_PAYLOAD_RUNTIME_WORK_DEFERRED.
Revision: 2026-09-10.

## Goal and work

Capture actual browser/OS print payload; establish label/invoice structure, deterministic association and authoritative quantity before parsing or enrichment.

The owner authorized Phase 19 after deferring Phase 18's evidence-blocked work. The supplied
private Myntra packed-orders CSV is valid representative CSV evidence, but it contains no
quantity column and is not a browser/OS print payload. It can verify the existing Batch A
parser only; it cannot authorize PDF parsing, association, geometry or enrichment.

The optional private-fixture regression parsed all 34 supplied rows and confirmed that no
quantity column has entered the source contract. The capture procedure is documented in
`../marketplaces/MYNTRA_PRINT_CAPTURE.md`. Runtime PDF work remains on the deferred evidence
backlog; this is not Phase 19 implementation completion.

## Boundaries

Preserve CSV foundation and review-required behavior where quantity evidence is absent. Photos do not establish geometry.

## Acceptance and verification

Existing CSV regressions, sanitized real print fixtures, quantity/association conflicts and artifact traceability.

Evidence-preparation verification on 2026-09-10 passed `make verify-full` against a fresh
disposable PostgreSQL database migrated through `000026`, with the private Myntra CSV enabled.
Backend formatting, vet, uncached PostgreSQL-backed tests, both executable builds, frontend
typecheck, lint, production build and repository checks passed. Browser/OS print capture,
sanitized print fixtures, PDF parsing, print generation and physical printer behavior were not
tested because the print payload is unavailable.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.
