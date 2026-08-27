# AI-Assisted Engineering Workflow

CommerceOps uses AI tools as constrained engineering contributors. Product
authority, approved architecture, domain rules, and phase gates remain the
source of truth.

## Review and approval flow

```text
User / product owner
→ architecture and scope review
→ Codex implementation
→ automated verification
→ external architecture review
→ optional Gemini or Claude second review for risky work
→ approval
→ next phase
```

External review is a gate, not a formality. A successful build does not approve
architecture, business behavior, or a phase transition. Secondary AI review is
useful for security-sensitive, tenant-sensitive, schema-heavy, or otherwise
risky changes, but it does not replace owner approval.

## Implementation lifecycle

Every implementation follows this lifecycle:

```text
READ
→ PLAN
→ IMPLEMENT SMALL BATCH
→ TEST
→ SELF-REVIEW
→ COMMIT
→ REPORT
→ STOP
```

- **READ:** Read `AGENTS.md`, the source-of-truth documents, the active phase
  specification, applicable module documentation, and the existing code.
- **PLAN:** State affected modules/files, schema and API impact, architecture
  risks, required tests, and active-phase compliance before broad work begins.
- **IMPLEMENT SMALL BATCH:** Make the smallest coherent change. Phases must be
  delivered incrementally, never as uncontrolled large rewrites.
- **TEST:** Run checks appropriate to the change. Clearly distinguish passed,
  failed, skipped, and unexecuted tests.
- **SELF-REVIEW:** Inspect the diff for tenant isolation, module ownership,
  domain invariants, phase scope, secrets, unrelated edits, and documentation
  drift.
- **COMMIT:** Create a focused commit only when requested or authorized. Do not
  mix unrelated work into it.
- **REPORT:** Use the completion-report format required by `AGENTS.md`.
- **STOP:** Do not automatically begin another task or phase.

## Phase discipline

Only the active phase may be implemented. Future phase documents are design
references until the current phase passes its review gate and the owner
explicitly authorizes the next phase. If a requested implementation requires a
foundational architecture change, stop and request an approved ADR rather than
silently expanding scope.
