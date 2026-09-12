# Production rollout and rollback

This runbook is the Phase 25 release gate for CommerceOps internal rollout. It assumes one Go API,
one Next.js frontend, an externally managed PostgreSQL database, S3-compatible private object
storage, and an HTTPS reverse proxy. It does not provision infrastructure or authorize a release.

## Required owner decisions

Before the first rollout, record the production owner, operator, user group, maintenance window,
public frontend and API domains, recovery-point objective, recovery-time objective, backup
schedule, retention period, escalation contact and rollback decision maker. Keep that record with
the deployment system; do not put credentials or private infrastructure details in Git.

The frontend and API domains must be HTTPS and should be same-site subdomains so the secure
`SameSite=Lax` session cookie works predictably. TLS terminates at the reverse proxy. The Compose
ports bind to loopback and must not be exposed directly to the network.

## Build and publish immutable images

Build from a clean, reviewed commit. The frontend API address is compiled into its browser bundle:

```sh
make production-images NEXT_PUBLIC_API_BASE_URL=https://api.ops.example.com
```

Tag and push both images to the private registry, then resolve their registry-provided
`name@sha256:...` references. Release configuration and manifests reject mutable tags such as
`latest`. Production images run as non-root users. The API image includes the existing Poppler and
Tesseract runtime tools; the frontend uses Next.js standalone output. CI independently rebuilds
both images.

## Prepare configuration and recovery evidence

Copy `deploy/production.env.example` to an owner-readable file outside the repository and replace
every placeholder. Do not use the example as a real configuration. PostgreSQL TLS must not be
disabled, the browser and object-storage endpoints must use HTTPS, production object storage must
be S3-compatible, and both container references must be immutable digests.

```sh
chmod 600 /secure/commerceops-production.env
make production-config-check PRODUCTION_ENV_FILE=/secure/commerceops-production.env
docker compose --env-file /secure/commerceops-production.env -f compose.production.yml config --quiet
```

Follow `data-lifecycle.md` to create, validate and restore a current protected backup. The restore
receipt must match that exact backup. Then create the non-authorizing release evidence:

```sh
python3 scripts/operations/readiness.py create-release /secure/releases/release.json \
  --api-image 'registry.example.com/commerceops-api@sha256:...' \
  --web-image 'registry.example.com/commerceops-web@sha256:...' \
  --commit "$(git rev-parse HEAD)" --migration-version 30 \
  --backup /secure/backups/current \
  --restore-receipt /secure/restore-drill/receipt.json
```

The manifest binds the commit and image digests to verified recovery evidence. It records
`operator_acceptance: pending` and `deployment_authorized: false`; creating it never deploys.

## Roll out

1. Announce the maintenance window and stop operator writes.
2. Validate the backup and restore receipt again.
3. Pull the two immutable image references.
4. Run the migration service. It applies only checked-in forward migrations and must exit zero.
5. Start API and web services. The API waits for migration success; the web waits for API health.
6. Keep both ports loopback-only behind the configured HTTPS proxy.
7. Run the readiness smoke and concurrent health measurement:

```sh
docker compose --env-file /secure/commerceops-production.env -f compose.production.yml pull
docker compose --env-file /secure/commerceops-production.env -f compose.production.yml up -d

make production-smoke \
  API_BASE_URL=https://api.ops.example.com \
  CORS_ORIGIN=https://ops.example.com
```

8. Sign in with a non-admin acceptance account. Verify dashboard, Product lookup, one marketplace
   test fixture, Trace Box scan/view, Reporting view and a reversible Print queue action. Confirm
   company isolation and expected permissions with a restricted account.
9. Record operator name, UTC time, release-manifest checksum, each observed result and the rollout
   decision outside Git. Resume writes only after acceptance.

Do not use production customer orders for a smoke test when a dedicated acceptance fixture can
exercise the same path. Do not test a physical print unless the selected printer and paper are
part of the announced acceptance window.

## Observe and respond

The API emits JSON logs to stdout. Every request includes `request_id`, method, path, status,
response bytes and duration; every response returns the same `X-Request-ID`. Alert on readiness
failures, repeated 5xx responses, Automation lease failures, Printing delivery failures, object
storage errors, database saturation and disk pressure at the container host or proxy. Central log
retention and alert routing are environment-owned and must be tested before acceptance.

`/api/v1/health` is the readiness probe and returns unavailable when PostgreSQL cannot be reached.
Container restart state is not proof that the database or object store is safe. Inspect the API,
proxy, database and object-storage signals together during an incident.

## Application rollback

Keep the previous accepted release manifest and images available. Generate a rollback dry run:

```sh
python3 scripts/operations/readiness.py plan-rollback \
  /secure/releases/failed.json /secure/releases/previous.json \
  /secure/releases/rollback-plan.json
```

Automatic application rollback is refused unless both releases use the same migration version.
For an accepted dry run, update only the two image digest values to the previous manifest, validate
the Compose configuration, restart the services, and rerun smoke plus operator acceptance. The
plan never authorizes deployment and never runs a down migration.

If migration versions differ, stop. Assess forward compatibility first. Prefer a corrective
forward deployment. Restore the protected database/object snapshot only as an explicitly approved
incident recovery operation with operator writes stopped; never run an automatic destructive down
migration against production history.

## Shutdown and recovery drills

Send `SIGTERM` through the container runtime and confirm the API logs `http server shutting down`,
finishes within `SHUTDOWN_TIMEOUT`, exits zero and becomes ready again after restart. At the agreed
drill frequency, restore the latest protected database/object snapshot into isolated destinations,
compare the receipt, run smoke against the isolated API, and record measured duration. These
measurements establish real recovery objectives; local Phase 25 results do not establish them for
production.

