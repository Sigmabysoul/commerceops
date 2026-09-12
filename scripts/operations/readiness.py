#!/usr/bin/env python3
"""Validate a CommerceOps release, measure readiness, and prepare rollback evidence."""

import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import statistics
import sys
import time
from urllib.error import HTTPError
from urllib.parse import parse_qsl, urlsplit
from urllib.request import Request, urlopen


ROOT = Path(__file__).resolve().parents[2]
LIFECYCLE_PATH = ROOT / "scripts/lifecycle/lifecycle.py"
spec = importlib.util.spec_from_file_location("commerceops_lifecycle", LIFECYCLE_PATH)
lifecycle = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lifecycle)

SHA256 = re.compile(r"^[0-9a-f]{64}$")
COMMIT = re.compile(r"^[0-9a-f]{40}$")
IMAGE = re.compile(r"^[A-Za-z0-9._:/-]+@sha256:[0-9a-f]{64}$")
REQUEST_ID = re.compile(r"^[A-Za-z0-9._-]{1,64}$")
REQUIRED_CONFIG = {
    "APP_ENV", "DATABASE_URL", "CORS_ALLOWED_ORIGINS", "NEXT_PUBLIC_API_BASE_URL",
    "OBJECT_STORAGE_DRIVER", "OBJECT_STORAGE_BUCKET", "OBJECT_STORAGE_REGION",
    "OBJECT_STORAGE_ACCESS_KEY", "OBJECT_STORAGE_SECRET_KEY", "COMMERCEOPS_API_IMAGE",
    "COMMERCEOPS_WEB_IMAGE",
}


class ReadinessError(Exception):
    """A preflight, smoke, release, or rollback safety refusal."""


def digest(path):
    value = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def write_json(path, value):
    if path.exists() or Path(str(path) + ".sha256").exists():
        raise ReadinessError("Output destination already exists")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")
    path.chmod(0o600)
    sidecar = Path(str(path) + ".sha256")
    sidecar.write_text(f"{digest(path)}  {path.name}\n")
    sidecar.chmod(0o600)


def load_environment(path):
    values = {}
    try:
        lines = path.read_text().splitlines()
    except OSError as error:
        raise ReadinessError("Environment file cannot be read") from error
    for number, line in enumerate(lines, 1):
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        key, separator, value = stripped.partition("=")
        if not separator or not re.fullmatch(r"[A-Z][A-Z0-9_]*", key):
            raise ReadinessError(f"Invalid environment assignment on line {number}")
        if key in values:
            raise ReadinessError(f"Duplicate environment setting {key}")
        values[key] = value
    return values


def absolute_https(value, name):
    parsed = urlsplit(value)
    if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ReadinessError(f"{name} must be an absolute HTTPS origin or URL")
    return parsed


def check_config(path, allow_local_images=False):
    values = load_environment(path)
    missing = sorted(key for key in REQUIRED_CONFIG if not values.get(key))
    if missing:
        raise ReadinessError("Missing production settings: " + ", ".join(missing))
    if values["APP_ENV"] != "production":
        raise ReadinessError("APP_ENV must be production")
    if values["OBJECT_STORAGE_DRIVER"] != "s3":
        raise ReadinessError("OBJECT_STORAGE_DRIVER must be s3")
    origins = values["CORS_ALLOWED_ORIGINS"].split(",")
    if len(origins) != len(set(origins)) or any(origin != origin.strip() for origin in origins):
        raise ReadinessError("CORS_ALLOWED_ORIGINS must be unique canonical origins")
    for origin in origins:
        parsed = absolute_https(origin, "CORS_ALLOWED_ORIGINS")
        if parsed.path not in {"", "/"} or origin.endswith("/"):
            raise ReadinessError("CORS_ALLOWED_ORIGINS must not contain a path or trailing slash")
    api_url = absolute_https(values["NEXT_PUBLIC_API_BASE_URL"], "NEXT_PUBLIC_API_BASE_URL")
    if api_url.path not in {"", "/"}:
        raise ReadinessError("NEXT_PUBLIC_API_BASE_URL must not contain a path")
    database = urlsplit(values["DATABASE_URL"])
    if database.scheme not in {"postgres", "postgresql"} or not database.hostname or not database.username or not database.password or not database.path.lstrip("/"):
        raise ReadinessError("DATABASE_URL must contain PostgreSQL host, database, user, and password")
    if dict(parse_qsl(database.query)).get("sslmode") == "disable":
        raise ReadinessError("DATABASE_URL must not disable TLS in production")
    endpoint = values.get("OBJECT_STORAGE_ENDPOINT", "")
    if endpoint:
        absolute_https(endpoint, "OBJECT_STORAGE_ENDPOINT")
    if len(values["OBJECT_STORAGE_ACCESS_KEY"]) < 8 or len(values["OBJECT_STORAGE_SECRET_KEY"]) < 16:
        raise ReadinessError("Object-storage credentials are implausibly short")
    placeholders = ("replace", "example", "changeme", "placeholder")
    for key in REQUIRED_CONFIG:
        if key not in {"CORS_ALLOWED_ORIGINS", "NEXT_PUBLIC_API_BASE_URL"} and any(word in values[key].lower() for word in placeholders):
            raise ReadinessError(f"{key} still contains a placeholder")
    if not allow_local_images:
        for key in ("COMMERCEOPS_API_IMAGE", "COMMERCEOPS_WEB_IMAGE"):
            if not IMAGE.fullmatch(values[key]):
                raise ReadinessError(f"{key} must use an immutable sha256 image digest")
    return {
        "status": "ready",
        "origin_count": len(origins),
        "database_tls_not_disabled": True,
        "immutable_images": all(IMAGE.fullmatch(values[key]) for key in ("COMMERCEOPS_API_IMAGE", "COMMERCEOPS_WEB_IMAGE")),
    }


