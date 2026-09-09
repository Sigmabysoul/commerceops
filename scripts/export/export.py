#!/usr/bin/env python3
"""Local source/history transfer tooling; Python standard library only."""
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
import zipfile


def git(repo, *args):
    return subprocess.check_output(['git', '-C', str(repo), *args], stderr=subprocess.PIPE)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def forbidden(name):
    parts = PurePosixPath(name).parts
    base = parts[-1] if parts else ''
    return (any(p in {'node_modules', '.next', '.cache', '.pnpm-store'} for p in parts)
            or (base.startswith('.env') and base != '.env.example')
            or base.endswith(('.pem', '.key', '.dump', '.backup'))
            or base == 'printer-agent-journal.jsonl')


def review_history(repo):
    """Reject known forbidden files/high-confidence credentials; not a complete secret audit."""
    records = git(repo, 'rev-list', '--objects', '--all').decode().splitlines()
    pattern = re.compile(rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{36,}|sk-proj-[A-Za-z0-9_-]{40,}')
    sizes = []
    for record in records:
        oid, _, name = record.partition(' ')
        if not name:
            continue
        if forbidden(name):
            raise ValueError('Forbidden historical path; review before export: ' + name)
        if git(repo, 'cat-file', '-t', oid).strip() != b'blob':
            continue
        size = int(git(repo, 'cat-file', '-s', oid))
        sizes.append((size, name))
        if size > 50 * 1024 * 1024:
            raise ValueError('Large historical blob requires separate review: ' + name)
        if pattern.search(git(repo, 'cat-file', 'blob', oid)):
            raise ValueError('Possible historical credential; review locally: ' + name)
    return sizes


def manifest(directory):
    files = sorted(p for p in directory.rglob('*') if p.is_file() and p.name != 'SHA256SUMS')
    return ''.join(sha(p.read_bytes()) + '  ' + p.relative_to(directory).as_posix() + '\n' for p in files)


def create(repo, output, ref='HEAD', allow_dirty=False):
    repo = repo.resolve()
    output = output.absolute()
    if output.exists():
        raise ValueError('Export destination already exists')
    if output.is_relative_to(repo):
        raise ValueError('Export destination must be outside the source checkout')
    before = git(repo, 'status', '--porcelain=v1', '-uall')
    if before and not allow_dirty:
        raise ValueError('Dirty source refused; commit/preserve work or explicitly use --allow-dirty')
    commit = git(repo, 'rev-parse', '--verify', ref + '^{commit}').decode().strip()
    sizes = review_history(repo)
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.commerceops-export-', dir=output.parent) as tmp:
        stage = Path(tmp) / 'export'
        stage.mkdir()
        (stage / 'manifests').mkdir()
        state = {'source_commit': commit, 'source_branch': git(repo, 'branch', '--show-current').decode().strip(),
                 'dirty_source': bool(before), 'source_ref': ref,
                 'dirty_policy': 'archive is committed ref only; patches/untracked snapshot are separate'}
        (stage / 'manifests/GIT_STATE.json').write_text(json.dumps(state, indent=2) + '\n')
        (stage / 'manifests/WORKTREE_STATUS.txt').write_bytes(before)
        paths = git(repo, 'ls-tree', '-rz', '--name-only', commit).decode().split('\0')
        paths = [p for p in paths if p]
        if any(forbidden(p) for p in paths):
            raise ValueError('Selected source tree contains forbidden local data')
        (stage / 'manifests/TRACKED_FILES.txt').write_text('\n'.join(paths) + '\n')
        (stage / 'manifests/HISTORY_BLOB_SIZES.tsv').write_text(''.join(f'{size}\t{name}\n' for size, name in sorted(sizes, reverse=True)))
        (stage / 'manifests/SECRET_REVIEW.txt').write_text('Known forbidden paths and high-confidence credential markers checked across reachable history.\nThis check is not a guarantee that history contains no secrets; review before sharing.\n')
        migrations = [p for p in paths if p.startswith('services/api/migrations/')]
        (stage / 'manifests/MIGRATIONS.sha256').write_text(''.join(sha(git(repo, 'show', commit + ':' + p)) + '  ' + p + '\n' for p in migrations))
        if before:
            changed = git(repo, 'diff', '--name-only', '-z', 'HEAD').decode().split('\0')
            if any(forbidden(p) for p in changed if p):
                raise ValueError('Dirty changes contain forbidden local data')
            (stage / 'patches').mkdir()
            (stage / 'patches/working-tree.patch').write_bytes(git(repo, 'diff', '--binary', 'HEAD'))
            (stage / 'patches/index.patch').write_bytes(git(repo, 'diff', '--cached', '--binary'))
            with zipfile.ZipFile(stage / 'patches/untracked.zip', 'w', zipfile.ZIP_DEFLATED) as z:
                for p in git(repo, 'ls-files', '-z', '--others', '--exclude-standard').decode().split('\0'):
                    if not p:
                        continue
                    if forbidden(p) or (repo / p).is_symlink():
                        raise ValueError('Untracked path requires separate review: ' + p)
                    z.write(repo / p, p)
        git(repo, 'bundle', 'create', str(stage / 'commerceops-history.bundle'), '--all')
        # The selected commit must be recoverable from the full-history bundle.
        git(repo, 'archive', '--format=zip', '--output=' + str(stage / 'commerceops-source.zip'), commit)
        result = subprocess.run(['git', '-C', str(repo), 'bundle', 'verify', str(stage / 'commerceops-history.bundle')], capture_output=True, check=True)
        (stage / 'manifests/BUNDLE_VERIFY.txt').write_bytes(result.stdout + result.stderr)
        (stage / 'manifests/BUNDLE_HEADS.txt').write_bytes(git(repo, 'bundle', 'list-heads', str(stage / 'commerceops-history.bundle')))
        if git(repo, 'status', '--porcelain=v1', '-uall') != before:
            raise ValueError('Source status changed during export; retry from a stable snapshot')
        (stage / 'SHA256SUMS').write_text(manifest(stage))
        validate(stage)
        stage.rename(output)
    return commit


def validate(directory):
    directory = directory.resolve()
    entries = (directory / 'SHA256SUMS').read_text().splitlines()
    covered = set()
    for line in entries:
        expected, sep, name = line.partition('  ')
        relative = PurePosixPath(name)
        if not sep or not re.fullmatch('[0-9a-f]{64}', expected) or relative.is_absolute() or '..' in relative.parts or name in covered:
            raise ValueError('Invalid checksum manifest entry')
        p = directory / name
        if p.is_symlink() or not p.resolve().is_relative_to(directory) or not p.is_file() or sha(p.read_bytes()) != expected:
            raise ValueError('Checksum verification failed: ' + name)
        covered.add(name)
    required = {'commerceops-history.bundle', 'commerceops-source.zip', 'manifests/GIT_STATE.json', 'manifests/MIGRATIONS.sha256'}
    if not required.issubset(covered):
        raise ValueError('Required export files are missing from the checksum manifest')
    state = json.loads((directory / 'manifests/GIT_STATE.json').read_text())
    commit = state['source_commit']
    if not re.fullmatch('[0-9a-f]{40}', commit):
        raise ValueError('Invalid recorded source commit')
    with tempfile.TemporaryDirectory(prefix='commerceops-validate-') as tmp:
        clone = Path(tmp) / 'repo'
        subprocess.run(['git', 'clone', '--quiet', '--no-checkout', str(directory / 'commerceops-history.bundle'), str(clone)], capture_output=True, check=True)
        git(clone, 'checkout', '--quiet', '--detach', commit)
        if git(clone, 'rev-parse', 'HEAD').decode().strip() != commit:
            raise ValueError('Restored commit does not match')
        files = set(filter(None, git(clone, 'ls-tree', '-rz', '--name-only', commit).decode().split('\0')))
        with zipfile.ZipFile(directory / 'commerceops-source.zip') as z:
            names = [i.filename for i in z.infolist() if not i.is_dir()]
            if len(names) != len(set(names)) or set(names) != files:
                raise ValueError('Source archive file inventory does not match selected commit')
            for name in names:
                if z.read(name) != git(clone, 'show', commit + ':' + name):
                    raise ValueError('Source archive content differs: ' + name)
        migration_names = set()
        for line in (directory / 'manifests/MIGRATIONS.sha256').read_text().splitlines():
            expected, _, name = line.partition('  ')
            if name in migration_names or name not in files or sha(git(clone, 'show', commit + ':' + name)) != expected:
                raise ValueError('Migration manifest differs: ' + name)
            migration_names.add(name)
        if migration_names != {p for p in files if p.startswith('services/api/migrations/')} :
            raise ValueError('Migration manifest inventory differs')
    return commit


def clone_export(directory, destination, overlay=None):
    destination = destination.absolute()
    if destination.exists():
        raise ValueError('Clone destination already exists')
    commit = validate(directory)
    if overlay:
        for root in ['AGENTS.md', 'docs', 'scripts/export']:
            p = overlay / root
            if p.is_symlink() or (p.is_dir() and any(f.is_symlink() for f in p.rglob('*'))):
                raise ValueError('Overlay symlinks are not supported')
    destination.parent.mkdir(parents=True, exist_ok=True)
    # A failed clone stays visible for diagnosis; never overwrite or delete an existing tree.
    subprocess.run(['git', 'clone', '--quiet', '--no-checkout', str(directory.resolve() / 'commerceops-history.bundle'), str(destination)], capture_output=True, check=True)
    git(destination, 'switch', '-c', 'me/phase-15-repository-architecture', commit)
    git(destination, 'remote', 'remove', 'origin')
    if overlay:
        for root in ['AGENTS.md', 'docs', 'scripts/export']:
            p = overlay / root
            target = destination / root
            if p.is_dir():
                shutil.copytree(p, target, dirs_exist_ok=True)
            elif p.is_file():
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(p, target)
    return commit


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    p = commands.add_parser('create')
    p.add_argument('output', type=Path)
    p.add_argument('--repo', type=Path, default=Path.cwd())
    p.add_argument('--ref', default='HEAD')
    p.add_argument('--allow-dirty', action='store_true')
    p = commands.add_parser('validate')
    p.add_argument('directory', type=Path)
    p = commands.add_parser('clone')
    p.add_argument('directory', type=Path)
    p.add_argument('destination', type=Path)
    p.add_argument('--overlay', type=Path)
    args = parser.parse_args()
    try:
        if args.command == 'create':
            commit = create(args.repo, args.output, args.ref, args.allow_dirty)
        elif args.command == 'validate':
            commit = validate(args.directory)
        else:
            commit = clone_export(args.directory, args.destination, args.overlay)
    except (ValueError, OSError, subprocess.CalledProcessError, zipfile.BadZipFile, KeyError) as error:
        # Do not echo subprocess output which may include local sensitive content.
        parser.exit(1, f'Export operation failed: {error}\n')
    print('Verified source commit: ' + commit)


if __name__ == '__main__':
    main()
