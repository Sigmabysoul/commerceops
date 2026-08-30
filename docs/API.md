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

## Batch foundation

Batch endpoints require the Flipkart entitlement and existing `labels.process`
permission. `labels.print` covers generation and downloads, while
`labels.reprint` separately authorizes source-linked reprints.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/batch-eligible-orders?marketplace=flipkart` | List completed, non-duplicate normalized orders not already in a batch |
| GET, POST | `/api/v1/batches` | List batches or idempotently create a draft from one to 500 orders |
| GET | `/api/v1/batches/{batch_id}` | Read ordered source traceability and Product Master totals |
| POST | `/api/v1/batches/{batch_id}/ready` | Ready a fully resolved draft |
| POST | `/api/v1/batches/{batch_id}/cancel` | Cancel a draft |

Company identity is never accepted from the request. Batch membership preserves
the selected sequence. An order can belong to only one operational batch in the
Batch A model. Replaying the same idempotency key and exact request returns the
original batch; using that key for different input is a conflict.

## Print generation

Print generation requires the Flipkart entitlement and `labels.print`.

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

Reprints create a separate print job with `source_print_job_id` and
`reprint_reason`; they do not mutate the source job or any inventory domain.

## Worker assignments

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/worker-assignment-rules?marketplace=flipkart` | List exact-product rules and the marketplace fallback worker (`labels.process`) |
| PUT | `/api/v1/worker-assignment-rules` | Atomically replace rules (`employees.manage`) |

Each configuration contains exactly one fallback. Exact Product Master rules
override it. Readying a batch snapshots assignments and worker totals so later
configuration changes do not rewrite historical workload.
# API

All endpoints use the authenticated server-side company context. Errors use the existing `{ "error": { "code", "message" } }` envelope.

## Flipkart processing

- `POST /api/v1/flipkart/jobs` — multipart upload using field `file`; requires the `flipkart` entitlement and `labels.upload` plus `labels.process`. Returns HTTP 202 with `{job, duplicate_source}`.
- `GET /api/v1/flipkart/jobs/{job_id}` — returns the tenant-scoped job, normalized orders/items, and page-level warnings/errors; requires the entitlement and `labels.process`.
- `POST /api/v1/flipkart/jobs/{job_id}` — clears derived results and safely queues the source again, allowing newly trained SKUs to resolve.

Uploads are limited to 20 MiB and must begin with a PDF signature. Processing is asynchronous and durably queued in PostgreSQL; clients poll the job endpoint while its state is `queued` or `processing`. Error records expose `source_page`, `severity`, `code`, and `message`.
