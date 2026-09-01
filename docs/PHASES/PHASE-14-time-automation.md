Implement CommerceOps Phase 14 — Printing Automation.

Do not begin until Phase 13 Printing Platform/Printer Agent is approved and
stable.

Goal:
Allow authorized users to automate print jobs using schedules or approved
CommerceOps domain events.

Automation must CREATE normal Print Jobs.
It must NOT bypass the Print Job domain or talk directly to printers.

--------------------------------------------------
A. Scheduled printing
--------------------------------------------------

Allow authorized users to create schedules such as:

Every weekday at 09:00
Print:
R1S Box Sticker
Copies: 20
Printer: Packing Sticker Printer

Support:
- timezone-aware schedules
- enabled/disabled
- start/end date where useful
- daily
- weekdays
- selected weekdays
- selected clock times
- next-run preview

Use company timezone explicitly.

Prevent duplicate execution after scheduler restart.

--------------------------------------------------
B. Event-driven printing
--------------------------------------------------

Support approved triggers such as:

When ecommerce batch becomes ready
→ create print job

When consignment reaches packing/packed
→ optionally print configured documents/stickers

When a designated operational event occurs
→ print configured reusable asset

Triggers must reference authoritative domain events.

Do NOT infer automation from UI state.

--------------------------------------------------
C. Automation rules
--------------------------------------------------

Rule contains:
- name
- enabled
- trigger type
- trigger configuration
- print asset/artifact strategy
- printer
- copies
- company
- creator
- timestamps
- version
- audit history

--------------------------------------------------
D. Safety
--------------------------------------------------

Require explicit permission to manage automations.

Add controls:
- maximum copies per execution
- optional daily print limit
- failure backoff
- disable rule after repeated failures if appropriate
- manual pause
- test-run function

Retries must not duplicate completed physical print jobs.

--------------------------------------------------
E. User experience
--------------------------------------------------

Provide simple screens:

Automation Rules
Scheduled Prints
Upcoming
Recent Runs
Failures

Example:

R1S Morning Stickers
Weekdays 09:00
20 copies
Sticker Printer
[ON]

--------------------------------------------------
F. Reporting
--------------------------------------------------

Expose:
- automatic vs manual jobs
- success/failure
- printer reliability
- copies
- automation rule history

Do not create independent counters if data can be derived from print jobs.

--------------------------------------------------
G. Tests
--------------------------------------------------

Cover:
- timezone
- DST/time boundary behavior where applicable
- scheduler restart
- exact-once logical execution
- duplicate prevention
- disabled rules
- authorization
- tenant isolation
- event trigger
- consignment trigger
- ecommerce trigger
- printer offline
- retry
- audit
- no inventory side effects

Update docs/CURRENT_STATE.

STOP.
Do not invent new automation domains without authorization.