# Data lifecycle operations

CommerceOps Phase 24 provides an operator-run backup, restore and archival-planning toolkit.
It protects the PostgreSQL database and the object files referenced by it as one verified
snapshot. The toolkit does not delete data and does not contain production credentials.

## Safety contract

- Store backups outside the source checkout and outside the live object directory.
- Supply the database URL only through `COMMERCEOPS_LIFECYCLE_DATABASE_URL`. The tooling passes
  connection fields to PostgreSQL commands through their environment, so the password does not
  appear in command arguments.
- Stop application writers, or take an infrastructure-level consistent snapshot, before backup.
  The database dump uses a serializable snapshot and object bytes are checked before and after
  copying, but those checks cannot make two independently changing systems atomic.
- Treat a backup as immutable. Validation rejects symlinks, path traversal, extra or missing
  files, changed bytes and duplicate checksum entries.
- Restore only into a newly created empty database and a path that does not exist. The toolkit
  refuses a nonempty database or existing object destination.
- Keep the restore receipt and archive plan outside the backup and restored object tree.
- Never use a dry-run archive plan as deletion authorization. Phase 24 has no deletion command.

For S3-compatible production storage, first create a provider-native, versioned and access-
protected export in a private staging location, then run the object snapshot against that stable
export. Provider credentials, bucket policies, scheduling and retention locks belong in the
deployment environment. This repository does not implement or claim a live S3 restore drill.

## Prerequisites

The operator host needs Python 3 and PostgreSQL client programs compatible with the server:
`pg_dump`, `pg_restore` and `psql`. Set the database URL without writing it to the repository:

```sh
export COMMERCEOPS_LIFECYCLE_DATABASE_URL='postgres://...'
```

Choose protected output locations. The examples use `/srv/commerceops-protected`; adapt that
path to the environment's encrypted and access-controlled storage.

## Create and verify a backup

```sh
make lifecycle-backup \
  BACKUP_DIR=/srv/commerceops-protected/backup-2026-09-11 \
  OBJECT_ROOT=/srv/commerceops-objects

make lifecycle-validate \
  BACKUP_DIR=/srv/commerceops-protected/backup-2026-09-11
```

The backup contains:

- `database.dump`: custom-format PostgreSQL dump;
- `objects/`: byte-for-byte object snapshot;
- `manifests/BACKUP.json`: snapshot metadata and tool versions;
- `manifests/DATABASE_CATALOG.json`: public table row counts and migration state;
- `manifests/OBJECTS.json`: object inventory and database references; and
- `SHA256SUMS`: checksum inventory for every protected file.

The backup command refuses an existing destination, an object source containing symlinks or
unfinished `.upload` files, and any missing or mismatched database-referenced object.

## Perform the restore drill

Create a disposable empty database. Do not point this command at a development or production
database. Set the lifecycle URL to that database, then restore:

```sh
export COMMERCEOPS_LIFECYCLE_DATABASE_URL='postgres://.../commerceops_restore_drill'

make lifecycle-restore \
  BACKUP_DIR=/srv/commerceops-protected/backup-2026-09-11 \
  RESTORE_OBJECT_DIR=/srv/commerceops-restore-drill/objects \
  RESTORE_RECEIPT=/srv/commerceops-restore-drill/restore-receipt.json
```

The command validates the backup before restoring, compares every public table count and the
migration ledger, compares all database object references, verifies the restored object bytes,
and only then writes the receipt. A receipt records evidence for one backup and always contains
`deletion_authorized: false`.

Recovery from failure is intentionally simple: discard the disposable database and restore
directory, investigate the first reported mismatch, validate the immutable backup again, then
repeat with newly empty destinations. Never repair a backup in place.

## Produce an archival dry run

An archive plan requires a matching successful restore receipt:

```sh
make lifecycle-archive-plan \
  BACKUP_DIR=/srv/commerceops-protected/backup-2026-09-11 \
  RESTORE_RECEIPT=/srv/commerceops-restore-drill/restore-receipt.json \
  ARCHIVE_BEFORE=2025-09-11T00:00:00Z \
  ARCHIVE_PLAN=/srv/commerceops-restore-drill/archive-plan.json
```

The plan lists old object references that are eligible for later policy review and those blocked
by active processing or unfinished printer delivery. Inactive Print Library assets can be
candidates; active assets cannot. The plan records the preserved counts of audit logs, Inventory
transactions and Trace Box events. It writes a checksum sidecar and always records
`mode: dry_run` and `deletion_enabled: false`.

No default retention duration is encoded. The business owner must approve retention periods,
legal obligations, responsible operators, review frequency and recovery objectives during Phase
25 before any destructive workflow can be designed. Until scheduled production drills exist,
CommerceOps makes no recovery-point or recovery-time guarantee.

