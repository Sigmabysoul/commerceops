#!/usr/bin/env python3
"""Protected CommerceOps database/object backup, restore drill, and archive planning."""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from urllib.parse import parse_qsl, unquote, urlsplit


FORMAT_VERSION = 1
DEFAULT_DATABASE_ENV = "COMMERCEOPS_LIFECYCLE_DATABASE_URL"
ENV_NAME = re.compile(r"^[A-Z][A-Z0-9_]*$")
SHA256 = re.compile(r"^[0-9a-f]{64}$")
IDENTIFIER = re.compile(r"^[a-z_][a-z0-9_]*$")


class LifecycleError(Exception):
    """An expected, safely reportable lifecycle refusal."""


def digest_bytes(data):
    return hashlib.sha256(data).hexdigest()


def digest_file(path):
    result = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")
    path.chmod(0o600)


def database_url(environment_name):
    if not ENV_NAME.fullmatch(environment_name):
        raise LifecycleError("Database environment variable name is invalid")
    value = os.environ.get(environment_name, "").strip()
    if not value:
        raise LifecycleError(f"{environment_name} is required")
    return value


def database_environment(url):
    environment = os.environ.copy()
    try:
        parsed = urlsplit(url)
        port = parsed.port
    except ValueError as error:
        raise LifecycleError("Database URL is invalid") from error
    if parsed.scheme not in {"postgres", "postgresql"} or not parsed.hostname or not parsed.username or not parsed.path.lstrip("/"):
        raise LifecycleError("Database URL must include PostgreSQL host, user, and database")
    environment.update({
        "PGHOST": parsed.hostname,
        "PGPORT": str(port or 5432),
        "PGUSER": unquote(parsed.username),
        "PGDATABASE": unquote(parsed.path.lstrip("/")),
    })
    if parsed.password is not None:
        environment["PGPASSWORD"] = unquote(parsed.password)
    else:
        environment.pop("PGPASSWORD", None)
    query_settings = dict(parse_qsl(parsed.query))
    for parameter, variable in {"sslmode": "PGSSLMODE", "connect_timeout": "PGCONNECT_TIMEOUT"}.items():
        if parameter in query_settings:
            environment[variable] = query_settings[parameter]
    return environment


def run_database(command, url, *, text=False):
    try:
        return subprocess.run(
            command,
            env=database_environment(url),
            capture_output=True,
            check=True,
            text=text,
        )
    except (OSError, subprocess.CalledProcessError) as error:
        # The URL stays in the child environment rather than the command or error message.
        raise LifecycleError(f"Database command failed: {command[0]}") from error


def query(url, sql):
    result = run_database(
        ["psql", "-X", "-v", "ON_ERROR_STOP=1", "-A", "-t", "-F", "\t", "-c", sql],
        url,
        text=True,
    )
    return [line.split("\t") for line in result.stdout.splitlines() if line]


def database_catalog(url):
    identity = query(url, "SELECT current_database(), current_setting('server_version_num')")
    if len(identity) != 1 or len(identity[0]) != 2:
        raise LifecycleError("Could not read database identity")
    tables = [row[0] for row in query(url, "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename")]
    counts = {}
    for table in tables:
        if not IDENTIFIER.fullmatch(table):
            raise LifecycleError("Unexpected database table name")
        rows = query(url, f'SELECT count(*) FROM public."{table}"')
        counts[table] = int(rows[0][0])
    migrations = []
    if "schema_migrations" in counts:
        migrations = [
            {"version": int(row[0]), "dirty": row[1] == "t"}
            for row in query(url, "SELECT version,dirty FROM schema_migrations ORDER BY version")
        ]
    return {
        "database_name": identity[0][0],
        "server_version_num": identity[0][1],
        "table_counts": counts,
        "schema_migrations": migrations,
    }


def table_exists(url, table):
    return query(url, f"SELECT to_regclass('public.{table}') IS NOT NULL")[0][0] == "t"


