#!/usr/bin/env python3
"""Regression tests for Phase 25 production preflight, smoke, release, and rollback evidence."""

import importlib.util
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import threading
import tempfile
import unittest


spec = importlib.util.spec_from_file_location("readiness", Path(__file__).with_name("readiness.py"))
readiness = importlib.util.module_from_spec(spec)
spec.loader.exec_module(readiness)


class HealthHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/api/v1/health":
            self.send_error(404)
            return
        body = b'{"status":"ok","database":"ok"}'
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("X-Frame-Options", "DENY")
        self.send_header("Referrer-Policy", "no-referrer")
        self.send_header("X-Request-ID", self.headers.get("X-Request-ID", "generated"))
        if self.headers.get("Origin"):
            self.send_header("Access-Control-Allow-Origin", self.headers["Origin"])
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass


class ReadinessTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="commerceops-readiness-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)

    def environment(self):
        return {
            "APP_ENV": "production",
            "DATABASE_URL": "postgresql://operator:a-secure-password@db.ops.internal/commerceops?sslmode=require",
            "CORS_ALLOWED_ORIGINS": "https://ops.acme.test",
            "NEXT_PUBLIC_API_BASE_URL": "https://api.acme.test",
            "OBJECT_STORAGE_DRIVER": "s3",
            "OBJECT_STORAGE_ENDPOINT": "https://objects.acme.test",
            "OBJECT_STORAGE_BUCKET": "commerceops-live",
            "OBJECT_STORAGE_REGION": "us-east-1",
            "OBJECT_STORAGE_ACCESS_KEY": "accesskey123",
            "OBJECT_STORAGE_SECRET_KEY": "secretkey1234567890",
            "COMMERCEOPS_API_IMAGE": "registry.acme.test/commerceops-api@sha256:" + "a" * 64,
            "COMMERCEOPS_WEB_IMAGE": "registry.acme.test/commerceops-web@sha256:" + "b" * 64,
        }

    def write_environment(self, values):
        path = self.root / "production.env"
        path.write_text("".join(f"{key}={value}\n" for key, value in values.items()))
        return path

    def test_production_config_requires_tls_s3_and_immutable_images(self):
        values = self.environment()
        self.assertTrue(readiness.check_config(self.write_environment(values))["immutable_images"])
        for key, unsafe, message in (
            ("DATABASE_URL", "postgresql://user:password@db/app?sslmode=disable", "TLS"),
            ("CORS_ALLOWED_ORIGINS", "http://ops.acme.test", "HTTPS"),
            ("OBJECT_STORAGE_DRIVER", "local", "must be s3"),
            ("COMMERCEOPS_API_IMAGE", "commerceops-api:latest", "immutable"),
        ):
            broken = dict(values)
            broken[key] = unsafe
            with self.assertRaisesRegex(readiness.ReadinessError, message):
                readiness.check_config(self.write_environment(broken))

    def test_concurrent_smoke_records_latency_without_failures(self):
        server = ThreadingHTTPServer(("127.0.0.1", 0), HealthHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        result = readiness.smoke(f"http://127.0.0.1:{server.server_port}", 20, 4, "https://ops.acme.test")
        self.assertEqual(result["requests"], 20)
        self.assertEqual(result["failures"], 0)
        self.assertGreaterEqual(result["latency_ms"]["max"], result["latency_ms"]["median"])

    def backup_and_receipt(self):
        backup = self.root / "backup"
        (backup / "manifests").mkdir(parents=True)
        (backup / "objects").mkdir()
        (backup / "database.dump").write_bytes(b"fixture")
        readiness.lifecycle.write_json(backup / "manifests/BACKUP.json", {
            "format_version": 1, "deletion_enabled": False, "object_count": 0,
            "database_reference_count": 0,
        })
        readiness.lifecycle.write_json(backup / "manifests/DATABASE_CATALOG.json", {
            "table_counts": {}, "schema_migrations": [{"dirty": False, "version": 30}],
        })
        readiness.lifecycle.write_json(backup / "manifests/OBJECTS.json", {"objects": [], "database_references": []})
        (backup / "SHA256SUMS").write_text(readiness.lifecycle.checksum_manifest(backup))
        validated = readiness.lifecycle.validate_backup(backup)
        receipt = self.root / "receipt.json"
        readiness.lifecycle.write_json(receipt, {
            "backup_sha256": validated["backup_sha256"],
            "database_catalog_sha256": readiness.lifecycle.digest_file(backup / "manifests/DATABASE_CATALOG.json"),
            "object_manifest_sha256": readiness.lifecycle.digest_file(backup / "manifests/OBJECTS.json"),
            "restored_table_counts_match": True, "deletion_authorized": False,
        })
        return backup, receipt

    def test_release_is_backup_bound_and_rollback_requires_same_schema(self):
        backup, receipt = self.backup_and_receipt()
        api = "registry.acme.test/api@sha256:" + "a" * 64
        web = "registry.acme.test/web@sha256:" + "b" * 64
        current = self.root / "current.json"
        previous = self.root / "previous.json"
        readiness.create_release(current, api, web, "c" * 40, 30, backup, receipt)
        readiness.create_release(previous, api, web, "d" * 40, 30, backup, receipt)
        plan = readiness.plan_rollback(current, previous, self.root / "rollback.json")
        self.assertEqual(plan["mode"], "dry_run")
        self.assertFalse(plan["database_down_migration_required"])
        self.assertFalse(plan["deployment_authorized"])

        value = json.loads(previous.read_text())
        value["migration_version"] = 29
        previous.write_text(json.dumps(value))
        Path(str(previous) + ".sha256").write_text(f"{readiness.digest(previous)}  {previous.name}\n")
        with self.assertRaisesRegex(readiness.ReadinessError, "identical migration"):
            readiness.plan_rollback(current, previous, self.root / "refused.json")

    def test_release_refuses_mutable_image_and_corrupt_receipt(self):
        backup, receipt = self.backup_and_receipt()
        with self.assertRaisesRegex(readiness.ReadinessError, "immutable"):
            readiness.create_release(self.root / "release.json", "api:latest", "web@sha256:" + "b" * 64, "c" * 40, 30, backup, receipt)
        receipt.write_text("{}")
        with self.assertRaisesRegex(readiness.ReadinessError, "does not match"):
            readiness.create_release(self.root / "release.json", "api@sha256:" + "a" * 64, "web@sha256:" + "b" * 64, "c" * 40, 30, backup, receipt)

    def test_release_manifest_cannot_modify_backup(self):
        backup, receipt = self.backup_and_receipt()
        with self.assertRaisesRegex(readiness.ReadinessError, "outside the immutable backup"):
            readiness.create_release(backup / "release.json", "api@sha256:" + "a" * 64, "web@sha256:" + "b" * 64, "c" * 40, 30, backup, receipt)


if __name__ == "__main__":
    unittest.main(verbosity=2)
