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

The OpenAPI source is `docs/openapi.yaml`. It must be updated whenever the public API contract changes.
