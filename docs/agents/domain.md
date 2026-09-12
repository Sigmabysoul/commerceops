# Domain documents

CommerceOps uses one shared domain context because the API, web application,
database, and optional workers implement one modular-monolith business model.
Folder boundaries do not create separate domain vocabularies.

## Read before domain work

- Read root `CONTEXT.md` when it exists.
- Read relevant accepted decisions under `docs/ADR/`.
- Continue with `docs/MASTER_SPEC.md`, `docs/ARCHITECTURE.md`,
  `docs/DOMAIN_RULES.md`, `docs/CURRENT_STATE.md`, and the active phase document
  as required by `AGENTS.md`.

`CONTEXT.md` is created lazily when domain-modeling work establishes a glossary
that is clearer than the existing specifications. Its absence is not a setup
failure and does not justify inventing terminology.

## Layout

```text
/
├── CONTEXT.md                 # shared glossary, when needed
├── docs/ADR/                  # accepted system-wide decisions
├── services/api/              # Go modular monolith
├── apps/web/                  # Next.js application
└── workers/                   # optional specialized workers
```

Use terms exactly as the current domain documentation defines them. If a needed
concept is missing, reconsider whether it is new language or record the gap for
domain-modeling work.

Surface any conflict with an accepted ADR explicitly. Follow the decision
priority in `AGENTS.md`; do not silently replace an existing architectural or
business invariant.
