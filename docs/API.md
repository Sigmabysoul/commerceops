# API conventions

CommerceOps exposes REST endpoints under `/api/v1`. JSON responses use `application/json`.

Errors use a stable envelope and never include internal SQL errors, credentials, host details, or stack traces:

```json
{"error":{"code":"INTERNAL_ERROR","message":"Something went wrong"}}
```

## Health

`GET /api/v1/health` checks the required PostgreSQL dependency.

- HTTP 200: `{"status":"ok","database":"ok"}`
- HTTP 503: `{"status":"unavailable","database":"unavailable"}`

Other methods receive HTTP 405 with the standard error envelope. The endpoint is intentionally tenant-independent and exposes no sensitive dependency details.

## Core Platform

Authentication uses an opaque server-side session in an `HttpOnly`, `SameSite=Lax` cookie. Except for login and health, all endpoints require that session. Login's `company_id` selects one of the user's company memberships; the server verifies that membership and establishes the tenant stored on the session. Tenant APIs never accept a company identifier.

| Method | Path | Required permission | Purpose |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/login` | Public | Start a company-scoped session |
| POST | `/api/v1/auth/logout` | Authenticated | Revoke the current session |
| GET | `/api/v1/auth/session` | Authenticated | Validate the current session |
| GET | `/api/v1/company` | Authenticated | Read the session company |
| GET, POST | `/api/v1/employees` | `employees.view`, `employees.manage` | List or create employees |
| PATCH | `/api/v1/employees/{employee_id}` | `employees.manage` | Change employee status |
| POST | `/api/v1/user-access` | `employees.manage` | Create a login and grant company access |
| PATCH | `/api/v1/user-access/{user_id}` | `employees.manage` | Change company access status |
| PUT | `/api/v1/user-access/{user_id}/roles` | `roles.manage` | Replace company role assignments |
| GET, POST | `/api/v1/roles` | `roles.view`, `roles.manage` | List or create company roles |
| PUT | `/api/v1/roles/{role_id}/permissions` | `roles.manage` | Replace role permissions |
| GET | `/api/v1/permissions` | `roles.view` | List permission definitions |
| GET | `/api/v1/module-entitlements` | `settings.manage` | List company module access |
| PUT | `/api/v1/module-entitlements/{module_key}` | `settings.manage` | Enable or disable a module |
| GET | `/api/v1/audit-logs` | `settings.manage` | Read recent company audit entries |

The `core` entitlement is always enabled. Entitlements represent technical module access only and contain no billing or pricing behavior.

## Product Master

Product Master endpoints use only the authenticated session company. No request accepts `company_id`.

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| GET | `/api/v1/marketplaces` | `products.view` | List normalized marketplace reference keys |
| GET, POST | `/api/v1/products` | `products.view`, `products.manage` | Search/list or create canonical products |
| GET, PATCH | `/api/v1/products/{product_id}` | `products.view`, `products.manage` | Read or update a product and its lifecycle status |
| GET, POST | `/api/v1/sku-mappings` | `products.view`, `products.manage` | List or manually train SKU mappings |
| PATCH | `/api/v1/sku-mappings/{mapping_id}` | `products.manage` | Edit or deactivate a mapping |
| POST | `/api/v1/sku-mappings/resolve` | `products.view` | Resolve one exact marketplace/SKU identifier |

SKU resolution trims surrounding whitespace and then performs a case-sensitive exact match within the authenticated company and marketplace. It never performs fuzzy, substring, case-insensitive, or fallback matching. A successful lookup returns `status: "resolved"` with its mapping and product; every unknown, inactive, or differently-cased identifier returns `status: "unresolved"` without guessing.

The OpenAPI source is `docs/openapi.yaml`. It must be updated whenever the public API contract changes.

## Consignment management

Consignment endpoints require the `consignments` entitlement. Broad readers
use `consignments.view`; managers use `consignments.manage`; department workers
use `consignments.work` and are server-scoped through their company employee
membership; stock confirmation uses `consignments.outbound`. No endpoint
accepts `company_id`.

| Method | Path | Purpose |
| --- | --- | --- |
| GET, POST | `/api/v1/consignment-departments` | List visible or create configurable departments |
| PATCH | `/api/v1/consignment-departments/{department_id}` | Rename or activate/deactivate a department |
| PUT | `/api/v1/consignment-departments/{department_id}/members` | Replace active employee membership |
| GET, POST | `/api/v1/consignments` | Filter visible work or idempotently create an SO-linked consignment |
| GET | `/api/v1/consignments/{consignment_id}` | Read visible lines and broad audit history |
| POST | `/api/v1/consignments/{consignment_id}/allocate` | Reserve canonical product requirements |
| POST | `/api/v1/consignments/{consignment_id}/transition` | Apply an allowed non-inventory state transition |
| POST | `/api/v1/consignments/{consignment_id}/lines/{line_id}/progress` | Optimistically update explicit ready/packed quantities |
| POST | `/api/v1/consignments/{consignment_id}/confirm-outbound` | Deduct the fully packed reservation exactly once |
| POST | `/api/v1/consignments/{consignment_id}/cancel` | Cancel pre-outbound work and release reservations |

The exact state path is `created → allocated → picking → ready → packing →
packed → outbound → completed`. Cancellation is allowed only before outbound.
`ready` and `packed` transitions require every canonical line quantity to be
complete; missing quantities are never defaulted. Mutations require an
idempotency key and expected versions where state or line concurrency matters.

## Batch foundation

Batch endpoints require the selected Flipkart, Amazon, Meesho, Myntra, or Snapdeal entitlement and the
existing `labels.process` permission. `labels.print` covers generation and downloads, while
`labels.reprint` separately authorizes source-linked reprints.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/batch-eligible-orders?marketplace={flipkart|amazon|meesho|myntra|snapdeal}` | List completed, non-duplicate normalized orders not already in a batch |
| GET, POST | `/api/v1/batches` | List batches or idempotently create a draft from one to 500 orders |
| GET | `/api/v1/batches/{batch_id}` | Read ordered source traceability and Product Master totals |
| POST | `/api/v1/batches/{batch_id}/ready` | Ready a fully resolved draft |
| POST | `/api/v1/batches/{batch_id}/cancel` | Cancel a draft |

