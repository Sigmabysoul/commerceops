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
