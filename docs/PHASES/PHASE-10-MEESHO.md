Implement CommerceOps Phase 10 — Meesho Marketplace Processing.

DO NOT begin until Phase 9 is formally approved.

Read first:
- AGENTS.md
- docs/AI_WORKFLOW.md
- docs/CURRENT_STATE.md
- docs/ARCHITECTURE.md
- docs/DOMAIN_RULES.md
- docs/MODULES.md
- Phase 3 Flipkart implementation/spec
- Phase 7 Amazon implementation/spec
- Product Master
- Batch/Printing
- Inventory
- Reporting
- Returns

Goal:
Add Meesho as another isolated marketplace adapter while reusing the existing
CommerceOps marketplace pipeline.

ARCHITECTURE RULE:
Do NOT create Meesho-specific copies of:
- upload infrastructure
- source files
- object storage
- processing jobs
- worker leases
- Product Master
- batch system
- Inventory
- reporting
- returns

Meesho-specific behavior belongs only inside an isolated marketplace adapter.

Required behavior:
1. Secure Meesho PDF upload using existing marketplace upload infrastructure.
2. Extract the real identifiers present in production Meesho labels:
   - order/sub-order identifier
   - AWB/tracking ID
   - seller SKU
   - explicit quantity
   - other stable identifiers required by real samples
3. Never silently default missing quantity to 1.
4. Unknown SKU → unresolved/manual review.
5. Resolve raw Meesho SKU through Product Master marketplace mappings.
6. Detect repeated source files and repeated business identifiers.
7. Preserve source file/page/parser-version traceability.
8. Support deterministic document association if Meesho splits complementary
   information across pages.
9. Add Meesho orders to the existing batch system.
10. Add specialized print adapter only if Meesho output genuinely needs label
    manipulation.
11. Parsing/printing/reprinting remain inventory-neutral.
12. Existing outbound confirmation creates ecommerce_out.
13. Existing dashboard/reporting must include Meesho naturally.
14. Existing Returns domain must accept normalized Meesho orders.
15. Require Meesho module entitlement and normal label permissions.

Production PDFs:
- keep private
- do not commit customer data
- create sanitized fixtures preserving structural patterns

Tests:
- parser correctness
- explicit quantity
- unknown SKU
- Product Master resolution
- duplicate source
- duplicate order/AWB
- tenant isolation
- entitlement/permissions
- retries
- batch compatibility
- printing if required
- inventory neutrality before outbound
- outbound idempotency
- reporting marketplace isolation
- Returns compatibility
- migration up/down if schema changes
- full PostgreSQL verification

Before editing:
Provide:
1. expected files
2. schema/API changes
3. generic reuse vs Meesho-specific responsibilities
4. risks
5. test strategy
6. confirmation Phase 10 is active

Implement in medium cohesive batches.

STOP after Phase 10.
Do not begin Myntra / Phase 11.