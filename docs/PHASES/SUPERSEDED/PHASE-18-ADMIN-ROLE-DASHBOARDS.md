# Superseded planning reference

Preserved from the pre-consolidation working tree on 2026-09-07. Phase numbers and authorization statements below are superseded by `../../ROADMAP.md`. This file does not authorize implementation.

# Phase 18 — Admin / Role Dashboards

**Status:** PLANNED / NOT IMPLEMENTED

## Goal

Provide role-appropriate dashboards without replacing granular backend permissions.

Initial internal deployment:
- 3 Owner accounts
- 1 Developer/Test Administrator
- HR
- Team Leaders
- Workers

The counts are deployment configuration, not schema limits.

## Owner dashboard

Full business/operations cockpit:
- marketplace + seller-account activity
- daily order/label volume
- Batch
- Inventory
- Returns
- Consignment
- Printing
- Automation
- Trace Box/QC/rework
- defects and bottlenecks
- employee/workflow throughput
- alerts/failures
- administration links

Owner access remains permission checked and audited.

## Developer/Test Admin

Dev/test capabilities:
- all normal module views
- health/build/schema status
- Printer Agent diagnostics
- object-storage diagnostics
- test fixtures/utilities if explicitly implemented
- safe simulated Print Jobs
- test-only reset/reseed tools where approved

Production:
- no tenant bypass
- no Inventory bypass
- no arbitrary SQL/shell
- dangerous diagnostics restricted/disabled
- critical actions audited

## HR

Focus:
- employees
- departments/memberships
- permitted workforce reports
- future attendance/leave if added

No automatic access to:
- Inventory adjustment
- marketplace secrets
- printer-agent credentials
- unrelated owner/system settings

## Team Leader

Focus:
- department queue
- Trace Boxes
- handovers
- pending/ready/packed
- blocked work
- assignments
- permitted correction/escalation
- Consignment progress

## Worker

Mobile/task-focused:
- scan QR
- receive
- QC
- rejection reasons
- rework/sticker
- handover
- packing
- final check when permitted
- blockers/instructions

## Authorization principle

Dashboards compose existing domain APIs.
No dashboard creates a second business truth or bypasses backend permissions.
