"""Validate original native-build provenance when recovering a release."""
import json
import os
from pathlib import Path
import re
import subprocess
import urllib.request


def validate_source(run, jobs, repository, version, tag_commit):
    if run.get('repository', {}).get('full_name') != repository:
        raise ValueError('Artifact source belongs to another repository')
    if run.get('path') != '.github/workflows/release.yml' or run.get('status') != 'completed':
        raise ValueError('Artifact source must be a completed native Release run')
    if run.get('event') != 'push' or run.get('head_branch') != 'v' + version or run.get('head_sha') != tag_commit:
        raise ValueError('Artifact source does not match the published version tag')
    builds = [job for job in jobs if job.get('name', '').startswith('build (')]
    expected = {
        'build (windows-2025, windows, amd64, x64)',
        'build (ubuntu-24.04, linux, amd64, x64)',
        'build (ubuntu-24.04-arm, linux, arm64, arm64)',
        'build (macos-15-intel, darwin, amd64, x64)',
        'build (macos-15, darwin, arm64, arm64)',
    }
    if len(builds) != 5 or {job['name'] for job in builds} != expected or any(job.get('conclusion') != 'success' for job in builds):
        raise ValueError('All five original native builds must have succeeded')
    return run['head_sha']


def main():
    run_id = os.environ.get('ARTIFACT_RUN_ID', '')
    if not run_id:
        print('Using native build artifacts from this workflow run.')
        return
    if not re.fullmatch(r'[1-9][0-9]*', run_id):
        raise ValueError('Invalid artifact run ID')
    version = os.environ['PACKAGE_VERSION'].removeprefix('v')
    if not re.fullmatch(r'\d+\.\d+\.\d+', version):
        raise ValueError('Invalid release version')
    repository = os.environ['GITHUB_REPOSITORY']
    commit = subprocess.check_output(['git', 'rev-parse', 'v' + version + '^{commit}'], text=True).strip()

    def api(path):
        request = urllib.request.Request('https://api.github.com/repos/' + repository + path,
                                         headers={'Authorization': 'Bearer ' + os.environ['GH_TOKEN'],
                                                  'Accept': 'application/vnd.github+json'})
        with urllib.request.urlopen(request, timeout=30) as response:
            return json.load(response)

    run = api('/actions/runs/' + run_id)
    jobs = api('/actions/runs/' + run_id + '/jobs?filter=latest&per_page=100')['jobs']
    source = validate_source(run, jobs, repository, version, commit)
    with Path(os.environ['GITHUB_ENV']).open('a', encoding='utf-8') as stream:
        stream.write('SOURCE_COMMIT=' + source + '\n')
    print(f'Validated five native builds from run {run_id}, source {source}.')


if __name__ == '__main__':
    main()
