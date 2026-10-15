#!/usr/bin/env python3
"""Offline failure gates, not runtime/host acceptance."""
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('publish', Path(__file__).with_name('publish-docker-preview-1.py'))
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)


class FailureGates(unittest.TestCase):
    def test_non_actions_context_cannot_mutate(self):
        with patch.dict(os.environ, {}, clear=True), patch.object(p, 'api') as api:
            with self.assertRaisesRegex(RuntimeError, 'Unauthorized'):
                p.promote_private(Path('/unused'))
            api.assert_not_called()

    def test_conflicting_tag_stops_before_registry_or_writes(self):
        context = {'GITHUB_ACTIONS': 'true', 'GITHUB_EVENT_NAME': 'workflow_dispatch',
                   'GITHUB_REPOSITORY': p.REPO, 'GITHUB_REF': 'refs/heads/main',
                   'GITHUB_ACTOR': 'FNB2026'}
        with patch.dict(os.environ, context, clear=True), patch.object(p, 'download_assets'), \
                patch.object(p, 'api', return_value={'object': {'type': 'commit', 'sha': 'wrong'}}) as api:
            with self.assertRaisesRegex(RuntimeError, 'conflicting tag'):
                p.promote_private(Path('/unused'))
            self.assertEqual(api.call_count, 1)
            self.assertEqual(api.call_args.args, ('repos/' + p.REPO + '/git/ref/tags/' + p.TAG,))

    def test_index_substitution_rejected(self):
        with self.assertRaisesRegex(RuntimeError, 'digest mismatch'):
            p.verify_index(json.dumps({'manifests': []}).encode())

    def test_extra_asset_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            folder = Path(temp)
            for name in p.PINS:
                (folder / name).touch()
            (folder / 'extra').touch()
            with self.assertRaisesRegex(RuntimeError, 'asset set mismatch'):
                p.verify_assets(folder)

    def test_corrupt_asset_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            folder = Path(temp)
            for name in p.PINS:
                (folder / name).write_bytes(b'corrupt')
            with self.assertRaisesRegex(RuntimeError, 'checksum mismatch'):
                p.verify_assets(folder)


if __name__ == '__main__':
    unittest.main()