def object_references(url):
    statements = []
    if table_exists(url, "source_files") and table_exists(url, "processing_jobs"):
        statements.append("""
            SELECT 'source_file',s.storage_key,s.size_bytes,s.sha256,
              to_char(s.created_at AT TIME ZONE 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"'),
              NOT EXISTS (SELECT 1 FROM processing_jobs j WHERE j.company_id=s.company_id
                AND j.source_file_id=s.id AND j.status IN ('queued','processing','needs_review'))
            FROM source_files s
        """)
    if table_exists(url, "print_artifacts") and table_exists(url, "print_jobs"):
        pending = "FALSE"
        if table_exists(url, "printer_jobs"):
            pending = "EXISTS (SELECT 1 FROM printer_jobs delivery WHERE delivery.company_id=a.company_id AND delivery.print_artifact_id=a.id AND delivery.status IN ('queued','claimed','printing'))"
        statements.append(f"""
            SELECT 'print_artifact',a.storage_key,a.size_bytes,a.sha256,
              to_char(a.created_at AT TIME ZONE 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"'),
              j.status IN ('ready','failed') AND NOT ({pending})
            FROM print_artifacts a JOIN print_jobs j ON j.company_id=a.company_id AND j.id=a.print_job_id
        """)
    if table_exists(url, "print_library_assets"):
        pending = "FALSE"
        if table_exists(url, "printer_jobs"):
            pending = "EXISTS (SELECT 1 FROM printer_jobs delivery WHERE delivery.company_id=a.company_id AND delivery.print_library_asset_id=a.id AND delivery.status IN ('queued','claimed','printing'))"
        statements.append(f"""
            SELECT 'print_library_asset',a.storage_key,a.size_bytes,a.sha256,
              to_char(a.created_at AT TIME ZONE 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"'),
              NOT a.active AND NOT ({pending})
            FROM print_library_assets a
        """)
    if not statements:
        return []
    rows = query(url, " UNION ALL ".join(statements) + " ORDER BY 2,1")
    references = []
    for kind, key, size, sha256, created_at, eligible in rows:
        validate_relative_name(key)
        if not SHA256.fullmatch(sha256):
            raise LifecycleError("Database object reference has an invalid checksum")
        references.append({
            "kind": kind,
            "key": key,
            "size_bytes": int(size),
            "sha256": sha256,
            "created_at": created_at,
            "archive_eligible": eligible == "t",
        })
    return references


def validate_relative_name(name):
    relative = PurePosixPath(name)
    if not name or relative.is_absolute() or ".." in relative.parts or "." in relative.parts or "\\" in name:
        raise LifecycleError("Invalid relative path in lifecycle data")
    return relative


def scan_object_root(root):
    root = root.absolute()
    if root.is_symlink() or not root.is_dir():
        raise LifecycleError("Object source must be a real directory")
    records = []
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            raise LifecycleError("Object source symlinks are not supported")
        if path.is_dir():
            continue
        if not path.is_file():
            raise LifecycleError("Object source contains a non-regular file")
        name = path.relative_to(root).as_posix()
        validate_relative_name(name)
        if path.name.endswith(".upload"):
            raise LifecycleError("Object source contains an in-progress upload")
        stat = path.stat()
        records.append({"key": name, "size_bytes": stat.st_size, "sha256": digest_file(path)})
    return records


def verify_references(references, objects):
    by_key = {item["key"]: item for item in objects}
    if len(by_key) != len(objects):
        raise LifecycleError("Object manifest contains duplicate keys")
    for reference in references:
        item = by_key.get(reference["key"])
        if item is None:
            raise LifecycleError("Database references an object missing from the snapshot")
        if item["size_bytes"] != reference["size_bytes"] or item["sha256"] != reference["sha256"]:
            raise LifecycleError("Database and object snapshot metadata differ")


def checksum_manifest(directory):
    paths = sorted(path for path in directory.rglob("*") if path.is_file() and path.name != "SHA256SUMS")
    return "".join(f"{digest_file(path)}  {path.relative_to(directory).as_posix()}\n" for path in paths)


