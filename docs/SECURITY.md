# Security foundation

- Secrets belong in local or deployment environment configuration and must never be committed.
- `.env.example` contains placeholders; `.env` is ignored.
- `DATABASE_URL` is validated as required configuration.
- Health and error responses do not disclose database errors, connection strings, credentials, hosts, or stack traces.
- Development CORS allows only configured origins; wildcard origins are not used.
- The backend remains the enforcement point for future authentication, authorization, entitlements, and tenant isolation.

Phase 0 does not implement authentication or tenant data because those belong to Phase 1.
