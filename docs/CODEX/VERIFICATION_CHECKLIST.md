# Phase 15 verification checklist

- Preserve source/index state, WIP ref and test-checkout patch; record exact baseline.
- Validate bundle, source ZIP, selected commit and checksum manifests.
- Fresh disposable PostgreSQL migrated through 000022; baseline make verify-full.
- Each batch: formatting, Go tests with PostgreSQL, vet/build and direct importer review.
- Final make verify-full: backend format/vet/uncached tests/server+agent builds and frontend typecheck/lint/build.
- Compare baseline migration filenames/bytes and OpenAPI/dependency hashes.
- Review fixture and migration relative paths, old imports, links and git diff --check.
- Test export integrity rejection, dirty-source policy, existing-destination refusal and exact-ref restoration.
- Record optional private fixture skips, physical hardware/browser limits and remote CI status explicitly.
- Clean committed tree and scope report; no Phase 16 or deployment.
