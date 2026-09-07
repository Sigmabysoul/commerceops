# Git and rollback strategy

Source baseline: d90e4f7c0ceab032bc33a0618e1ceabdc20906b5.
Deferred WIP ref: me/preserve-marketplace-accounts-20260907.
Implementation branch: me/phase-15-repository-architecture in commerceops-next.

Preserve the source checkout, use a verified bundle, and select the exact baseline rather
than the bundle's default branch. Keep planning/docs separate from each tested package
move. Revert a failed committed batch or reclone into a new directory; never reset the
original checkout. Do not rewrite shared history. Retain backups outside source control.

Completion records local verification only unless remote CI actually runs. No remote
creation, push, default-branch rename, merge, deployment or deletion of old checkouts is
part of this task. Phase 16 starts only with explicit owner authorization.
