# Phase 25 — Production hardening and internal rollout

Status: Active implementation — owner authorized 2026-09-11; production rollout remains pending.
Revision: 2026-09-11.

## Goal and work

Complete security, observability, deployment, recovery, performance and operational readiness gates for internal rollout.

Phase 25 owns production configuration validation, shared HTTP boundary hardening, non-root
container packaging, release/recovery evidence, smoke/load measurement and the operator rollout
and rollback runbook. It adds no new business domain.

## Boundaries

Production readiness is not claimed by Phase 15. Define environment-specific rollout and rollback with measured evidence.

Local implementation evidence cannot substitute for the real production domain/TLS, PostgreSQL,
object storage, centralized logs, alert delivery, backup schedule or operator acceptance. The
repository must refuse unsafe configuration and mutable release references, but it must not
deploy automatically.

## Acceptance and verification

Full verification, load/crash tests, backup restore, deployment rollback and operator acceptance.

Completion requires production image builds, a migrated PostgreSQL-backed full gate, concurrent
readiness measurement, graceful shutdown/restart, Phase 24 recovery evidence, rollback rehearsal
and a clean committed tree. Actual internal rollout remains pending until an owner supplies the
environment and records acceptance.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.
