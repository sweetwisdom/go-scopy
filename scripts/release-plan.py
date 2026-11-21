"""Lock a fresh build to one version and source commit before native jobs start."""
import os
from pathlib import Path
import re
import subprocess


def plan(version, source, publish, declared_version, tag_commit=None):
    version = version.removeprefix('v')
    if not re.fullmatch(r'\d+\.\d+\.\d+', version) or version != declared_version:
        raise ValueError('Release version must match internal/buildinfo/version.go')
    if not re.fullmatch(r'[0-9a-f]{40}', source):
        raise ValueError('Release source must be a full commit SHA')
    if publish and tag_commit != source:
        raise ValueError('Published version tag must point to the exact source commit')
    return dict(version=version, source=source, publish=str(publish).lower())


def main():
    version = os.environ['PACKAGE_VERSION'].removeprefix('v')
    if not re.fullmatch(r'\d+\.\d+\.\d+', version):
        raise ValueError('Invalid release version')
    publish = os.environ.get('PUBLISH_RELEASE') == 'true'
    source = os.environ['SOURCE_COMMIT']
    declared = re.search(r'var Version = "([^"]+)"', Path('internal/buildinfo/version.go').read_text()).group(1)
    tag_commit = None
    if publish:
        tag_commit = subprocess.check_output(['git', 'rev-parse', 'v'+version+'^{commit}'], text=True).strip()
        # A dispatch may fix build orchestration without moving a published tag.
        # Always build the application's exact tagged source.
        source = tag_commit
    result = plan(version, source, publish, declared, tag_commit)
    with Path(os.environ['GITHUB_OUTPUT']).open('a', encoding='utf-8') as stream:
        for key, value in result.items():
            stream.write(key+'='+value+'\n')
    print(f'Fresh build: v{version}, source {source}, publish={publish}.')


if __name__ == '__main__':
    main()
