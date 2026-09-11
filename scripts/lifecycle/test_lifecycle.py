#!/usr/bin/env python3
"""Lifecycle integrity tests, including an optional real PostgreSQL restore drill."""

import importlib.util
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from urllib.parse import urlsplit, urlunsplit


spec = importlib.util.spec_from_file_location("lifecycle", Path(__file__).with_name("lifecycle.py"))
lifecycle = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lifecycle)


class LifecycleManifestTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="commerceops-lifecycle-unit-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.backup = self.root / "backup"
        (self.backup / "manifests").mkdir(parents=True)
        (self.backup / "objects" / "tenant").mkdir(parents=True)
        data = b"object-data"
        (self.backup / "objects" / "tenant" / "source.pdf").write_bytes(data)
        (self.backup / "database.dump").write_bytes(b"fixture-dump")
        objects = [{"key": "tenant/source.pdf", "size_bytes": len(data), "sha256": lifecycle.digest_bytes(data)}]
        references = [{"kind": "source_file", **objects[0], "created_at": "2026-01-01T00:00:00.000000Z", "archive_eligible": True}]
        lifecycle.write_json(self.backup / "manifests" / "BACKUP.json", {
            "format_version": 1, "deletion_enabled": False, "object_count": 1,
            "database_reference_count": 1,
        })
        lifecycle.write_json(self.backup / "manifests" / "DATABASE_CATALOG.json", {
            "table_counts": {"audit_logs": 2, "inventory_transactions": 3, "trace_box_events": 4},
            "schema_migrations": [],
        })
        lifecycle.write_json(self.backup / "manifests" / "OBJECTS.json", {"objects": objects, "database_references": references})
        (self.backup / "SHA256SUMS").write_text(lifecycle.checksum_manifest(self.backup))

    def test_validation_corruption_traversal_extra_and_symlink_refused(self):
        lifecycle.validate_backup(self.backup)
        with (self.backup / "database.dump").open("ab") as handle:
            handle.write(b"corrupt")
        with self.assertRaisesRegex(lifecycle.LifecycleError, "checksum verification"):
            lifecycle.validate_backup(self.backup)
        (self.backup / "database.dump").write_bytes(b"fixture-dump")
        (self.backup / "SHA256SUMS").write_text(lifecycle.checksum_manifest(self.backup))
        with (self.backup / "SHA256SUMS").open("a") as handle:
            handle.write("0" * 64 + "  ../outside\n")
        with self.assertRaisesRegex(lifecycle.LifecycleError, "relative path"):
            lifecycle.validate_backup(self.backup)
        (self.backup / "SHA256SUMS").write_text(lifecycle.checksum_manifest(self.backup))
        (self.backup / "extra").write_text("unexpected")
        with self.assertRaisesRegex(lifecycle.LifecycleError, "inventory differs"):
            lifecycle.validate_backup(self.backup)
        (self.backup / "extra").unlink()
        target = self.backup / "objects" / "tenant" / "source.pdf"
        target.unlink()
        target.symlink_to(self.root / "outside")
        (self.backup / "SHA256SUMS").write_text(lifecycle.checksum_manifest(self.backup))
        with self.assertRaisesRegex(lifecycle.LifecycleError, "unsafe|manifest verification"):
            lifecycle.validate_backup(self.backup)

    def test_archive_plan_requires_matching_restore_and_never_enables_deletion(self):
        validated = lifecycle.validate_backup(self.backup)
        receipt = self.root / "receipt.json"
        lifecycle.write_json(receipt, {
            "backup_sha256": validated["backup_sha256"],
            "database_catalog_sha256": lifecycle.digest_file(self.backup / "manifests" / "DATABASE_CATALOG.json"),
            "object_manifest_sha256": lifecycle.digest_file(self.backup / "manifests" / "OBJECTS.json"),
            "restored_table_counts_match": True,
            "deletion_authorized": False,
        })
        output = self.root / "archive-plan.json"
        plan = lifecycle.plan_archive(self.backup, receipt, "2026-02-01T00:00:00Z", output)
        self.assertEqual(plan["candidate_count"], 1)
        self.assertFalse(plan["deletion_enabled"])
        self.assertEqual(plan["permanently_preserved_history"]["audit_logs"], 2)
        self.assertTrue(Path(str(output) + ".sha256").is_file())
        with self.assertRaisesRegex(lifecycle.LifecycleError, "destination already exists"):
            lifecycle.plan_archive(self.backup, receipt, "2026-02-01T00:00:00Z", output)
        lifecycle.write_json(receipt, {"backup_sha256": "0" * 64})
        output.unlink()
        Path(str(output) + ".sha256").unlink()
        with self.assertRaisesRegex(lifecycle.LifecycleError, "does not prove"):
            lifecycle.plan_archive(self.backup, receipt, "2026-02-01T00:00:00Z", output)

    def test_in_progress_and_symlinked_object_sources_are_refused(self):
        source = self.root / "source"
        source.mkdir()
        pending = source / "pending.upload"
        pending.write_text("partial")
        with self.assertRaisesRegex(lifecycle.LifecycleError, "in-progress upload"):
            lifecycle.scan_object_root(source)

        pending.unlink()
        outside = self.root / "outside"
        outside.write_text("external")
        (source / "linked-object").symlink_to(outside)
        with self.assertRaisesRegex(lifecycle.LifecycleError, "symlinks"):
            lifecycle.scan_object_root(source)


