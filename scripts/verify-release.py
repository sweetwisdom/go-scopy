"""Verify all native release archives/installers and write release notes."""
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def targets(version):
    return {
        f'Go-Scopy-{version}-{system}-{arch}{extension}': (system, arch)
        for system, arch, extensions in (
            ('windows', 'amd64', ('.zip', '-setup.exe')),
            ('darwin', 'amd64', ('.zip', '.dmg')),
            ('darwin', 'arm64', ('.zip', '.dmg')),
            ('linux', 'amd64', ('.tar.gz', '.deb')),
            ('linux', 'arm64', ('.tar.gz', '.deb')),
        ) for extension in extensions
    }


def verify_payload(entries, read, prefix, target, version, commit):
    info = json.loads(read(prefix + 'build-info.json'))
    require((info['os'], info['arch'], info['version'], info['commit']) ==
            (*target, version, commit), f'Build identity mismatch: {info}')
    lines = read(prefix + 'SHA256SUMS.txt').decode().splitlines()
    require(bool(lines), 'Empty payload checksum manifest')
    verified = set()
    for line in lines:
        checksum, relative = line.split('  ', 1)
        require(re.fullmatch(r'[0-9a-f]{64}', checksum), 'Invalid payload digest')
        path = PurePosixPath(relative)
        require(not path.is_absolute() and '..' not in path.parts and '\\' not in relative,
                f'Invalid payload path: {relative}')
        require(relative not in verified, f'Duplicate payload checksum: {relative}')
        require(hashlib.sha256(read(prefix + relative)).hexdigest() == checksum,
                f'Payload checksum mismatch: {relative}')
        verified.add(relative)
    system = target[0]
    resources = 'Go-Scopy.app/Contents/Resources/' if system == 'darwin' else ''
    tools = resources + 'tools/scrcpy/'
    suffix = '.exe' if system == 'windows' else ''
    app = {'windows': 'Go-Scopy.exe', 'linux': 'Go-Scopy',
           'darwin': 'Go-Scopy.app/Contents/MacOS/Go-Scopy'}[system]
    executable = [app] + [tools + name + suffix for name in ('adb', 'scrcpy', 'fastboot')]
    cli = resources + 'tools/go-scopy-script' if system == 'darwin' else 'go-scopy-script' + suffix
    executable += [cli]
    required = executable + [tools + name for name in ('scrcpy-server', 'source.json', 'go-scrcpy-agent.jar')]
    required += ['build-info.json']
    for relative in required:
        require(relative in verified, f'Unchecked required payload: {relative}')
    for relative in executable:
        require(prefix + relative in entries, f'Missing executable: {relative}')
        if system != 'windows':
            require(entries[prefix + relative] & 0o111, f'Executable bit missing: {relative}')
    require(any(n.startswith(prefix + 'licenses/') for n in entries), 'Missing licenses')
    return info


def tar_entries(archive):
    entries = {}
    members = {}
    for member in archive.getmembers():
        name = member.name.removeprefix('./')
        require(name not in entries, f'Duplicate tar member: {name}')
        entries[name] = member.mode
        members[name] = member

    def read(name):
        member = members[name]
        require(member.isfile(), f'Expected regular payload file: {name}')
        return archive.extractfile(member).read()
    return entries, members, read


def verify_archive(path, target, version, commit):
    if path.name.endswith('.zip'):
        with zipfile.ZipFile(path) as archive:
            require(archive.testzip() is None, f'Bad ZIP CRC: {path.name}')
            entries = {i.filename: i.external_attr >> 16 for i in archive.infolist()}
            require(len(entries) == len(archive.infolist()), 'Duplicate ZIP members')
            return verify_payload(entries, archive.read, path.name[:-4] + '/', target, version, commit)
    with tarfile.open(path, 'r:gz') as archive:
        entries, _, read = tar_entries(archive)
        return verify_payload(entries, read, path.name[:-7] + '/', target, version, commit)


