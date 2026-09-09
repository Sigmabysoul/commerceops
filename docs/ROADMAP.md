# CommerceOps Roadmap

**Current approved implementation baseline:** Phase 14 — Printing Automation  
**Approved commit:** `d90e4f7c0ceab032bc33a0618e1ceabdc20906b5`

This roadmap separates implemented phases from future authorized phases.

## Completed / approved

- Phase 0 — Foundation
- Phase 1 — Core Platform
- Phase 2 — Product Master
- Phase 3 — Flipkart Processing
- Phase 4 — Batch + Printing
- Phase 5 — Inventory
- Phase 6 — Dashboard + Reporting
- Phase 7 — Amazon
- Phase 8 — Returns & Cancellations
- Phase 9 — Consignment
- Phase 10 — Meesho
- Phase 11 — Myntra Batch A (CSV foundation; print-payload completion pending)
- Phase 12 — Snapdeal
- Phase 13 — Printing Platform / Printer Agent
- Phase 14 — Printing Automation

## Next authorized planning sequence

### Phase 15 — Multi-ID Business Model / Marketplace Seller Accounts
First-class seller accounts under one tenant; account-aware SKU mappings, provenance,
dedupe, Returns, reporting and workstation routing.

### Phase 11 Completion — Myntra Print Capture
Remain under Phase 11 because this is completion of deferred Myntra behavior, not
a new marketplace phase. Capture actual browser/OS print payload before implementing
layout/parser/enrichment.

### Phase 16 — JioMart
Add JioMart evidence-backed ingestion after Marketplace Accounts are available.

### Phase 17 — Return Rework & Box Traceability
Opaque QR Trace Boxes, QC, handovers, rework/sticker work, packing, final check,
Returns and Consignment integration.

### Phase 18 — Admin / Role Dashboards
Full Owner dashboard for the initial three owners, Developer/Test Admin dashboard,
restricted HR/TL/Worker workspaces. Authorization remains permission-driven.

### Phase 19 — Operations Analytics & Gamification
Defect trends, cycle time, employee workload-normalized quality metrics and only
later badges/leaderboards after reliable data exists.

## Stabilization between phases

Before starting Phase 15 implementation, run the current Phase 14 application locally
on Fedora, test screen-by-screen, collect UX/bug feedback and verify the real operator
workflow.

See `docs/LOCAL_TESTING_FEDORA.md`.

No AI agent may automatically start the next phase without explicit owner authorization.
