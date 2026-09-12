#!/usr/bin/env python3
"""Exercise recovery and refusal paths without using any operational data."""
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location('transfer', Path(__file__).with_name('export.py'))
transfer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(transfer)


class ExportTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='commerceops-export-test-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / 'source'
        self.repo.mkdir()
        self.git('init', '-q')
        self.git('config', 'user.name', 'Export fixture')
        self.git('config', 'user.email', 'fixture@example.test')
        (self.repo / '.gitignore').write_text('.env\nnode_modules/\n')
        (self.repo / 'README.md').write_text('original project README\n')
        migrations = self.repo / 'services/api/migrations'
        migrations.mkdir(parents=True)
        (migrations / '000001_fixture.up.sql').write_text('SELECT 1;\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'baseline')
        self.base = self.git('rev-parse', 'HEAD').decode().strip()
        self.output = self.root / 'export'

    def git(self, *args):
        return transfer.git(self.repo, *args)

    def test_exact_older_ref_and_overlay_preserve_readme(self):
        (self.repo / 'future.txt').write_text('later feature\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'later feature')
        transfer.create(self.repo, self.output, self.base)
        overlay = self.root / 'overlay'
        (overlay / 'docs').mkdir(parents=True)
        (overlay / 'docs/plan.md').write_text('planning\n')
        (overlay / 'README.md').write_text('must not replace project README\n')
        dest = self.root / 'clone'
        self.assertEqual(transfer.clone_export(self.output, dest, overlay), self.base)
        self.assertFalse((dest / 'future.txt').exists())
        self.assertEqual((dest / 'README.md').read_text(), 'original project README\n')
        self.assertTrue((dest / 'docs/plan.md').exists())

    def test_dirty_refused_and_explicit_snapshot_separate(self):
        (self.repo / 'README.md').write_text('changed\n')
        (self.repo / 'draft.txt').write_text('untracked\n')
        (self.repo / '.env').write_text('fixture local value, not for export\n')
        status = self.git('status', '--porcelain')
        with self.assertRaisesRegex(ValueError, 'Dirty source refused'):
            transfer.create(self.repo, self.output)
        self.assertFalse(self.output.exists())
        transfer.create(self.repo, self.output, allow_dirty=True)
        self.assertEqual(self.git('status', '--porcelain'), status)
        with zipfile.ZipFile(self.output / 'commerceops-source.zip') as z:
            self.assertEqual(z.read('README.md'), b'original project README\n')
            self.assertNotIn('.env', z.namelist())
        with zipfile.ZipFile(self.output / 'patches/untracked.zip') as z:
            self.assertEqual(z.namelist(), ['draft.txt'])

    def test_existing_export_and_clone_destinations_refused(self):
        transfer.create(self.repo, self.output)
        with self.assertRaisesRegex(ValueError, 'destination already exists'):
            transfer.create(self.repo, self.output)
        dest = self.root / 'occupied'
        dest.mkdir()
        (dest / 'keep').write_text('keep')
        with self.assertRaisesRegex(ValueError, 'destination already exists'):
            transfer.clone_export(self.output, dest)
        self.assertEqual((dest / 'keep').read_text(), 'keep')

    def test_checksum_corruption_rejected(self):
        transfer.create(self.repo, self.output)
        with (self.output / 'commerceops-source.zip').open('ab') as f:
            f.write(b'corruption')
        with self.assertRaisesRegex(ValueError, 'Checksum verification failed'):
            transfer.validate(self.output)

    def test_archive_mismatch_rejected_even_with_updated_checksum(self):
        transfer.create(self.repo, self.output)
        with zipfile.ZipFile(self.output / 'commerceops-source.zip', 'w') as z:
            z.writestr('different.txt', 'tampered')
        (self.output / 'SHA256SUMS').write_text(transfer.manifest(self.output))
        with self.assertRaisesRegex(ValueError, 'inventory does not match'):
            transfer.validate(self.output)

    def test_manifest_traversal_rejected(self):
        transfer.create(self.repo, self.output)
        with (self.output / 'SHA256SUMS').open('a') as f:
            f.write('0' * 64 + '  ../outside\n')
        with self.assertRaisesRegex(ValueError, 'Invalid checksum'):
            transfer.validate(self.output)

    def test_removed_secret_path_in_history_is_rejected(self):
        (self.repo / '.env').write_text('fixture\n')
        self.git('add', '-f', '.env')
        self.git('commit', '-qm', 'forbidden history fixture')
        self.git('rm', '-q', '.env')
        self.git('commit', '-qm', 'remove fixture')
        with self.assertRaisesRegex(ValueError, 'Forbidden historical path'):
            transfer.create(self.repo, self.output)
        self.assertFalse(self.output.exists())

    def test_missing_migration_manifest_entry_rejected(self):
        transfer.create(self.repo, self.output)
        (self.output / 'manifests/MIGRATIONS.sha256').write_text('')
        (self.output / 'SHA256SUMS').write_text(transfer.manifest(self.output))
        with self.assertRaisesRegex(ValueError, 'Migration manifest inventory differs'):
            transfer.validate(self.output)

    def test_new_forbidden_staged_path_rejected_in_dirty_mode(self):
        (self.repo / '.env').write_text('fixture local value')
        self.git('add', '-f', '.env')
        with self.assertRaisesRegex(ValueError, 'Dirty changes contain forbidden'):
            transfer.create(self.repo, self.output, allow_dirty=True)
        self.assertFalse(self.output.exists())

    def test_high_confidence_token_is_rejected_without_value_output(self):
        value = 'ghp_' + 'x' * 40
        (self.repo / 'config.txt').write_text(value)
        self.git('add', '.')
        self.git('commit', '-qm', 'credential fixture')
        with self.assertRaisesRegex(ValueError, 'Possible historical credential') as caught:
            transfer.create(self.repo, self.output)
        self.assertNotIn(value, str(caught.exception))


if __name__ == '__main__':
    unittest.main(verbosity=2)