Company identity is never accepted from the request. Batch membership preserves
the selected sequence. An order can belong to only one operational batch in the
Batch A model. Replaying the same idempotency key and exact request returns the
original batch; using that key for different input is a conflict.

## Print generation

Print generation requires the batch marketplace entitlement and `labels.print`.

| Method | Path | Purpose |
| --- | --- | --- |
| POST | `/api/v1/batches/{batch_id}/print-jobs` | Generate idempotent label and optional invoice artifacts for a ready batch |
| GET | `/api/v1/batches/{batch_id}/print-jobs` | List print and reprint history for a batch |
| GET | `/api/v1/print-jobs/{print_job_id}` | Read tenant-scoped generation status and artifact metadata |
| GET | `/api/v1/print-artifacts/{artifact_id}` | Download a tenant-scoped generated PDF |
| POST | `/api/v1/print-jobs/{print_job_id}/reprints` | Regenerate source configuration with a required reason and idempotency key |

The generation request accepts `sort_labels`, `export_invoices`, and a required
idempotency key. Sorting uses normalized Product Master code, normalized raw SKU,
marketplace order ID, and original batch position as deterministic tie-breakers.
When sorting is disabled, original batch position is preserved. Generation is
bounded and synchronous in Batch B; persisted status remains visible as `ready`
or `failed`.

Flipkart batches retain `flipkart-a4-v1` generation. Amazon batches use
`amazon-a4-enriched-v1`: the adapter validates A4 geometry, preserves the full
shipping-label page with uniform scaling, and reserves a non-overlapping banner
for `SKU: <raw seller SKU> | QTY: <explicit quantity>`. Optional Amazon invoice
artifacts use the associated invoice pages. Missing traceability or enrichment
values fails generation rather than producing a guessed label.

