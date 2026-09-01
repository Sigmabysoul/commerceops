Implement CommerceOps Phase 13 — Printing Platform and Printer Agent.

Do not begin until Phase 12 is approved.

This phase turns existing PDF artifact generation into a real warehouse printing
platform.

IMPORTANT:
Printing must remain completely inventory-neutral.

Architecture:

CommerceOps Server
    ↓
Print Jobs
    ↓
Printer Agent
    ↓
OS printing system / physical printer

The browser/mobile frontend must NOT need direct printer access.

--------------------------------------------------
A. Printer Registry
--------------------------------------------------

Create tenant-scoped registered printers.

Printer information:
- id
- friendly name
- agent/device ownership
- operating-system printer identifier
- capability metadata
- online/offline status
- last seen
- optional location
- enabled/disabled

Users must select printers by friendly CommerceOps name, never arbitrary system
commands.

--------------------------------------------------
B. Printer Agent
--------------------------------------------------

Create a small local agent that runs on a warehouse/office computer connected
to printers.

The agent should:
- authenticate securely with CommerceOps
- register itself
- report available printers
- heartbeat
- poll/receive authorized print jobs
- securely download print artifacts
- print requested copies
- report queued/printing/completed/failed state
- retry safely
- never print the same job twice because of reconnect/retry
- never contain business-domain logic
- never mutate inventory

Support Linux/CUPS first if appropriate for current environment.
Keep platform abstraction so Windows printing can be added later.

Do not expose arbitrary shell-command execution.

--------------------------------------------------
C. Unified Print Job Domain
--------------------------------------------------

Every physical printing action should become the same canonical print job.

Possible origin types:
- ecommerce_batch
- ecommerce_reprint
- consignment
- quick_print
- scheduled
- automation

Store:
- company
- requested by
- printer
- artifact
- copies
- origin type/reference
- status
- timestamps
- failure information
- idempotency key
- audit history

--------------------------------------------------
D. Print Library
--------------------------------------------------

Create tenant-scoped reusable printable assets.

Users with permission can upload a PDF and define:
- custom name
- category
- description
- default printer
- default copies
- active/inactive
- favorite
- optional Product Master association

Examples:
- R1S Outer Box Sticker
- Butter Paper Box Sticker
- Aluminium 25pc Sticker
- Fragile
- Handle With Care
- Product carton label

Store original assets in existing object storage.

Never trust client-provided storage keys.

--------------------------------------------------
E. Mobile Quick Print
--------------------------------------------------

Create a mobile-first/PWA interface.

Workers see large simple buttons/cards for authorized Print Library assets.

Example:

[ R1S ]
[ R16S ]
[ Butter Paper ]
[ AL 25 ]
[ Fragile ]

Tap asset
→ quantity/copies dialog
→ choose/default printer
→ confirm
→ server creates print job
→ printer agent executes it.

The phone does NOT communicate directly with the printer.

Support:
- favorites
- categories
- recently printed
- search
- large readable controls
- copy count guardrails
- confirmation for large quantities

--------------------------------------------------
F. Existing printing integration
--------------------------------------------------

Existing Flipkart/Amazon/Meesho/Myntra/Snapdeal print artifacts should be able
to enter this same print queue.

Do not replace existing artifact generation.

The Printing Platform owns physical delivery of generated artifacts.

--------------------------------------------------
G. Security
--------------------------------------------------

Add dedicated permissions such as:
- printers.view
- printers.manage
- printing.print
- printing.reprint
- print_library.view
- print_library.manage

Tenant isolation mandatory.

Agent credentials must be scoped/revocable.

Never expose unrestricted local filesystem or shell execution through agent APIs.

--------------------------------------------------
H. Audit and history
--------------------------------------------------

Audit:
- printer registration
- asset upload/change/archive
- print job request
- cancel
- completion/failure
- reprint

Printing/reprinting must NEVER imply stock movement.

--------------------------------------------------
I. Tests
--------------------------------------------------

Cover:
- tenant isolation
- permissions
- agent authentication
- printer registration
- heartbeat/offline state
- print-job idempotency
- duplicate delivery/reconnect
- failed printer
- retry
- quick-print quantity
- reusable PDF assets
- invalid PDF
- cross-tenant asset access
- existing batch artifact printing
- reprint inventory neutrality
- audit
- concurrency
- migration up/down

Before coding, first produce:
1. domain/schema design
2. agent protocol
3. security threat model
4. Linux/CUPS implementation boundary
5. future Windows compatibility
6. mobile UX plan
7. migration/API plan
8. test plan

STOP after Phase 13.
Do not implement automatic schedules yet.