def health_request(base_url, origin=None):
    headers = {"X-Request-ID": "phase25-smoke"}
    if origin:
        headers["Origin"] = origin
    request = Request(base_url.rstrip("/") + "/api/v1/health", headers=headers)
    started = time.perf_counter()
    with urlopen(request, timeout=10) as response:
        body = json.load(response)
        elapsed = (time.perf_counter() - started) * 1000
        headers = response.headers
        if response.status != 200 or body != {"status": "ok", "database": "ok"}:
            raise ReadinessError("Health response is not ready")
        for key, expected in {
            "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff",
            "X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer",
        }.items():
            if headers.get(key) != expected:
                raise ReadinessError(f"Health response is missing {key}")
        if not REQUEST_ID.fullmatch(headers.get("X-Request-ID", "")):
            raise ReadinessError("Health response is missing a valid request ID")
        if origin and headers.get("Access-Control-Allow-Origin") != origin:
            raise ReadinessError("Health response did not authorize the expected origin")
        return elapsed


def smoke(base_url, requests, concurrency, origin=None):
    if requests < 1 or requests > 10000 or concurrency < 1 or concurrency > 100 or concurrency > requests:
        raise ReadinessError("Smoke request and concurrency bounds are invalid")
    with ThreadPoolExecutor(max_workers=concurrency) as workers:
        latencies = list(workers.map(lambda _: health_request(base_url, origin), range(requests)))
    ordered = sorted(latencies)
    p95 = ordered[min(len(ordered) - 1, int(len(ordered) * 0.95))]
    return {
        "status": "passed", "requests": requests, "failures": 0, "concurrency": concurrency,
        "latency_ms": {"median": round(statistics.median(ordered), 3), "p95": round(p95, 3), "max": round(max(ordered), 3)},
    }


def validated_receipt(backup, receipt):
    validated = lifecycle.validate_backup(backup)
    try:
        evidence = json.loads(receipt.read_text())
    except (OSError, json.JSONDecodeError) as error:
        raise ReadinessError("Restore receipt is missing or invalid") from error
    expected = {
        "backup_sha256": validated["backup_sha256"],
        "database_catalog_sha256": lifecycle.digest_file(backup / "manifests/DATABASE_CATALOG.json"),
        "object_manifest_sha256": lifecycle.digest_file(backup / "manifests/OBJECTS.json"),
        "restored_table_counts_match": True,
        "deletion_authorized": False,
    }
    if any(evidence.get(key) != value for key, value in expected.items()):
        raise ReadinessError("Restore receipt does not match the validated backup")
    return validated