def create_backup(output, object_root, environment_name):
    output = output.absolute()
    object_root = object_root.absolute()
    if output.exists():
        raise LifecycleError("Backup destination already exists")
    if output == object_root or object_root in output.parents:
        raise LifecycleError("Backup destination must be outside the object source")
    url = database_url(environment_name)
    before_objects = scan_object_root(object_root)
    before_references = object_references(url)
    verify_references(before_references, before_objects)
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".commerceops-backup-", dir=output.parent) as temporary:
        stage = Path(temporary) / "backup"
        stage.mkdir(mode=0o700)
        (stage / "manifests").mkdir(mode=0o700)
        object_stage = stage / "objects"
        object_stage.mkdir(mode=0o700)
        run_database([
            "pg_dump", "--format=custom", "--compress=9", "--no-owner", "--no-privileges",
            "--serializable-deferrable", f"--file={stage / 'database.dump'}",
        ], url)
        (stage / "database.dump").chmod(0o600)
        for item in before_objects:
            source = object_root / Path(item["key"])
            target = object_stage / Path(item["key"])
            target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
            shutil.copyfile(source, target)
            target.chmod(0o600)
        after_objects = scan_object_root(object_root)
        after_references = object_references(url)
        if before_objects != after_objects or before_references != after_references:
            raise LifecycleError("Object source or database references changed during backup")
        copied_objects = scan_object_root(object_stage)
        if copied_objects != before_objects:
            raise LifecycleError("Copied object snapshot differs from its source")
        catalog = database_catalog(url)
        write_json(stage / "manifests" / "DATABASE_CATALOG.json", catalog)
        write_json(stage / "manifests" / "OBJECTS.json", {
            "objects": copied_objects,
            "database_references": before_references,
        })
        write_json(stage / "manifests" / "BACKUP.json", {
            "format_version": FORMAT_VERSION,
            "created_at": datetime.now(timezone.utc).isoformat(),
            "database_name": catalog["database_name"],
            "object_source_type": "protected-local-snapshot",
            "object_count": len(copied_objects),
            "database_reference_count": len(before_references),
            "restore_required_before_archive": True,
            "deletion_enabled": False,
        })
        (stage / "SHA256SUMS").write_text(checksum_manifest(stage))
        (stage / "SHA256SUMS").chmod(0o600)
        validate_backup(stage)
        stage.rename(output)
    return validate_backup(output)


def validate_backup(directory):
    directory = directory.absolute()
    if directory.is_symlink() or not directory.is_dir():
        raise LifecycleError("Backup directory is missing or is a symlink")
    checksum_path = directory / "SHA256SUMS"
    if checksum_path.is_symlink() or not checksum_path.is_file():
        raise LifecycleError("Backup checksum manifest is missing")
    covered = set()
    for line in checksum_path.read_text().splitlines():
        expected, separator, name = line.partition("  ")
        if not separator or not SHA256.fullmatch(expected) or name in covered:
            raise LifecycleError("Invalid backup checksum entry")
        validate_relative_name(name)
        path = directory / Path(name)
        if path.is_symlink() or not path.is_file() or not path.resolve().is_relative_to(directory.resolve()):
            raise LifecycleError("Backup checksum path is unsafe")
        if digest_file(path) != expected:
            raise LifecycleError("Backup checksum verification failed")
        covered.add(name)
    actual = {
        path.relative_to(directory).as_posix()
        for path in directory.rglob("*")
        if path.is_file() and path.name != "SHA256SUMS"
    }
    if covered != actual:
        raise LifecycleError("Backup file inventory differs from its checksum manifest")
    required = {
        "database.dump", "manifests/BACKUP.json", "manifests/DATABASE_CATALOG.json",
        "manifests/OBJECTS.json",
    }
    if not required.issubset(covered):
        raise LifecycleError("Backup is missing required files")
    try:
        metadata = json.loads((directory / "manifests/BACKUP.json").read_text())
        catalog = json.loads((directory / "manifests/DATABASE_CATALOG.json").read_text())
        object_manifest = json.loads((directory / "manifests/OBJECTS.json").read_text())
    except (json.JSONDecodeError, OSError) as error:
        raise LifecycleError("Backup manifest is invalid") from error
    if metadata.get("format_version") != FORMAT_VERSION or metadata.get("deletion_enabled") is not False:
        raise LifecycleError("Unsupported or unsafe backup format")
    objects = object_manifest.get("objects")
    references = object_manifest.get("database_references")
    if not isinstance(objects, list) or not isinstance(references, list):
        raise LifecycleError("Object manifest structure is invalid")
    if metadata.get("object_count") != len(objects) or metadata.get("database_reference_count") != len(references):
        raise LifecycleError("Backup object counts do not reconcile")
    for item in objects:
        validate_relative_name(item.get("key", ""))
        path = directory / "objects" / Path(item["key"])
        if not path.is_file() or path.is_symlink() or path.stat().st_size != item.get("size_bytes") or digest_file(path) != item.get("sha256"):
            raise LifecycleError("Object manifest verification failed")
    verify_references(references, objects)
    return {
        "backup_sha256": digest_bytes(checksum_path.read_bytes()),
        "metadata": metadata,
        "catalog": catalog,
        "objects": object_manifest,
    }