Meesho batches use generic `source-page-v1` generation. It preserves each full
shipping-label source page in deterministic batch/sort order and produces one
traceable labels artifact. Invoice export is rejected because no representative
Meesho evidence establishes a deterministic invoice association. No cropping or
overlay geometry is guessed.

Reprints create a separate print job with `source_print_job_id` and
`reprint_reason`; they do not mutate the source job or any inventory domain.

## Worker assignments

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/worker-assignment-rules?marketplace={flipkart|amazon|meesho|myntra|snapdeal}` | List exact-product rules and the marketplace fallback worker (`labels.process`) |
| PUT | `/api/v1/worker-assignment-rules` | Atomically replace rules (`employees.manage`) |

Each configuration contains exactly one fallback. Exact Product Master rules
override it. Readying a batch snapshots assignments and worker totals so later
configuration changes do not rewrite historical workload.
# API

All endpoints use the authenticated server-side company context. Errors use the existing `{ "error": { "code", "message" } }` envelope.

## Dashboard and reporting

`GET /api/v1/reports/dashboard` requires `reports.view`. It accepts required
RFC3339 `from` and `to` instants using inclusive-start/exclusive-end semantics,
plus optional `marketplace`, `product_id`, `limit`, and `offset` filters. Ranges
are limited to 366 days. Explicit offsets preserve Today/Yesterday boundaries
in the operator's timezone while PostgreSQL compares authoritative
`timestamptz` values.

Marketplace, processing, batch, and print metrics derive from their owning
records. The dashboard marketplace selector supports `flipkart`, `amazon`, and
`meesho`. Inventory snapshots and ledger movement are returned only when the
principal also has the `inventory` entitlement and `inventory.view`; otherwise
`inventory_access` is false and restricted fields are omitted.

Return/cancellation metrics are returned only when the principal has the
`returns` entitlement and `returns.view`; otherwise `returns_access` is false
and the return summary is omitted. The summary derives cancellation occurrence,
physical receipt, inspected damage, gross restock, and closure metrics from
their authoritative records. `cohort_return_rate_percent` is the percentage of
resolved source orders created in the selected range that have any physically
received return; it is not a profitability or refund-rate metric.

With a marketplace filter, ecommerce stock-out and its contribution to product
net movement are limited through each ledger transaction's tenant-scoped batch
reference. Stock-in, manual adjustments, general corrections, and current
balances stay company-wide because they have no marketplace ownership.

Return restock and its linked compensating corrections follow the return's
normalized source-order marketplace. They appear separately as
`return_restock` movement so displayed categories reconcile with net movement.

## Inventory ledger

Inventory endpoints require the `inventory` entitlement. Commands accept a
canonical tenant Product Master ID, reason, and idempotency key; they never
accept a company ID or raw marketplace SKU.

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| GET | `/api/v1/inventory` | `inventory.view` | List product on-hand, reserved, and available balances |
| GET | `/api/v1/inventory/transactions` | `inventory.view` | List ledger entries filtered by `product_id` and/or `type` |
| POST | `/api/v1/inventory/stock-in` | `inventory.stock_in` | Record positive stock-in |
| POST | `/api/v1/inventory/adjustments` | `inventory.adjust` | Record a nonzero reasoned manual delta |
| POST | `/api/v1/inventory/corrections` | `inventory.adjust` | Record a compensating correction |
| POST | `/api/v1/inventory/batches/{batch_id}/confirm-outbound` | `inventory.dispatch` | Atomically deduct a ready marketplace batch's Product Master totals once; Myntra imports cannot reach ready without explicit quantity evidence |
| GET, POST | `/api/v1/inventory/reservations` | `inventory.view` / `inventory.adjust` | List reservations or reserve available stock for a source |
| POST | `/api/v1/inventory/reservations/{reservation_id}/release` | `inventory.adjust` | Idempotently release an active reservation with a reason |

Balance locking, ledger insertion, shared audit recording, and balance update
occur in one PostgreSQL transaction. Negative on-hand and reductions below
reserved stock are rejected. Ledger update/delete endpoints do not exist.

The explicit ecommerce trigger is outbound confirmation of a `ready` batch.
Upload, parsing, readiness, printing, and reprinting are inventory-neutral. A
company/batch unique outbound event and company/source/product ledger index make
duplicate deductions impossible. All product balances are locked in canonical
product-ID order; any shortage rolls back the event and every product movement.

Reservations are source-linked and unique per company/source/product. Creating
one increases `reserved` without changing `on_hand`; releasing it restores
availability. Reservation consumption is deferred to the future owning workflow.

## Returns and cancellations — Phase 8

Returns endpoints require the `returns` module entitlement. Read operations
require `returns.view`; intake, receipt, and inspection require
`returns.manage`. Restock and restock correction require `returns.restock` plus
the `inventory` entitlement.
Company identity always comes from the authenticated session.

| Method | Path | Purpose |
| --- | --- | --- |
| GET, POST | `/api/v1/cancellations` | List or idempotently record a normalized order cancellation |
| GET | `/api/v1/cancellations/{cancellation_id}` | Read cancellation detail and its outbound snapshot |
| POST | `/api/v1/cancellations/{cancellation_id}/close` | Idempotently close a cancellation without changing inventory |
| GET, POST | `/api/v1/returns` | List or idempotently create expected physical returns |
| GET | `/api/v1/returns/{return_id}` | Read return items and append-only lifecycle history |
| POST | `/api/v1/returns/{return_id}/receive` | Idempotently record explicit received quantities |
| POST | `/api/v1/returns/{return_id}/inspect` | Assign an explicit disposition to every received line |
| POST | `/api/v1/returns/{return_id}/restock` | Atomically restock only inspected/restockable quantities |
| POST | `/api/v1/returns/{return_id}/restock-corrections` | Record a bounded compensating correction |
| POST | `/api/v1/returns/{return_id}/close` | Idempotently close a disposition-complete return |

Cancellation creation records whether a central ready-batch outbound event
already exists. Neither pre-outbound nor post-outbound cancellation changes
stock. Return intake accepts only resolved normalized order items and explicit
quantities within the original ordered quantity. Marking a return received does
not restock it. Inspection is inventory-neutral; only the explicit restock
endpoint creates centralized `return_restock` ledger entries. Damaged, rejected,
wrong-product, and missing lines remain inventory-neutral. Corrections append
negative `correction` entries and never rewrite the original ledger.

Pre-outbound cancellations are excluded from batch eligibility and creation,
block draft readiness, and block outbound confirmation. A cancellation recorded
after an already committed outbound does not reverse stock by itself.

Closure requires `returns.manage`, records actor/time and append-only history,
and is inventory-neutral. A return can close only after restock/restock
correction or a terminal non-restockable disposition; expected, received, and
unrestocked inspected cases cannot be closed.

## Flipkart processing

- `POST /api/v1/flipkart/jobs` — multipart upload using field `file`; requires the `flipkart` entitlement and `labels.upload` plus `labels.process`. Returns HTTP 202 with `{job, duplicate_source}`.
- `GET /api/v1/flipkart/jobs/{job_id}` — returns the tenant-scoped job, normalized orders/items, and page-level warnings/errors; requires the entitlement and `labels.process`.
- `POST /api/v1/flipkart/jobs/{job_id}` — clears derived results and safely queues the source again, allowing newly trained SKUs to resolve.

Uploads are limited to 20 MiB and must begin with a PDF signature. Processing is asynchronous and durably queued in PostgreSQL; clients poll the job endpoint while its state is `queued` or `processing`. Error records expose `source_page`, `severity`, `code`, and `message`.

## Amazon processing — Phase 7 Batch C

- `POST /api/v1/amazon/jobs` — multipart PDF upload using `file`; requires the
  `amazon` entitlement plus `labels.upload` and `labels.process`.
- `GET /api/v1/amazon/jobs/{job_id}` — reads the tenant-scoped job, normalized
  normalized orders/items, contributing `documents`, and page-level review
  errors. Each document exposes `source_page`, `role`, and
  `extraction_method` (`text` or `ocr`).
- `POST /api/v1/amazon/jobs/{job_id}` — safely retries a completed/review/failed
  Amazon source after Product Master training or parser updates.

Amazon uses the same 20 MiB validation, server-generated tenant storage keys,
company/marketplace hash deduplication, PostgreSQL job leases, response shapes,
and error envelope as Flipkart. Exact active `amazon` SKU mappings resolve to
canonical Product Master products. Missing tracking, seller SKU, quantity, or
mapping values remain explicit review data and are never guessed.

Amazon image-only pages use bounded OCR. A shipping label and invoice normally
become one canonical order when their extracted Amazon order IDs match exactly.
The validated adjacency fallback requires a mutually unique opposite-role pair,
one stable order ID, unique AWB, invoice SKU, and explicit quantity. Duplicate
roles, conflicting identifiers, competing neighbors, and incomplete values
remain review records.

Resolved Amazon orders participate in the shared batch, print, outbound
confirmation, inventory ledger, and reporting APIs. No Amazon-specific stock
mutation or PDF-download endpoint exists; print and reprint operations remain
inventory-neutral.

## Meesho processing — Phase 10

- `POST /api/v1/meesho/jobs` — multipart PDF upload using `file`; requires the
  `meesho` entitlement plus `labels.upload` and `labels.process`.
- `GET /api/v1/meesho/jobs/{job_id}` — reads the tenant-scoped job, normalized
  order/item records, source-page documents, and review errors.
- `POST /api/v1/meesho/jobs/{job_id}` — safely retries a terminal Meesho source
  after Product Master training or parser updates.

Meesho reuses the generic 20 MiB PDF validation, tenant storage keys,
company/marketplace source deduplication, PostgreSQL leases, response shapes,
error envelope, and exact Product Master mapping behavior. The adapter accepts
only explicitly labeled order/sub-order ID, AWB/tracking ID, seller/supplier
SKU, and positive quantity values. Missing, zero, conflicting, or ambiguous
values remain review-required; quantity is never defaulted to one.

Resolved Meesho orders participate in the shared batch, worker-assignment,
source-page print artifact, reprint, outbound confirmation, reporting, and
returns workflows. Printing/reprinting are inventory-neutral; only explicit
ready-batch outbound confirmation creates idempotent `ecommerce_out` entries.

## Myntra CSV processing — Phase 11 Batch A

- `POST /api/v1/myntra/jobs` accepts a UTF-8 packed-orders CSV plus a required
  multipart `idempotency_key`; it requires the `myntra` entitlement and existing
  `labels.upload` and `labels.process` permissions.
- `GET /api/v1/myntra/jobs/{job_id}` returns tenant-scoped normalized rows,
  Product Master resolution, review reasons, and preserved Myntra evidence.
- `POST /api/v1/myntra/jobs/{job_id}` retries a terminal import against current
  Product Master mappings.

The exact active `myntra` mapping key is `Seller_sku_code`. `Order id` maps to
the shared marketplace order ID and `Tracking_id` maps to AWB. Myntra SKU code,
Store Packet ID, Order_release_id, marketplace status, Packed On, and Created On
remain extraction metadata. The observed CSV has no authoritative quantity;
all rows retain missing quantity and remain blocked from readiness, printing,
outbound confirmation, and quantity-dependent return intake. No Myntra PDF
contract or print generator exists in Batch A.

## Snapdeal processing — Phase 12

- `POST /api/v1/snapdeal/jobs` uploads a Snapdeal PDF with the `snapdeal`
  entitlement and existing label upload/process permissions.
- `GET /api/v1/snapdeal/jobs/{job_id}` exposes normalized order/item data,
  exact-suborder document traceability, and review warnings.
- `POST /api/v1/snapdeal/jobs/{job_id}` retries a terminal source.

The adapter classifies shipping and invoice pages from observed text signals,
associates only by exact `SUBORDER`, uses invoice `SKU CODE` for Product Master,
and accepts only one explicit positive quantity across associated evidence.
Snapdeal batches use the shared print API; output preserves the measured source
shipping page, enriches a verified blank band with SKU/QTY, and optionally
exports the exactly associated invoice.