class PostgreSQLRestoreDrill(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        base = os.environ.get("TEST_DATABASE_URL", "")
        if not base:
            raise unittest.SkipTest("TEST_DATABASE_URL is not set")
        cls.temporary = tempfile.TemporaryDirectory(prefix="commerceops-lifecycle-pg-")
        cls.root = Path(cls.temporary.name)
        token = hashlib.sha256(str(cls.root).encode()).hexdigest()[:12]
        cls.source_name = "lifecycle_source_" + token
        cls.target_name = "lifecycle_target_" + token
        cls.blocked_name = "lifecycle_blocked_" + token
        cls.base = base
        cls.urls = {name: cls.database_url(base, name) for name in (cls.source_name, cls.target_name, cls.blocked_name)}
        for name in cls.urls:
            cls.database_tool("createdb", name)

    @classmethod
    def tearDownClass(cls):
        for name in getattr(cls, "urls", {}):
            cls.database_tool("dropdb", "--force", "--if-exists", name, check=False)
        if hasattr(cls, "temporary"):
            cls.temporary.cleanup()

    @staticmethod
    def database_url(base, name):
        parsed = urlsplit(base)
        return urlunsplit((parsed.scheme, parsed.netloc, "/" + name, parsed.query, parsed.fragment))

    @classmethod
    def database_tool(cls, command, *args, check=True):
        environment = lifecycle.database_environment(cls.base)
        name = args[-1]
        if not lifecycle.IDENTIFIER.fullmatch(name):
            raise ValueError("unsafe test database name")
        statement = f'CREATE DATABASE "{name}"' if command == "createdb" else f'DROP DATABASE IF EXISTS "{name}" WITH (FORCE)'
        return subprocess.run(
            ["psql", "-X", "-v", "ON_ERROR_STOP=1", "-c", statement],
            env=environment, capture_output=True, check=check,
        )

    @staticmethod
    def sql(url, statement):
        environment = lifecycle.database_environment(url)
        return subprocess.check_output(["psql", "-X", "-v", "ON_ERROR_STOP=1", "-A", "-t", "-c", statement], env=environment).decode().strip()

    def test_database_objects_restore_refusal_and_archive_dry_run(self):
        source = self.urls[self.source_name]
        self.sql(source, """
          CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL);
          CREATE TABLE source_files(id text PRIMARY KEY,company_id text,storage_key text,size_bytes bigint,sha256 text,created_at timestamptz);
          CREATE TABLE processing_jobs(id text PRIMARY KEY,company_id text,source_file_id text,status text);
          CREATE TABLE audit_logs(id integer PRIMARY KEY,note text);
          CREATE TABLE inventory_transactions(id integer PRIMARY KEY,quantity integer);
          CREATE TABLE trace_box_events(id integer PRIMARY KEY,event_type text);
          INSERT INTO schema_migrations VALUES(30,false);
          INSERT INTO audit_logs VALUES(1,'preserve');
          INSERT INTO inventory_transactions VALUES(1,2);
          INSERT INTO trace_box_events VALUES(1,'box_created');
        """)
        object_root = self.root / "source-objects"
        key = Path("company/source.pdf")
        (object_root / key.parent).mkdir(parents=True)
        contents = b"recoverable-object"
        (object_root / key).write_bytes(contents)
        self.sql(source, "INSERT INTO source_files VALUES('source','company','company/source.pdf',%d,'%s','2026-01-01T00:00:00Z'); INSERT INTO processing_jobs VALUES('job','company','source','processed')" % (len(contents), lifecycle.digest_bytes(contents)))
        backup = self.root / "backup"
        os.environ["LIFECYCLE_SOURCE_URL"] = source
        validated = lifecycle.create_backup(backup, object_root, "LIFECYCLE_SOURCE_URL")
        self.assertEqual(validated["metadata"]["object_count"], 1)
        with self.assertRaisesRegex(lifecycle.LifecycleError, "destination already exists"):
            lifecycle.create_backup(backup, object_root, "LIFECYCLE_SOURCE_URL")
        missing_root = self.root / "missing-objects"
        missing_root.mkdir()
        with self.assertRaisesRegex(lifecycle.LifecycleError, "missing from the snapshot"):
            lifecycle.create_backup(self.root / "missing-backup", missing_root, "LIFECYCLE_SOURCE_URL")
        self.sql(self.urls[self.blocked_name], "CREATE TABLE occupied(id integer)")
        os.environ["LIFECYCLE_BLOCKED_URL"] = self.urls[self.blocked_name]
        with self.assertRaisesRegex(lifecycle.LifecycleError, "not empty"):
            lifecycle.restore_backup(backup, self.root / "blocked-objects", self.root / "blocked-receipt.json", "LIFECYCLE_BLOCKED_URL")
        os.environ["LIFECYCLE_TARGET_URL"] = self.urls[self.target_name]
        object_restore = self.root / "restored-objects"
        receipt = self.root / "restore-receipt.json"
        lifecycle.restore_backup(backup, object_restore, receipt, "LIFECYCLE_TARGET_URL")
        self.assertEqual(self.sql(self.urls[self.target_name], "SELECT note FROM audit_logs"), "preserve")
        self.assertEqual((object_restore / key).read_bytes(), contents)
        evidence = json.loads(receipt.read_text())
        self.assertTrue(evidence["restored_table_counts_match"])
        self.assertFalse(evidence["deletion_authorized"])
        plan = lifecycle.plan_archive(backup, receipt, "2026-02-01T00:00:00Z", self.root / "archive.json")
        self.assertEqual(plan["candidate_count"], 1)
        self.assertEqual(plan["permanently_preserved_history"], {"audit_logs": 1, "inventory_transactions": 1, "trace_box_events": 1})
        self.assertFalse(plan["deletion_enabled"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