def target_is_empty(url):
    rows = query(url, """
        SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','S','f')
    """)
    return int(rows[0][0]) == 0


def restore_backup(directory, object_destination, receipt, environment_name):
    directory = directory.absolute()
    object_destination = object_destination.absolute()
    receipt = receipt.absolute()
    validated = validate_backup(directory)
    if object_destination.exists():
        raise LifecycleError("Object restore destination already exists")
    if receipt.exists():
        raise LifecycleError("Restore receipt destination already exists")
    if directory == object_destination or directory in object_destination.parents:
        raise LifecycleError("Object restore destination must be outside the immutable backup")
    if directory == receipt or directory in receipt.parents:
        raise LifecycleError("Restore receipt must be outside the immutable backup")
    if object_destination == receipt or object_destination in receipt.parents:
        raise LifecycleError("Restore receipt must be outside the restored object tree")
    url = database_url(environment_name)
    if not target_is_empty(url):
        raise LifecycleError("Restore target database is not empty")
    run_database([
        "pg_restore", "--exit-on-error", "--single-transaction", "--no-owner", "--no-privileges",
        "--dbname=",
        str(directory / "database.dump"),
    ], url)
    restored_catalog = database_catalog(url)
    expected_catalog = validated["catalog"]
    if restored_catalog.get("table_counts") != expected_catalog.get("table_counts") or restored_catalog.get("schema_migrations") != expected_catalog.get("schema_migrations"):
        raise LifecycleError("Restored database catalog does not match the backup")
    restored_references = object_references(url)
    if restored_references != validated["objects"]["database_references"]:
        raise LifecycleError("Restored database object references do not match the backup")
    object_destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = Path(tempfile.mkdtemp(prefix=".commerceops-objects-", dir=object_destination.parent))
    try:
        stage = temporary / "objects"
        shutil.copytree(directory / "objects", stage)
        if scan_object_root(stage) != validated["objects"]["objects"]:
            raise LifecycleError("Restored objects differ from the backup")
        stage.rename(object_destination)
    finally:
        if temporary.exists():
            shutil.rmtree(temporary)
    receipt.parent.mkdir(parents=True, exist_ok=True)
    write_json(receipt, {
        "format_version": FORMAT_VERSION,
        "restored_at": datetime.now(timezone.utc).isoformat(),
        "backup_sha256": validated["backup_sha256"],
        "database_catalog_sha256": digest_file(directory / "manifests/DATABASE_CATALOG.json"),
        "object_manifest_sha256": digest_file(directory / "manifests/OBJECTS.json"),
        "restored_table_counts_match": True,
        "restored_object_count": len(validated["objects"]["objects"]),
        "deletion_authorized": False,
    })
    return receipt


def parse_instant(value):
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as error:
        raise LifecycleError("Archive cutoff must be an RFC3339 instant") from error
    if parsed.tzinfo is None:
        raise LifecycleError("Archive cutoff must include a timezone offset")
    return parsed.astimezone(timezone.utc)


