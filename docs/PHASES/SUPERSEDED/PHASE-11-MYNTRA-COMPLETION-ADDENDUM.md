# Superseded planning reference

Preserved from the pre-consolidation working tree on 2026-09-07. Phase numbers and authorization statements below are superseded by `../../ROADMAP.md`. This file does not authorize implementation.

# Phase 11 — Myntra Completion Addendum

**Status:** PHASE 11 BATCH A IMPLEMENTED / PRINT-CAPTURE COMPLETION PENDING

This addendum does not create a new phase. It completes the evidence-deferred
portion of Phase 11.

## Existing implemented foundation

- packed-orders CSV ingestion
- Order id
- Tracking_id
- Seller_sku_code
- Myntra SKU evidence
- Store Packet ID / Order Release metadata
- Product Master resolution where sufficient
- shared tenant/permission/audit/Returns/reporting integration

## New real-world evidence

Real photographs show:
- Myntra shipping label
- invoice
- tracking/AWB area
- invoice PacketID/order data
- seller SKU/product code
- explicit Qty column
- usable blank region on shipping label

However the portal reportedly provides a Print action rather than a normal PDF
download.

## Required next evidence

1. attempt browser Save to PDF / Print to File
2. if unavailable, capture using controlled virtual CUPS printer/spool
3. record actual MIME/format
4. inspect page structure
5. determine exact shared association identifiers
6. determine explicit SKU and quantity extraction
7. sanitize a regression fixture
8. only then implement enrichment geometry

Do not use photographs alone to hardcode coordinates.

## Target architecture

Myntra seller portal
→ browser/OS print path
→ controlled CommerceOps print capture
→ Myntra processor
→ normalized order/item
→ Product Master
→ Batch
→ enriched print artifact
→ canonical Print Job
→ Printer Agent
→ physical printer

## Safety

- no browser-network scraping as the primary architecture unless separately approved
- no guessed page adjacency
- no default quantity = 1
- no Inventory side effects from capture/parse/print/reprint
- ambiguous account/document association → review
