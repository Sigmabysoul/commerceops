# Phase 15 — Multi-ID Business Model / Marketplace Seller Accounts

**Status:** PLANNED / NOT IMPLEMENTED  
**Depends on:** approved Phase 14 baseline

## Goal

Allow one CommerceOps tenant to operate multiple seller IDs/accounts on the same
marketplace without duplicating Product Master, Inventory or company tenancy.

## Real operating configuration

Current known seller/account structure:

- Flipkart
  - Brothers — most products
  - Veritas — currently a small subset; known example combo = tripod + ring light
- Meesho
  - Aver Retail
  - Supr Retail
  - broadly overlapping product range
- Myntra
  - Brothers
  - Veritas (currently dormant operationally)
- Snapdeal
  - Brothers
- Amazon
  - Brothers
- JioMart
  - Brothers

Separate AK/Brothers computers may be accessed through Chrome Remote Desktop.

Machine identity is NOT seller-account identity.

## Target model

Company
→ Business / Trading Identity
→ Marketplace Seller Account

Separately:

Company
→ Workstation / Printer Agent

Suggested generic entities:

- `business_identities`
- `marketplace_accounts`

No business name is hardcoded into source logic.

## Marketplace Account fields

Suggested:
- id
- company_id
- marketplace_key
- business_identity_id
- internal_key
- display_name
- external_seller_id nullable
- integration_mode
- status: active / dormant / inactive
- metadata
- created_at
- updated_at

Integration mode examples:
- manual_upload
- api
- csv
- file_watch
- print_capture
- partner_connector

Secrets must not be stored in generic metadata.

## Product Master change

Current conceptual lookup:
`company + marketplace + sku`

Target:
`company + marketplace_account + sku`

Required:
- same raw SKU may exist in two accounts
- two account SKUs may map to one canonical Product Master item
- account-specific mapping is allowed where real evidence differs
- no duplicate canonical products solely because seller IDs differ

## Provenance

Every marketplace source/order/item must preserve authoritative
`marketplace_account_id`.

Do not infer seller account solely from:
- workstation
- SKU
- employee
- browser title

Ambiguity must become explicit selection or review.

## Duplicate / Returns / Reporting

Review uniqueness and matching so external IDs are seller-account aware where needed.

Returns must never resolve an order from account A against account B.

Dashboard/reporting must support seller-account filters.

## Printer Agent / workstation routing

Optional relation:
Printer Agent ↔ allowed Marketplace Accounts

This is routing/authorization only.

The workstation does not decide authoritative seller ownership.

## UI

Add Marketplace Accounts administration.

Operational source flow should become:
Marketplace → Seller Account → Source

If one active account exists, UI may preselect it but still persist the ID explicitly.

## Migration

Likely new migration after 000022.

Existing historical rows must not be silently guessed into current seller identities.
Define an explicit migration/default-legacy strategy and document ambiguity.

## Tests

Must cover:
- two accounts same marketplace
- same raw SKU in two accounts
- same canonical product across accounts
- cross-account Product Master isolation
- same external order ID in two accounts
- account-aware dedupe
- Returns isolation
- reporting filters
- dormant account behavior
- tenant isolation
- workstation routing
- migration up/down

## Delivery

Batch A:
schema/domain/backend/migration/tests

STOP for external review.

Batch B after approval:
frontend/filtering/agent routing/docs/full verification.
