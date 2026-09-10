# CommerceOps local launcher

`CommerceOps` is a Linux desktop development launcher. Double-clicking it starts the existing
modular-monolith application and opens `http://localhost:3000`; it does not package or replace
the Go API, Next.js frontend, PostgreSQL or migration system.

## First run

The launcher checks for Docker, Go, pnpm, `xdg-open`, Poppler utilities and Tesseract (plus Docker Compose when starting the database). It then:

1. creates `.env` with a random local PostgreSQL password when `.env` does not exist;
2. starts the Compose PostgreSQL service and applies every existing migration;
3. creates one local administrator only when the database contains no users or companies;
4. installs the pinned frontend dependencies when `node_modules` is absent;
5. builds and starts the API and starts the Next.js development server; and
6. opens the web application and installs a CommerceOps entry in the desktop application menu.

The generated login is stored with owner-only permissions in
`.commerceops-local/credentials.txt`. Runtime output is written to
`.commerceops-local/launcher.log`. Uploaded files live in `.commerceops-local/uploads`. Both paths and the generated executable are ignored by Git.

## Stop and restart

The desktop application entry includes a **Stop CommerceOps** action. The equivalent command is
`./CommerceOps --stop`. PostgreSQL data remains in its Docker volume and is available on the
next launch.

If both recorded application processes are still running, another launch waits for readiness and opens the browser without restarting them. Process records include Linux start times so reused process IDs cannot stop unrelated apps.

## Safety boundaries

The launcher refuses to run unless `APP_ENV=development` and `DATABASE_URL` points to localhost.
It never overwrites an existing `.env`, never resets an existing database and never creates an
administrator after any user or company exists. It applies the repository's migration files through the
existing Compose migration container. The local administrator receives the existing permission rows
and local module entitlements; no authorization bypass is added to the server.

This launcher is for local development. Production deployment and workstation installation
remain Phase 25 work.

## Installation and troubleshooting

Build with `make local-launcher` and keep the resulting `CommerceOps` file in the repository
root. The menu entry is installed on the first successful launch. This is a Linux executable;
it is not a Windows `.exe` or a portable installer. The repository and prerequisite programs
must remain available.

API and frontend listeners bind to loopback, on ports 8080 and 3000. The existing Compose
PostgreSQL mapping uses port 5432. An existing configuration must match these local endpoints
and the Compose database credentials. Ports occupied by unrelated apps produce an error.

If startup fails, read `.commerceops-local/launcher.log`; desktop notifications also report
the failure when `notify-send` is available. An existing database keeps its existing login.
The launcher does not reset forgotten passwords. Do not delete an existing configuration
without keeping its database credentials.

Stop CommerceOps before running a frontend production build or `make verify-full`.
Next.js development and production builds share `apps/web/.next`.
The Compose migration bind mount uses the shared SELinux label so Fedora can read migrations.

## Verification evidence — 2026-09-10

- `make local-launcher`: passed; Linux executable created in the checkout root.
- `TEST_DATABASE_URL=<disposable migrated PostgreSQL> make verify-full`: passed.
  This ran formatting, Go vet, uncached Go tests with PostgreSQL enabled, all three executable
  builds, frontend typecheck, lint, production build and `git diff --check`.
- Launcher tests cover private configuration generation, preserving an existing configuration,
  local database guards, bootstrap and repeated bootstrap against migrated PostgreSQL,
  and refusing to signal a reused process ID.
- Live launcher smoke checks passed: frontend and API readiness, administrator login, session,
  permissions and products responses; repeated launch preserved both process identities;
  stopping closed ports 3000 and 8080; restarting preserved administrator login.
- Existing migrations and dependency manifests/lockfiles are unchanged.
- Optional private marketplace fixture tests were skipped because their environment variables
  were unset. Physical printer hardware, Windows packaging, remote CI and a manual
  desktop double-click were not verified.
- Earlier attempts exposed a Fedora migration-mount permission failure and a bootstrap SQL
  parameter-type error; both were fixed before the successful run. A concurrent Next.js dev/build
  attempt and a later disposable-database shutdown interrupted verification; the final full run
  completed successfully with the app stopped and PostgreSQL managed separately.

The full local verification output is retained in the ignored
`.commerceops-local/verify-full.log`. These results cover the launcher and existing regression
suite; they do not complete deferred marketplace work or production hardening.
