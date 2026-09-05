# Security foundation

- Secrets belong in local or deployment environment configuration and must never be committed.
- `.env.example` contains placeholders; `.env` is ignored.
- Object-storage credentials come only from deployment or local environment
  configuration. They are never exposed to the frontend or stored in business
  metadata.
- `DATABASE_URL` is validated as required configuration.
- Health and error responses do not disclose database errors, connection strings, credentials, hosts, or stack traces.
- Development CORS allows only configured origins; wildcard origins are not used.
- Passwords are hashed with bcrypt and plaintext passwords are never stored.
- Sessions use 256-bit random opaque tokens; only SHA-256 token hashes are stored. Logout and disabled company access revoke active sessions.
- Session cookies are `HttpOnly` and `SameSite=Lax`; they are also `Secure` outside the development environment.
- Login verifies active user, company membership, and company status. The verified membership establishes the server-side company context for the session.
- Tenant APIs take company identity only from the authenticated principal. Tenant-owned database queries and composite foreign keys enforce company isolation.
- Marketplace storage keys are generated server-side from the authenticated
  company context; storage drivers do not accept frontend tenant identity as
  authorization.
- Backend authorization evaluates current granular permission assignments on every request. Role names are never authorization rules.
- Backend module entitlement checks are independent of billing or pricing.
- Security and administrative mutations produce company-scoped audit records containing actor, action, target, metadata, and timestamp.
- Product and SKU mapping APIs derive company scope exclusively from the validated session principal and enforce `products.view` or `products.manage` in the backend.
- SKU resolution is exact and case-sensitive after trimming surrounding whitespace. Unknown, inactive, differently-cased, or marketplace-mismatched identifiers remain explicitly unresolved.
- Printer-agent credentials are independent 256-bit opaque bearer secrets,
  displayed once, stored only as SHA-256 hashes, scoped to one tenant/device,
  and revocable with the agent. Production transport must terminate TLS.
- Agent artifact downloads require the authenticated owning agent and the
  matching unexpired job lease. Storage keys and document contents never appear
  in agent job JSON, browser responses, logs, or audit metadata.
- Physical print requests contain a registered printer UUID, immutable artifact
  reference, and bounded copies only. They cannot supply an executable, local
  path, OS printer identifier, shell fragment, or arbitrary CUPS options.
- Print Library PDFs are size/signature/structure validated, hash-addressed in
  metadata, and stored under server-generated tenant keys. Agent downloads are
  verified against both persisted size and SHA-256 before local submission.
