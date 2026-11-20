"""Build native DMG/DEB installers from the already verified portable payload."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import struct
import tempfile


def run(*args, **kwargs):
    try:
        return subprocess.run([str(a) for a in args], check=True, **kwargs)
    except subprocess.CalledProcessError as error:
        if kwargs.get('capture_output'):
            print(error.stdout or '', error.stderr or '')
        raise


def digest(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def verify_payload(root):
    for line in (root / 'SHA256SUMS.txt').read_text().splitlines():
        expected, relative = line.split('  ', 1)
        path = root / relative
        if not path.resolve().is_relative_to(root.resolve()):
            raise ValueError(f'Invalid payload path: {relative}')
        if digest(path) != expected:
            raise ValueError(f'Payload checksum mismatch: {relative}')


def build_dmg(root, output, info):
    with tempfile.TemporaryDirectory(prefix='scopy-dmg-') as temporary:
        temporary = Path(temporary)
        source = temporary / 'source'
        shutil.copytree(root, source, symlinks=True)
        (source / 'Applications').symlink_to('/Applications', target_is_directory=True)
        run('hdiutil', 'create', '-volname', 'Go-Scopy', '-srcfolder', source,
            '-format', 'UDZO', output)
        mount = temporary / 'mounted'
        run('hdiutil', 'attach', '-readonly', '-nobrowse', '-mountpoint', mount, output)
        try:
            verify_payload(mount)
            if os.readlink(mount / 'Applications') != '/Applications':
                raise ValueError('Missing Applications installation shortcut')
            run('codesign', '--verify', '--deep', '--strict', mount / 'Go-Scopy.app')
            if json.loads((mount / 'build-info.json').read_text()) != info:
                raise ValueError('DMG build identity mismatch')
        finally:
            run('hdiutil', 'detach', mount)
    report = dict(info, artifact=output.name, sha256=digest(output),
                  verification='readonly-mount-payload-checksums-codesign')
    Path(str(output) + '.verification.json').write_text(json.dumps(report, indent=2) + '\n')


def deb_control(info, dependencies, installed_size):
    return (f'Package: go-scopy\nVersion: {info["version"]}\n'
            f'Architecture: {info["arch"]}\nMaintainer: Go-Scopy contributors\n'
            'Section: utils\nPriority: optional\n'
            f'Installed-Size: {installed_size}\nDepends: {dependencies}\n'
            'Recommends: fonts-noto-cjk, mesa-vulkan-drivers\n'
            'Homepage: https://github.com/sweetwisdom/go-scopy\n'
            'Description: Android device mirroring and management workspace\n'
            ' Includes adb, scrcpy and the Android agent.\n')


def build_deb(root, output, info):
    with tempfile.TemporaryDirectory(prefix='scopy-deb-') as temporary:
        temporary = Path(temporary)
        stage = temporary / 'package'
        payload = stage / 'opt' / 'go-scopy'
        shutil.copytree(root, payload, symlinks=True)
        control = stage / 'DEBIAN'
        control.mkdir()
        # dpkg-shlibdeps needs a source control file, even outside a source tree.
        (temporary / 'debian').mkdir()
        (temporary / 'debian' / 'control').write_text(
            'Source: go-scopy\nMaintainer: Go-Scopy contributors\n\n'
            'Package: go-scopy\nArchitecture: any\nDescription: Android workspace\n')
        binaries = [payload / 'Go-Scopy', payload / 'go-scopy-script']
        binaries += [payload / 'tools' / 'scrcpy' / name for name in ('adb', 'scrcpy', 'fastboot')]
        dynamic = []
        for binary in binaries:
            result = run('readelf', '-d', binary, capture_output=True, text=True)
            if '(NEEDED)' in result.stdout:
                dynamic.append('-e' + str(binary))
        dependencies = {'libx11-6', 'libx11-xcb1', 'libxcb1', 'libxkbcommon0',
                        'libxkbcommon-x11-0', 'libvulkan1', 'libudev1'}
        if dynamic:
            result = run('dpkg-shlibdeps', '-O', '--ignore-missing-info',
                         '-l' + str(payload / 'tools' / 'scrcpy'), *dynamic,
                         cwd=temporary, capture_output=True, text=True)
            line = next(line for line in result.stdout.splitlines() if line.startswith('shlibs:Depends='))
            dependencies.update(part.strip() for part in line.split('=', 1)[1].split(',') if part.strip())
        size = sum(p.stat().st_size for p in payload.rglob('*') if p.is_file())
        (control / 'control').write_text(deb_control(info, ', '.join(sorted(dependencies)), (size + 1023)//1024))
        bindir = stage / 'usr' / 'bin'
        bindir.mkdir(parents=True)
        (bindir / 'go-scopy').symlink_to('/opt/go-scopy/Go-Scopy')
        (bindir / 'go-scopy-script').symlink_to('/opt/go-scopy/go-scopy-script')
        apps = stage / 'usr' / 'share' / 'applications'
        apps.mkdir(parents=True)
        (apps / 'go-scopy.desktop').write_text(
            '[Desktop Entry]\nType=Application\nName=Go-Scopy\n'
            'Comment=Android device workspace\nExec=/opt/go-scopy/Go-Scopy\n'
            'Icon=go-scopy\nTerminal=false\nCategories=Utility;Development;\nStartupWMClass=GoScopy.Desktop\n')
        source_icon = Path(__file__).resolve().parents[1] / 'internal/brand/assets/icon.png'
        width, height = struct.unpack('>II', source_icon.read_bytes()[16:24])
        icon = stage / 'usr' / 'share' / 'icons' / 'hicolor' / f'{width}x{height}' / 'apps' / 'go-scopy.png'
        icon.parent.mkdir(parents=True)
        shutil.copyfile(source_icon, icon)
        run('dpkg-deb', '--root-owner-group', '--build', stage, output)
        extracted = temporary / 'extracted'
        run('dpkg-deb', '--extract', output, extracted)
        verify_payload(extracted / 'opt' / 'go-scopy')
        for field, expected in [('Version', info['version']), ('Architecture', info['arch'])]:
            result = run('dpkg-deb', '--field', output, field, capture_output=True, text=True)
            if result.stdout.strip() != expected:
                raise ValueError(f'DEB {field} mismatch')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    root, out = args.root.resolve(), args.out.resolve()
    info = json.loads((root / 'build-info.json').read_text())
    verify_payload(root)
    extension, builder = {'Darwin': ('.dmg', build_dmg), 'Linux': ('.deb', build_deb)}[platform.system()]
    output = out / (root.name + extension)
    builder(root, output, info)
    Path(str(output) + '.sha256').write_text(f'{digest(output)}  {output.name}\n')
    print(f'Verified native installer: {output}')


if __name__ == '__main__':
    main()
