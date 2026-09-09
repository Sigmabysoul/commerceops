# AI-Assisted Engineering Workflow

CommerceOps uses AI tools as constrained contributors.

Authoritative sources:
- product-owner decisions
- MASTER_SPEC.md
- DOMAIN_RULES.md
- MODULES.md
- CURRENT_STATE.md
- approved Phase documents / ADRs

## Responsibilities

### Codex
Primary implementation agent:
- code
- migrations
- tests
- refactors
- frontend/backend changes
- CI fixes

### ChatGPT
Primary architecture/review agent:
- architecture
- Phase planning
- repo review
- approval gates
- integration design
- debugging strategy
- Codex prompts

### Gemini / Google AI Studio
Use when it has an advantage:
- many screenshots/PDFs/images
- marketplace label/invoice comparison
- visual anchor/layout analysis
- UI screenshot critique
- long multimodal context
- structured extraction experiments
- sanitized fixture ideation
- independent second review

Gemini output is advisory unless an approved Phase explicitly introduces runtime AI.

Deterministic parsing remains preferred for stable business identifiers/layouts.

## Lifecycle

READ
→ PLAN
→ IMPLEMENT COHESIVE BATCH
→ TEST
→ SELF-REVIEW
→ COMMIT
→ REPORT
→ STOP

Never automatically start the next Phase.
