"""Publish only this run's verified assets, using an existing immutable tag."""
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess


spec = importlib.util.spec_from_file_location('verify_release', Path(__file__).with_name('verify-release.py'))
verify = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verify)


def gh(*args, allow_missing=False):
    result = subprocess.run(['gh', *args], capture_output=True, text=True)
    if result.returncode:
        if allow_missing and 'release not found' in result.stderr.lower():
            return None
        raise RuntimeError(result.stderr.strip() or 'GitHub CLI command failed')
    return result.stdout


def assets_for_release(root, version):
    expected = set(verify.targets(version))
    checksums = {}
    for line in (root/'SHA256SUMS').read_text().splitlines():
        checksum, name = line.split('  ', 1)
        verify.require(name in expected and name not in checksums, 'Unexpected/duplicate release checksum entry')
        checksums[name] = checksum
    verify.require(set(checksums) == expected, 'Release must include all expected packages')
    files = []
    for name in sorted(expected):
        path = root/name
        verify.require(verify.digest(path) == checksums[name], 'Verified artifact changed: '+name)
        sidecar = root/(name+'.sha256')
        verify.require(sidecar.read_text().split() == [checksums[name], name], 'Sidecar mismatch: '+name)
        files.extend([path, sidecar])
    return files+[root/'SHA256SUMS']


def validate_uploaded(state, files):
    expected = {path.name: path.stat().st_size for path in files}
    uploaded = state.get('assets', [])
    verify.require(len(uploaded) == len(expected), 'Draft release asset count mismatch')
    for asset in uploaded:
        verify.require(asset['name'] in expected and asset['size'] == expected[asset['name']]
                       and asset['state'] == 'uploaded', 'Incomplete/unexpected draft asset')
    verify.require({asset['name'] for asset in uploaded} == set(expected), 'Missing draft release assets')


def publish(root, version, files):
    tag = 'v'+version
    existing = gh('release', 'view', tag, '--json', 'isDraft', allow_missing=True)
    if existing is None:
        # The tag was checked against SOURCE_COMMIT. Do not send target_commitish:
        # release creation must use the existing tag, not request tag creation.
        gh('release', 'create', tag, '--verify-tag', '--draft', '--title', 'Go-Scopy '+tag,
           '--notes-file', str(root/'RELEASE_NOTES.md'))
    else:
        verify.require(json.loads(existing)['isDraft'], 'Release is already public; assets will not be replaced')
    gh('release', 'upload', tag, *[str(path) for path in files], '--clobber')
    draft = json.loads(gh('release', 'view', tag, '--json', 'assets,isDraft'))
    verify.require(draft['isDraft'], 'Release changed state during upload')
    validate_uploaded(draft, files)
    gh('release', 'edit', tag, '--draft=false', '--latest')
    public = json.loads(gh('release', 'view', tag, '--json', 'assets,isDraft,url'))
    verify.require(not public['isDraft'], 'Release is still a draft')
    validate_uploaded(public, files)
    print('Published complete release:', public['url'])


def main():
    version = os.environ['PACKAGE_VERSION'].removeprefix('v')
    verify.require(re.fullmatch(r'\d+\.\d+\.\d+', version), 'Invalid release version')
    tag_commit = subprocess.check_output(['git', 'rev-parse', 'v'+version+'^{commit}'], text=True).strip()
    verify.require(tag_commit == os.environ['SOURCE_COMMIT'], 'Tag/source mismatch before publishing')
    root = Path('release')
    files = assets_for_release(root, version)
    publish(root, version, files)


if __name__ == '__main__':
    main()