def plan_archive(directory, receipt, cutoff, output):
    directory = directory.absolute()
    receipt = receipt.absolute()
    output = output.absolute()
    checksum_output = Path(str(output) + ".sha256")
    if output.exists() or checksum_output.exists():
        raise LifecycleError("Archive-plan destination already exists")
    if (
        directory == output
        or directory in output.parents
        or directory == checksum_output
        or directory in checksum_output.parents
    ):
        raise LifecycleError("Archive plan must be outside the immutable backup")
    validated = validate_backup(directory)
    try:
        evidence = json.loads(receipt.read_text())
    except (OSError, json.JSONDecodeError) as error:
        raise LifecycleError("Restore receipt is missing or invalid") from error
    expected = {
        "backup_sha256": validated["backup_sha256"],
        "database_catalog_sha256": digest_file(directory / "manifests/DATABASE_CATALOG.json"),
        "object_manifest_sha256": digest_file(directory / "manifests/OBJECTS.json"),
        "restored_table_counts_match": True,
        "deletion_authorized": False,
    }
    if any(evidence.get(key) != value for key, value in expected.items()):
        raise LifecycleError("Restore receipt does not prove this backup was restored")
    before = parse_instant(cutoff)
    objects = {item["key"]: item for item in validated["objects"]["objects"]}
    candidates = []
    blocked = []
    for reference in validated["objects"]["database_references"]:
        created = parse_instant(reference["created_at"])
        if created >= before:
            continue
        item = {**reference, "snapshot": objects[reference["key"]]}
        if reference["archive_eligible"]:
            candidates.append(item)
        else:
            blocked.append(item)
    catalog_counts = validated["catalog"].get("table_counts", {})
    plan = {
        "format_version": FORMAT_VERSION,
        "mode": "dry_run",
        "created_at": datetime.now(timezone.utc).isoformat(),
        "cutoff": before.isoformat(),
        "backup_sha256": validated["backup_sha256"],
        "restore_receipt_sha256": digest_file(receipt),
        "deletion_enabled": False,
        "candidate_count": len(candidates),
        "blocked_count": len(blocked),
        "candidates": candidates,
        "blocked": blocked,
        "permanently_preserved_history": {
            key: catalog_counts.get(key, 0)
            for key in ("audit_logs", "inventory_transactions", "trace_box_events")
        },
    }
    output.parent.mkdir(parents=True, exist_ok=True)
    write_json(output, plan)
    checksum_output.write_text(f"{digest_file(output)}  {output.name}\n")
    checksum_output.chmod(0o600)
    return plan


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    backup = commands.add_parser("backup", help="Create an immutable database and local-object snapshot")
    backup.add_argument("output", type=Path)
    backup.add_argument("--object-root", type=Path, required=True)
    backup.add_argument("--database-url-env", default=DEFAULT_DATABASE_ENV)
    validate = commands.add_parser("validate", help="Verify checksums, inventory, and object references")
    validate.add_argument("directory", type=Path)
    restore = commands.add_parser("restore", help="Restore into an empty database and absent object directory")
    restore.add_argument("directory", type=Path)
    restore.add_argument("--object-destination", type=Path, required=True)
    restore.add_argument("--receipt", type=Path, required=True)
    restore.add_argument("--database-url-env", default=DEFAULT_DATABASE_ENV)
    archive = commands.add_parser("plan-archive", help="Create a non-deleting plan after a verified restore")
    archive.add_argument("directory", type=Path)
    archive.add_argument("--restore-receipt", type=Path, required=True)
    archive.add_argument("--before", required=True)
    archive.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        if args.command == "backup":
            result = create_backup(args.output, args.object_root, args.database_url_env)
            print("Verified backup SHA-256: " + result["backup_sha256"])
        elif args.command == "validate":
            result = validate_backup(args.directory)
            print("Verified backup SHA-256: " + result["backup_sha256"])
        elif args.command == "restore":
            print("Restore receipt: " + str(restore_backup(args.directory, args.object_destination, args.receipt, args.database_url_env)))
        else:
            result = plan_archive(args.directory, args.restore_receipt, args.before, args.output)
            print(f"Dry-run archive candidates: {result['candidate_count']}; deletion remains disabled")
    except (LifecycleError, OSError, KeyError, TypeError) as error:
        parser.exit(1, f"Lifecycle operation failed: {error}\n")


if __name__ == "__main__":
    main()
