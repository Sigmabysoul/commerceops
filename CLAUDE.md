# CommerceOps

Read `AGENTS.md` first and follow it as the primary AI engineering policy.

Before reviewing architecture or business logic, read:

- docs/MASTER_SPEC.md
- docs/ARCHITECTURE.md
- docs/DOMAIN_RULES.md
- docs/CURRENT_STATE.md
- the active phase specification under docs/PHASES/

For review tasks, do not modify code unless explicitly asked.

Report P0, P1, and P2 findings with file/line evidence. Verify tenant isolation,
domain ownership, and active-phase boundaries even when no finding is present.

Do not approve work outside the current phase or future-phase work before the
current review gate passes.

## Agent skills

### Issue tracker

Track CommerceOps work in the repository's GitHub Issues. See
`docs/agents/issue-tracker.md`.

### Triage labels

Use the five canonical triage labels without repository-specific aliases. See
`docs/agents/triage-labels.md`.

### Domain docs

Use a single CommerceOps context with root domain vocabulary and system-wide
decisions under `docs/ADR/`. See `docs/agents/domain.md`.
