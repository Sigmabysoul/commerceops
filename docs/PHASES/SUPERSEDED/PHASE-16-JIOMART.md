# Superseded planning reference

Preserved from the pre-consolidation working tree on 2026-09-07. Phase numbers and authorization statements below are superseded by `../../ROADMAP.md`. This file does not authorize implementation.

# Phase 16 — JioMart

**Status:** PLANNED / NOT IMPLEMENTED
**Depends on:** Phase 15 Marketplace Seller Accounts

## Goal

Add JioMart as an evidence-backed marketplace adapter using the shared CommerceOps
ingestion → Product Master → Batch → Printing → Inventory → Returns pipeline.

## Initial account configuration

Current known internal deployment uses Brothers only.

This is configuration, not source-code logic.

## Evidence gate

Before coding the adapter collect:
- seller portal workflow
- downloadable files/API/print behavior
- representative label/invoice/manifest samples
- exact order/AWB/SKU/quantity fields
- batch/bulk-print behavior
- reprint behavior

Do not infer implementation from other marketplaces.

## Scope

- marketplace registry/account readiness
- evidence-backed source ingestion
- normalized order/item
- Product Master mapping
- Batch
- Printing
- Inventory outbound boundary
- Returns
- Reporting
- tenant/account isolation

No JioMart-specific Inventory/Returns/reporting tables.
