# Security foundation

- Secrets belong in local or deployment environment configuration and must never be committed.
- `.env.example` contains placeholders; `.env` is ignored.
- `DATABASE_URL` is validated as required configuration.
- Health and error responses do not disclose database errors, connection strings, credentials, hosts, or stack traces.
- Development CORS allows only configured origins; wildcard origins are not used.
- Passwords are hashed with bcrypt and plaintext passwords are never stored.
- Sessions use 256-bit random opaque tokens; only SHA-256 token hashes are stored. Logout and disabled company access revoke active sessions.
- Session cookies are `HttpOnly` and `SameSite=Lax`; they are also `Secure` outside the development environment.
- Login verifies active user, company membership, and company status. The verified membership establishes the server-side company context for the session.
- Tenant APIs take company identity only from the authenticated principal. Tenant-owned database queries and composite foreign keys enforce company isolation.
- Backend authorization evaluates current granular permission assignments on every request. Role names are never authorization rules.
- Backend module entitlement checks are independent of billing or pricing.
- Security and administrative mutations produce company-scoped audit records containing actor, action, target, metadata, and timestamp.
