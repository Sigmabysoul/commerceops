# Phase 15 risk register

| Risk | Mitigation |
|---|---|
| Loss of seller-account WIP | Separate preservation ref with temporary index; compare original file/index hashes |
| Bundle selects WIP HEAD instead of Phase 14 | Record and explicitly checkout exact baseline commit |
| Import cycles / changed ownership | Preserve packages and symbols; mechanical imports only; no PDF merge |
| Relative fixture paths | Move colocated fixtures; verify cross-package references after each batch |
| Automation migrations silently missing | Update glob and round-trip paths; full PostgreSQL schema setup/tests |
| API/SQL/dependency drift | Compare exact baseline paths and checksums |
| Agent build omitted by CI | Build both cmd/server and cmd/printer-agent |
| Historical docs imply current state | Preserve historical records and mark superseded drafts |
| Secret or operational data in export | Exclude local data; review history paths/content; checksums are integrity, not confidentiality |
| Optional private evidence unavailable | Retain sanitized fixture tests; report skips, do not claim production evidence |
| Premature phase progression | CURRENT_STATE gate; require explicit Phase 16 authorization |
