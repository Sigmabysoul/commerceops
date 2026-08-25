COMMERCEOPS — PHASE 2: PRODUCT MASTER

ROLE

Implement the central CommerceOps Product Master and SKU mapping/training system.

Phase 0 and Phase 1 must already be approved.

Do not implement Flipkart label processing yet.


==================================================
MANDATORY READING
==================================================

Read:

- AGENTS.md
- docs/MASTER_SPEC.md
- docs/ARCHITECTURE.md
- docs/DOMAIN_RULES.md
- docs/CURRENT_STATE.md
- docs/ROADMAP.md
- Phase 2 documentation
- Product-related ADRs if any

Inspect existing Core authorization and tenant patterns.


==================================================
PHASE GOAL
==================================================

Create the canonical product system that every future module will use.

The core principle is:

Marketplace SKU != Product Identity

Different marketplace SKUs must resolve to a single internal CommerceOps product/variant.


==================================================
PRODUCT MODEL
==================================================

Design a clean Product Master.

Avoid over-modeling.

Support the actual operational concepts needed now while allowing future extension.

Conceptual fields/entities may include:

Product
- id
- company_id
- name
- internal_code
- active
- created_at
- updated_at

Optional structured attributes may include:

- brand
- variant
- size
- pack type
- unit count

Only introduce separate tables/entities where there is clear value.

Do not build a generic ERP product engine.


==================================================
EXAMPLE REAL PRODUCTS
==================================================

The current company configuration includes products such as:

Garbage Bags:
- Averx 2 Bag
- Averx 3 Bag
- Star 2 Bag
- Star 3 Bag
- Plain 2 Bag
- Plain 3 Bag

Garbage Rolls:
- 17x19
- 19x21

Butter Paper

Aluminium Containers:
- 25 pc set
- 50 pc set
- sometimes 100 pc set

These examples are data/configuration.

They must NOT become hardcoded source-code rules.


==================================================
INTERNAL PRODUCT CODE
==================================================

Allow a company to assign a stable internal product code.

Example:

GB-AVX-3B

Internal codes should be unique within a company where appropriate.


==================================================
SKU ALIAS / MAPPING
==================================================

Implement marketplace SKU aliases.

Concept:

Marketplace SKU:
ABC-XYZ-123

Marketplace:
Flipkart

resolves to:

Internal Product:
GB-AVX-3B


A product can have multiple aliases.

Aliases belong to a company and marketplace context where required.


==================================================
MARKETPLACE IDENTIFIERS
==================================================

Create or reuse a normalized marketplace concept.

Initial marketplaces:

- flipkart
- amazon
- meesho
- myntra
- snapdeal

Marketplace-specific parsing logic does NOT belong in Phase 2.


==================================================
TRAINING WORKFLOW
==================================================

Implement the foundation for training unknown SKU strings.

Future workflow:

Unknown marketplace SKU appears
→ user selects "Train"
→ choose internal product
→ confirm marketplace
→ optionally configure interpretation metadata
→ save mapping
→ future occurrence resolves automatically

Phase 2 should support manually creating/editing these mappings even before automated label processing exists.


==================================================
MATCHING BEHAVIOR
==================================================

Keep matching deterministic.

Initial matching should prefer explicit known aliases.

Do not introduce machine learning.

Do not create fuzzy automatic mapping that could silently assign the wrong product.

Ambiguous/unknown identifiers should return an explicit unresolved state.


==================================================
QUANTITY/PACK INTERPRETATION
==================================================

If quantity metadata is required for current known product rules, design it carefully.

Do not bury pack calculations inside arbitrary SKU parsing code.

The product model should support expressing useful product packaging/quantity information where it belongs.

Avoid implementing marketplace-specific label quantity extraction in this phase.


==================================================
WORKER ASSIGNMENT PREPARATION
==================================================

Product records must be capable of participating in future employee assignment rules.

However, the current responsibility of Kartik or other workers must NOT be hardcoded on Product.

Assignment rules will be configuration and should remain logically separable.

If Phase 2 needs a minimal assignment-rule foundation to properly model this, keep it generic and documented.

Do not implement full workload processing yet.


==================================================
PRODUCT STATE
==================================================

Support product lifecycle such as:

active
inactive

Avoid destructive product deletion when historical references may exist.

Mappings should also be deactivatable/editable rather than forcing deletion.


==================================================
AUDIT
==================================================

Audit important changes:

- product created
- product edited
- product deactivated
- SKU mapping added
- SKU mapping changed
- SKU mapping removed/deactivated
- training mapping created

Record actor/company/context using the Phase 1 audit system.


==================================================
AUTHORIZATION
==================================================

Use Phase 1 permissions.

Potential operations:

products.view
products.manage

Add more granular permissions only if justified.

Backend enforcement is mandatory.


==================================================
DATABASE
==================================================

Create only the schema required for Product Master.

Potential entities include:

products
product_variants if justified
marketplaces
sku_aliases / sku_mappings

Design for tenant isolation.

Add uniqueness/indexes suitable for:

company
marketplace
SKU lookup
internal code

Do not create order, label, batch or inventory tables yet.


==================================================
API
==================================================

Implement APIs for:

- list/search products
- create product
- update product
- activate/deactivate product
- view product
- create SKU mapping
- update/deactivate SKU mapping
- resolve an identifier to a product
- list mappings
- identify unknown/unresolved mapping

Use existing API conventions.


==================================================
FRONTEND
==================================================

Create usable Product Master screens.

Suggested capabilities:

Products
- search
- filters
- create
- edit
- active/inactive

Product detail
- core info
- aliases/mappings

SKU Training
- marketplace
- raw SKU
- selected internal product
- save mapping

Do not over-polish.

Do not implement ecommerce label upload yet.


==================================================
TESTS
==================================================

Test at minimum:

1. tenant cannot see another tenant's products
2. internal product code uniqueness rules
3. known alias resolves correctly
4. unknown alias returns unresolved state
5. same SKU may behave correctly across distinct companies
6. marketplace context is respected
7. inactive mapping behavior
8. permission enforcement
9. mapping changes produce audit records
10. ambiguous mappings cannot silently resolve incorrectly


==================================================
NON-GOALS
==================================================

Do not implement:

- PDF parsing
- OCR
- Flipkart uploads
- Amazon
- batches
- printing
- stock
- returns
- consignment
- machine learning
- fuzzy AI product guessing


==================================================
DEFINITION OF DONE
==================================================

Phase 2 is implementation-complete if:

- Product Master exists
- products are tenant-isolated
- internal codes work
- SKU aliases work
- mappings can be trained manually
- deterministic resolution works
- unknown identifiers remain explicit
- permissions work
- audit works
- frontend management works
- migrations pass
- tests pass
- documentation is updated


==================================================
COMPLETION REPORT
==================================================

Report:

1. schema introduced
2. Product model decisions
3. SKU matching design
4. training workflow
5. API endpoints
6. frontend screens
7. tests
8. dependencies
9. known limitations
10. unresolved design questions
11. documentation updates

Finish with:

PHASE 2 IMPLEMENTATION COMPLETE — READY FOR EXTERNAL REVIEW

or

PHASE 2 NOT COMPLETE

STOP.
Do not start Flipkart.