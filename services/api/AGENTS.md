This is the main Go backend.

Read the root AGENTS.md and docs/CURRENT_STATE.md first.
Business packages belong under internal/domain; technical mechanisms under internal/platform.
Application composition stays in internal/app. During Phase 15, move only approved packages.
HTTP handlers remain thin. Database access must respect server-established company scope.
Keep PDF extractor and generator separate; keep Automation's scheduler in its domain.