def create_release(output, api_image, web_image, commit, migration_version, backup, receipt):
    output = output.absolute()
    backup = backup.absolute()
    receipt = receipt.absolute()
    if backup == output or backup in output.parents or backup == Path(str(output) + ".sha256") or backup in Path(str(output) + ".sha256").parents:
        raise ReadinessError("Release manifest must be outside the immutable backup")
    if not IMAGE.fullmatch(api_image) or not IMAGE.fullmatch(web_image):
        raise ReadinessError("Release images must use immutable sha256 digests")
    if not COMMIT.fullmatch(commit) or migration_version < 1:
        raise ReadinessError("Release commit or migration version is invalid")
    validated = validated_receipt(backup, receipt)
    migrations = validated["catalog"].get("schema_migrations", [])
    if migrations != [{"dirty": False, "version": migration_version}]:
        raise ReadinessError("Backup migration state does not match the release")
    manifest = {
        "format_version": 1, "created_at": datetime.now(timezone.utc).isoformat(),
        "git_commit": commit, "migration_version": migration_version,
        "api_image": api_image, "web_image": web_image,
        "backup_sha256": validated["backup_sha256"], "restore_receipt_sha256": digest(receipt),
        "operator_acceptance": "pending", "deployment_authorized": False,
    }
    write_json(output, manifest)
    return manifest


def validate_release(path):
    sidecar = Path(str(path) + ".sha256")
    try:
        expected, separator, name = sidecar.read_text().strip().partition("  ")
        manifest = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as error:
        raise ReadinessError("Release manifest or checksum is invalid") from error
    if not separator or name != path.name or not SHA256.fullmatch(expected) or digest(path) != expected:
        raise ReadinessError("Release manifest checksum failed")
    if manifest.get("format_version") != 1 or not COMMIT.fullmatch(manifest.get("git_commit", "")):
        raise ReadinessError("Release manifest format is invalid")
    if not IMAGE.fullmatch(manifest.get("api_image", "")) or not IMAGE.fullmatch(manifest.get("web_image", "")):
        raise ReadinessError("Release manifest image is mutable or invalid")
    if manifest.get("deployment_authorized") is not False:
        raise ReadinessError("Release manifest must not authorize deployment")
    return manifest


def plan_rollback(current_path, previous_path, output):
    current = validate_release(current_path)
    previous = validate_release(previous_path)
    if current["migration_version"] != previous["migration_version"]:
        raise ReadinessError("Automatic application rollback requires identical migration versions")
    plan = {
        "format_version": 1, "mode": "dry_run", "created_at": datetime.now(timezone.utc).isoformat(),
        "failed_release_sha256": digest(current_path), "restore_release_sha256": digest(previous_path),
        "restore_api_image": previous["api_image"], "restore_web_image": previous["web_image"],
        "migration_version": previous["migration_version"], "database_down_migration_required": False,
        "deployment_authorized": False,
    }
    write_json(output, plan)
    return plan


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    config = commands.add_parser("check-config")
    config.add_argument("environment_file", type=Path)
    config.add_argument("--allow-local-images", action="store_true")
    probe = commands.add_parser("smoke")
    probe.add_argument("base_url")
    probe.add_argument("--requests", type=int, default=100)
    probe.add_argument("--concurrency", type=int, default=10)
    probe.add_argument("--origin")
    release = commands.add_parser("create-release")
    release.add_argument("output", type=Path)
    release.add_argument("--api-image", required=True)
    release.add_argument("--web-image", required=True)
    release.add_argument("--commit", required=True)
    release.add_argument("--migration-version", required=True, type=int)
    release.add_argument("--backup", required=True, type=Path)
    release.add_argument("--restore-receipt", required=True, type=Path)
    validate = commands.add_parser("validate-release")
    validate.add_argument("manifest", type=Path)
    rollback = commands.add_parser("plan-rollback")
    rollback.add_argument("current", type=Path)
    rollback.add_argument("previous", type=Path)
    rollback.add_argument("output", type=Path)
    args = parser.parse_args()
    try:
        if args.command == "check-config":
            result = check_config(args.environment_file, args.allow_local_images)
        elif args.command == "smoke":
            result = smoke(args.base_url, args.requests, args.concurrency, args.origin)
        elif args.command == "create-release":
            result = create_release(args.output, args.api_image, args.web_image, args.commit, args.migration_version, args.backup, args.restore_receipt)
        elif args.command == "validate-release":
            result = validate_release(args.manifest)
        else:
            result = plan_rollback(args.current, args.previous, args.output)
        print(json.dumps(result, indent=2, sort_keys=True))
    except (ReadinessError, lifecycle.LifecycleError, HTTPError, OSError, ValueError) as error:
        parser.exit(1, f"Readiness check failed: {error}\n")


if __name__ == "__main__":
    main()
