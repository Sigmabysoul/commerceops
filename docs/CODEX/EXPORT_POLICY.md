# Source and history export policy

The recreated shell entrypoints use Python 3 standard-library ZIP, path and checksum
handling plus Git. This is local development tooling, not a second business backend.
No additional package dependency is required. The scripts are newly authored from the
report requirements; they are not the unavailable original ZIP's exact scripts.

Run from the source repository:

```bash
bash scripts/export/create_commerceops_export.sh /absolute/path/new-export --ref COMMIT
bash scripts/export/validate_commerceops_export.sh /absolute/path/new-export
bash scripts/export/clone_bundle_to_new_folder.sh /absolute/path/new-export /absolute/path/new-checkout
```

The clone helper consumes the validated export DIRECTORY (not a bare bundle), so the
recorded exact commit and integrity manifest travel with it. An optional `--overlay DIR`
copies only AGENTS.md, docs/ and scripts/export/; never the project README or application.
It creates the Phase 15 branch and removes the local bundle remote. It never attaches a
network remote, pushes, overwrites an existing destination or migrates a database.

Creation refuses a dirty tree unless `--allow-dirty` is explicit. The source ZIP always
represents the selected committed ref. In dirty mode tracked changes are separate binary
patches and nonignored untracked files are a separate ZIP; no work is applied automatically.
Prefer a committed preservation ref and a clean exact baseline for migrations.

Every export includes all reachable refs/history, an exact source archive, Git state,
tracked-file list, history blob sizes, migration hashes, bundle validation and SHA256SUMS.
Validation checks checksums, test-clones the bundle, selects the recorded commit and
compares archive paths/content and migration hashes against that commit.

Known secret/local-data paths, historical private-key/token markers and blobs over 50 MiB
are refused pending review. This targeted check does not prove absence of secrets or
production data. Review history before sharing. Rotate/remediate real exposed credentials;
do not conceal them by deleting only today's file. Never publish raw review findings.

Do not include real .env, production database dumps, production documents, object-store
contents, node_modules, build caches or printer journals. .env.example and sanitized
tracked fixtures may be included. Operational backups remain separate and protected.
Checksums detect corruption; they do not authenticate an untrusted package or encrypt it.

Test with `PYTHONDONTWRITEBYTECODE=1 python3 scripts/export/test_export.py`.