def verify_deb(path, target, version, commit):
    def field(name):
        return subprocess.check_output(['dpkg-deb', '--field', str(path), name], text=True).strip()
    require(field('Package') == 'go-scopy', 'Wrong DEB package name')
    require(field('Version') == version and field('Architecture') == target[1], 'Wrong DEB version/architecture')
    require(bool(field('Depends')), 'DEB dependencies absent')
    with tempfile.TemporaryFile() as stream:
        subprocess.run(['dpkg-deb', '--fsys-tarfile', str(path)], stdout=stream, check=True)
        stream.seek(0)
        with tarfile.open(fileobj=stream, mode='r:') as archive:
            entries, members, read = tar_entries(archive)
            info = verify_payload(entries, read, 'opt/go-scopy/', target, version, commit)
            for name, destination in [('usr/bin/go-scopy', '/opt/go-scopy/Go-Scopy'),
                                      ('usr/bin/go-scopy-script', '/opt/go-scopy/go-scopy-script')]:
                require(name in members and members[name].issym() and members[name].linkname == destination,
                        f'Invalid launcher: {name}')
            desktop = read('usr/share/applications/go-scopy.desktop').decode()
            require('Exec=/opt/go-scopy/Go-Scopy' in desktop and 'Icon=go-scopy' in desktop, 'Invalid desktop entry')
            require(any(n.startswith('usr/share/icons/hicolor/') and n.endswith('/apps/go-scopy.png') for n in entries), 'Missing DEB icon')
            require(all(member.uid == 0 and member.gid == 0 for member in members.values()), 'Non-root DEB payload ownership')
            require(not any(n.startswith(('home/', 'root/')) for n in entries), 'User data in DEB')
            return info


def verify_windows_installer(path, target, version, commit):
    sevenzip = shutil.which('7zz') or shutil.which('7z')
    require(sevenzip is not None, '7-Zip is required to verify the NSIS installer')
    result = subprocess.run([sevenzip, 't', str(path)], check=True, capture_output=True, text=True)
    require('type = nsis' in result.stdout.lower(), 'Windows setup must be an NSIS installer')
    with tempfile.TemporaryDirectory(prefix='scopy-setup-check-') as temporary:
        root = Path(temporary)
        # Extract only: never execute the installer or install on the runner.
        subprocess.run([sevenzip, 'x', str(path), '-o'+str(root), '-y'], check=True, capture_output=True)
        entries = {p.relative_to(root).as_posix(): p.stat().st_mode for p in root.rglob('*') if p.is_file()}
        def read(name):
            return (root/name).read_bytes()
        return verify_payload(entries, read, '', target, version, commit)


def verify_dmg(path, target, version, commit):
    report = json.loads(Path(str(path) + '.verification.json').read_text())
    require((report['os'], report['arch'], report['version'], report['commit']) ==
            (*target, version, commit), 'Wrong DMG native verification identity')
    require(report['artifact'] == path.name and report['sha256'] == digest(path), 'DMG native verification digest mismatch')
    require(report['verification'] == 'readonly-mount-payload-checksums-codesign', 'DMG native verification missing')


def release_notes(version, commit):
    return f"""Go-Scopy v{version}

- 设备概况自动截图，默认 5 秒刷新，设置可调整；手机预览采用全面屏窄边框。
- 应用名称、图标按包名全局缓存，修复重复加载和图标读取的闪退风险，新增崩溃日志。
- 优化虚拟屏幕、设备管理弹窗及连接恢复。
- Windows 提供 EXE 安装包和 ZIP；macOS 提供 DMG / ZIP；Linux 提供 DEB / tar.gz，均附带 adb 与 scrcpy。

macOS 13.0+，未经 Apple 公证。文件校验见 SHA256SUMS。
源码：{commit}
"""


def main():
    root = Path('release')
    version = os.environ['PACKAGE_VERSION'].removeprefix('v')
    commit = os.environ['SOURCE_COMMIT']
    require(re.fullmatch(r'\d+\.\d+\.\d+', version), f'Invalid version: {version}')
    expected = targets(version)
    reports = {name + '.verification.json' for name in expected if name.endswith('.dmg')}
    checksums = {name + '.sha256' for name in expected}
    allowed = set(expected) | reports | checksums | {'SHA256SUMS', 'RELEASE_NOTES.md'}
    found = {p.name for p in root.iterdir()}
    require(found <= allowed and set(expected) | reports | checksums <= found, f'Unexpected/missing release files: {found ^ allowed}')
    sums = []
    for name, target in sorted(expected.items()):
        path = root / name
        checksum = digest(path)
        require((root / (name + '.sha256')).read_text().split() == [checksum, name], f'Checksum mismatch: {name}')
        if name.endswith('.dmg'):
            verify_dmg(path, target, version, commit)
        elif name.endswith('.deb'):
            verify_deb(path, target, version, commit)
        elif name.endswith('-setup.exe'):
            verify_windows_installer(path, target, version, commit)
        else:
            verify_archive(path, target, version, commit)
        sums.append(f'{checksum}  {name}')
    (root / 'SHA256SUMS').write_text('\n'.join(sums) + '\n')
    (root / 'RELEASE_NOTES.md').write_text(release_notes(version, commit), encoding='utf-8')
    print(f'Verified {len(expected)} native release packages.')


if __name__ == '__main__':
    main()
