"""Verify all native release archives/installers and write release notes."""
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
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
            ('windows', 'amd64', ('.zip',)),
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
    executable += [resources + 'go-scopy-script' + suffix]
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


def verify_dmg(path, target, version, commit):
    report = json.loads(Path(str(path) + '.verification.json').read_text())
    require((report['os'], report['arch'], report['version'], report['commit']) ==
            (*target, version, commit), 'Wrong DMG native verification identity')
    require(report['artifact'] == path.name and report['sha256'] == digest(path), 'DMG native verification digest mismatch')
    require(report['verification'] == 'readonly-mount-payload-checksums-codesign', 'DMG native verification missing')


def release_notes(version, commit):
    return f"""Go-Scopy v{version}

### 发布修复
- 修复 Linux/macOS 测试助手对 Unix SIGINT 的处理，避免停止会话测试错过中间状态而阻止打包；生产停止逻辑不变。
- 发布流程拆分测试、静态检查和打包；支持仅验证，并校验版本标签与源提交一致。

### 设备概况与屏幕预览
- 设备连接、重连或进入设备概况时自动获取截图；概况页默认每 5 秒刷新，设置中支持 1–300 秒，0 关闭定时刷新。
- 离开概况页或最小化时暂停，慢请求不叠加；截图只保留在内存中，失败保留上一张。
- 右侧设备区域采用紧凑摘要与窄列全面屏预览，详细设备信息收进弹窗，移除底部操作行；左侧设备导航保持原有布局。
- 手机预览使用圆角窄边框，保存与刷新入口集中在截图区顶部。

### 应用缓存与设备管理
- 应用名称、图标按包名全局缓存，跨 USB、Wi-Fi 与不同设备共享；安装列表、路径及版本仍按设备隔离，设置中可清理缓存。
- 修复缓存命中后重复启动图标采集的问题，图标读取使用 Android Agent，避开不稳定的本地 APK 资源解析路径；增加读取大小限制和崩溃日志。
- 虚拟屏幕使用弹窗，复用应用包组件，支持名称与包名搜索；编辑设备、删除记录和投屏参数均通过弹窗操作。
- 无线连接图标紧跟设备名称并随状态变化；ADB 服务退出后重建监听并尝试恢复先前活动的无线连接，显式断开不会自动重连。
- 非 Windows 标题栏避让系统窗口控制；主屏使用品牌图标，独立应用及虚拟屏幕在启动时使用对应应用名称和图标。

### 下载与安装
- Windows x64：ZIP 绿色包，解压后运行 Go-Scopy.exe。
- macOS Intel / Apple Silicon：新增 DMG，拖动 Go-Scopy.app 到 Applications；也保留 ZIP。要求 macOS 13.0+，仅 ad-hoc 签名，未经 Apple 公证。
- Linux x64 / ARM64：新增 DEB（Ubuntu 24.04 / Debian 兼容环境），可用 sudo apt install ./Go-Scopy-*.deb 安装；也保留 tar.gz。
- 包含各平台的 adb、fastboot、scrcpy 4.1、Android server 和应用图标 Agent。Linux 需要图形桌面及包声明的运行库；便携包需自行安装依赖。
- Python 脚本使用系统 Python，不附带解释器。详见包内文档。

发布门槛包含五个平台的原生编译、工具版本及架构检查、九个主包的内容和 SHA256 校验。DMG 在 macOS 只读挂载并校验签名与文件，DEB 校验版本、架构、安装路径和权限；不代表已完成目标系统真机验收。

源码：{commit}
工具来源：viarotel-org/escrcpy @ 3a73e0d4de58a4674dddd61fc43956b4882c8bec
下载校验：SHA256SUMS
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
        else:
            verify_archive(path, target, version, commit)
        sums.append(f'{checksum}  {name}')
    (root / 'SHA256SUMS').write_text('\n'.join(sums) + '\n')
    (root / 'RELEASE_NOTES.md').write_text(release_notes(version, commit), encoding='utf-8')
    print('Verified nine native release packages.')


if __name__ == '__main__':
    main()
