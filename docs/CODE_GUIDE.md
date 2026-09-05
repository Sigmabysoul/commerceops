# CommerceOps code guide

This guide explains where to look before changing behavior. Source comments are
reserved for non-obvious invariants and failure semantics; this document gives
the broader map without repeating every line of code.

## Request path

```text
Next.js component
  → typed file in apps/web/src/api
  → /api/v1 route wired by internal/app/app.go
  → small HTTP adapter in the owning domain
  → domain service and PostgreSQL transaction
  → audit/object storage/another explicit domain boundary when required
```

The session middleware establishes `auth.Principal`. Its `CompanyID` is the
only tenant identity business services use. Frontend-supplied company IDs are
never authorization.

## Backend map

- `cmd/server`: loads validated configuration and starts the modular monolith.
- `cmd/printer-agent`: local Phase 13 process; it has no business-domain logic.
- `internal/app`: dependency and route wiring. Domain rules do not belong here.
- `internal/auth`, `internal/authorization`: sessions, principals, live role
  permissions, and module entitlements.
- `internal/core`: companies, employees, roles, access, settings, and audit view.
- `internal/product`: canonical Product Master and exact marketplace SKU maps.
- `internal/marketplace`: shared upload/job/normalization orchestration. Each
  marketplace subpackage owns only its evidence-specific parsing/printing rules.
- `internal/batch`: order grouping, assignment snapshots, PDF generation records,
  immutable artifacts, and document-level reprints.
- `internal/printing`: reusable assets, agents/printers, and every physical print
  request. It must never depend on Inventory.
- `internal/printeragent`: polling client, hash verification, durable at-most-once
  journal, and OS backend interface. `cups.go` is the Linux implementation seam.
- `internal/inventory`: the only authority for balances, reservations, and ledger
  mutations.
- `internal/returns`, `internal/consignment`: lifecycle owners that call explicit
  Inventory boundaries only at approved stock transitions.
- `internal/reporting`: read models derived from authoritative domain tables.
- `internal/platform`: database, HTTP envelope, object storage, PDF extraction,
  and PDF generation infrastructure without business ownership.

## Printing terms that look similar

- `print_jobs`: creates a PDF from a ready ecommerce batch.
- `print_artifacts`: immutable generated label/invoice PDF metadata.
- `printer_jobs`: sends one immutable PDF to one registered physical printer.
- `printer_job_events`: append-only physical-delivery history.
- `print_library_assets`: reusable uploaded PDFs used by Quick Print.

Keeping these separate is intentional. Regenerating or physically printing a
PDF is never an Inventory event.

## Frontend map

- `apps/web/src/api`: typed REST boundary. Components should not guess response
  shapes or contain authoritative domain rules.
- `apps/web/src/components/core-platform.tsx`: authenticated workspace assembly.
- Marketplace processing components: upload and poll shared backend jobs.
- `batch-printing.tsx`: creates batches and immutable PDFs; Phase 13 adds the
  explicit action that queues an artifact to hardware.
- `quick-print.tsx`: mobile Print Library browsing and server-side queue request.
- `pwa-registration.tsx` and `app/manifest.ts`: installability only; authenticated
  API data is not cached and the phone never accesses a printer.

## Database and files

Migrations are ordered under `services/api/migrations`; startup never applies
them. Binary PDFs live behind `internal/platform/objectstorage`. Tables store
server-generated keys, hashes, sizes, and traceability—not PDF blobs.

## Safe change checklist

1. Identify the owning domain and read its module/workflow documentation.
2. Preserve server-established tenant scope and granular permissions.
3. Keep marketplace evidence parsing isolated and quantities explicit.
4. Route every stock mutation through Inventory with an auditable reason.
5. Treat reprints and all physical printing as Inventory-neutral.
6. Add a regression test, update OpenAPI/docs, and run `make verify-full` against
   a migrated disposable PostgreSQL database.
