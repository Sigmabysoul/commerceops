# API conventions

CommerceOps exposes REST endpoints under `/api/v1`. JSON responses use `application/json`.

Errors use a stable envelope and never include internal SQL errors, credentials, host details, or stack traces:

```json
{"error":{"code":"INTERNAL_ERROR","message":"Something went wrong"}}
```

## Health

`GET /api/v1/health` checks the required PostgreSQL dependency.

- HTTP 200: `{"status":"ok","database":"ok"}`
- HTTP 503: `{"status":"unavailable","database":"unavailable"}`

Other methods receive HTTP 405 with the standard error envelope. The endpoint is intentionally tenant-independent and exposes no sensitive dependency details.

## Core Platform

Authentication uses an opaque server-side session in an `HttpOnly`, `SameSite=Lax` cookie. Except for login and health, all endpoints require that session. Login's `company_id` selects one of the user's company memberships; the server verifies that membership and establishes the tenant stored on the session. Tenant APIs never accept a company identifier.

| Method | Path | Required permission | Purpose |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/login` | Public | Start a company-scoped session |
| POST | `/api/v1/auth/logout` | Authenticated | Revoke the current session |
| GET | `/api/v1/auth/session` | Authenticated | Validate the current session |
| GET | `/api/v1/company` | Authenticated | Read the session company |
| GET, POST | `/api/v1/employees` | `employees.view`, `employees.manage` | List or create employees |
| PATCH | `/api/v1/employees/{employee_id}` | `employees.manage` | Change employee status |
| POST | `/api/v1/user-access` | `employees.manage` | Create a login and grant company access |
| PATCH | `/api/v1/user-access/{user_id}` | `employees.manage` | Change company access status |
| PUT | `/api/v1/user-access/{user_id}/roles` | `roles.manage` | Replace company role assignments |
| GET, POST | `/api/v1/roles` | `roles.view`, `roles.manage` | List or create company roles |
| PUT | `/api/v1/roles/{role_id}/permissions` | `roles.manage` | Replace role permissions |
| GET | `/api/v1/permissions` | `roles.view` | List permission definitions |
| GET | `/api/v1/module-entitlements` | `settings.manage` | List company module access |
| PUT | `/api/v1/module-entitlements/{module_key}` | `settings.manage` | Enable or disable a module |
| GET | `/api/v1/audit-logs` | `settings.manage` | Read recent company audit entries |

The `core` entitlement is always enabled. Entitlements represent technical module access only and contain no billing or pricing behavior.

The OpenAPI source is `docs/openapi.yaml`. It must be updated whenever the public API contract changes.
