"""Release orchestration checks without contacting GitHub or uploading files."""
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


release_plan = load('release_plan', 'release-plan.py')
publisher = load('publisher', 'publish-release.py')
VERSION = '0.1.12'
SOURCE = 'a'*40


class PublishingChecks(unittest.TestCase):
    def test_source_version_and_tag_are_locked_before_build(self):
        self.assertEqual(release_plan.plan(VERSION, SOURCE, True, VERSION, SOURCE)['publish'], 'true')
        self.assertEqual(release_plan.plan(VERSION, SOURCE, False, VERSION)['publish'], 'false')
        # New workflow scripts can rebuild the exact tagged app source without
        # moving that existing version tag or taking assets from an earlier run.
        self.assertEqual(release_plan.plan(VERSION, SOURCE, True, VERSION, SOURCE)['source'], SOURCE)
        for version, source, declared, tag in [('0.1.11', SOURCE, VERSION, SOURCE),
                                               (VERSION, 'short-sha', VERSION, SOURCE),
                                               (VERSION, SOURCE, VERSION, 'b'*40)]:
            with self.assertRaises(ValueError):
                release_plan.plan(version, source, True, declared, tag)

    def fixture_assets(self, root):
        sums = []
        for name in publisher.verify.targets(VERSION):
            data = ('native fixture '+name).encode()
            (root/name).write_bytes(data)
            digest = hashlib.sha256(data).hexdigest()
            (root/(name+'.sha256')).write_text(digest+'  '+name+'\n')
            sums.append(digest+'  '+name)
        (root/'SHA256SUMS').write_text('\n'.join(sums)+'\n')
        (root/'RELEASE_NOTES.md').write_text('Release notes')
        return publisher.assets_for_release(root, VERSION)

    def test_uploads_exactly_twenty_one_verified_assets(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            files = self.fixture_assets(root)
            self.assertEqual(len(files), 21)
            (root/next(iter(publisher.verify.targets(VERSION)))).write_bytes(b'changed after verification')
            with self.assertRaises(ValueError):
                publisher.assets_for_release(root, VERSION)

    def test_draft_upload_verify_then_publish_without_target(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            files = self.fixture_assets(root)
            assets = [dict(name=p.name, size=p.stat().st_size, state='uploaded') for p in files]
            commands = []
            def gh(*args, **kwargs):
                commands.append(args)
                if args[:2] == ('release', 'view'):
                    if args[-1] == 'isDraft':
                        return None
                    return json.dumps(dict(isDraft=args[-1] != 'assets,isDraft,url', assets=assets, url='https://example.test/release'))
                return ''
            with patch.object(publisher, 'gh', gh):
                publisher.publish(root, VERSION, files)
            self.assertEqual([c[1] for c in commands], ['view', 'create', 'upload', 'view', 'edit', 'view'])
            create = commands[1]
            self.assertIn('--verify-tag', create)
            self.assertIn('--draft', create)
            self.assertNotIn('--target', create)

    def test_failed_asset_verification_never_publishes(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            files = self.fixture_assets(root)
            commands = []
            def gh(*args, **kwargs):
                commands.append(args)
                if args[:2] == ('release', 'view'):
                    if args[-1] == 'isDraft':
                        return json.dumps(dict(isDraft=True))
                    return json.dumps(dict(isDraft=True, assets=[]))
                return ''
            with patch.object(publisher, 'gh', gh), self.assertRaises(ValueError):
                publisher.publish(root, VERSION, files)
            self.assertNotIn('edit', [c[1] for c in commands])

    def test_public_release_is_not_modified(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            files = self.fixture_assets(root)
            with patch.object(publisher, 'gh', return_value=json.dumps(dict(isDraft=False))) as gh:
                with self.assertRaises(ValueError):
                    publisher.publish(root, VERSION, files)
                self.assertEqual(gh.call_count, 1)


if __name__ == '__main__':
    unittest.main()
