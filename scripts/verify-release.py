"""Verify native packages before publishing a complete release."""
import hashlib
import json
import os
from pathlib import Path
import re
import tarfile
import zipfile

root = Path('release')
version = os.environ['PACKAGE_VERSION'].removeprefix('v')
assert re.fullmatch(r'\d+\.\d+\.\d+', version), version
expected = {
    f'Go-Scopy-{version}-{system}-{arch}{ext}': (system, arch)
    for system, arch, ext in (
        ('windows', 'amd64', '.zip'),
        ('darwin', 'amd64', '.zip'), ('darwin', 'arm64', '.zip'),
        ('linux', 'amd64', '.tar.gz'), ('linux', 'arm64', '.tar.gz'),
    )
}
found = {p.name for p in root.iterdir() if not p.name.endswith('.sha256')}
assert found == expected.keys(), (found, expected.keys())
sums = []
for name, target in sorted(expected.items()):
    path = root / name
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    assert (root / (name + '.sha256')).read_text().split() == [digest, name], name
    sums.append(f'{digest}  {name}')
    if name.endswith('.zip'):
        with zipfile.ZipFile(path) as archive:
            entries = {i.filename: (i.external_attr >> 16) for i in archive.infolist()}
            read = archive.read
            info = json.loads(read(next(n for n in entries if n.endswith('/build-info.json'))))
            checksums = read(next(n for n in entries if n.endswith('/SHA256SUMS.txt'))).decode()
            for line in checksums.splitlines():
                checksum, relative = line.split('  ', 1)
                assert hashlib.sha256(read(name.removesuffix('.zip') + '/' + relative)).hexdigest() == checksum, relative
    else:
        with tarfile.open(path) as archive:
            entries = {i.name: i.mode for i in archive.getmembers()}
            info = json.load(archive.extractfile(next(n for n in entries if n.endswith('/build-info.json'))))
            checksums = archive.extractfile(next(n for n in entries if n.endswith('/SHA256SUMS.txt'))).read().decode()
            for line in checksums.splitlines():
                checksum, relative = line.split('  ', 1)
                content = archive.extractfile(name.removesuffix('.tar.gz') + '/' + relative).read()
                assert hashlib.sha256(content).hexdigest() == checksum, relative
    assert (info['os'], info['arch']) == target and info['version'] == version, info
    assert info['commit'] == os.environ['SOURCE_COMMIT'], info
    system = target[0]
    prefix = name.removesuffix('.zip').removesuffix('.tar.gz') + '/'
    tool_dir = prefix + ('Go-Scopy.app/Contents/Resources/' if system == 'darwin' else '') + 'tools/scrcpy/'
    suffix = '.exe' if system == 'windows' else ''
    app = prefix + {'windows': 'Go-Scopy.exe', 'linux': 'Go-Scopy', 'darwin': 'Go-Scopy.app/Contents/MacOS/Go-Scopy'}[system]
    assert app in entries, app
    for entry in [app] + [tool_dir + t + suffix for t in ('adb', 'scrcpy', 'fastboot')]:
        assert entry in entries, entry
        if system != 'windows':
            assert entries[entry] & 0o111, entry
    assert tool_dir + 'scrcpy-server' in entries
    assert tool_dir + 'source.json' in entries
    assert any('/licenses/' in n for n in entries)
(root / 'SHA256SUMS').write_text('\n'.join(sums) + '\n')
notes = f'''Go-Scopy v{version}

- Windows x64、macOS Intel / Apple Silicon、Linux x64 / ARM64 原生构建。
- 包含对应平台的 adb、scrcpy 4.1 和 Android server，解压即可使用。
- Windows 支持内嵌投屏和透明可收起工具栏；macOS/Linux 使用 scrcpy 独立窗口。
- macOS 要求 13.0+；应用为 ad-hoc 签名，未经 Apple 公证。
- Linux 需要图形桌面及 X11/XCB、xkbcommon、Vulkan、libudev 运行库，详见 README。

五个平台均通过原生编译、工具版本运行检查、架构与包内容校验；真机操作需在目标系统验收。

源码：{os.environ['SOURCE_COMMIT']}
工具来源：viarotel-org/escrcpy @ 3a73e0d4de58a4674dddd61fc43956b4882c8bec
下载校验：SHA256SUMS
'''
(root / 'RELEASE_NOTES.md').write_text(notes, encoding='utf-8')
print('Verified all five release packages.')
