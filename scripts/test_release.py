"""Synthetic release checks; no device commands or application launches."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import zipfile


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


verify = load('verify', 'verify-release.py')
installers = load('installers', 'package-installers.py')
VERSION = '0.1.10'
COMMIT = 'a'*40


def payload(system, arch):
    resources = 'Go-Scopy.app/Contents/Resources/' if system == 'darwin' else ''
    app = {'windows': 'Go-Scopy.exe', 'darwin': 'Go-Scopy.app/Contents/MacOS/Go-Scopy', 'linux': 'Go-Scopy'}[system]
    suffix = '.exe' if system == 'windows' else ''
    # Model the packager's platform layouts, not the verifier's assumptions:
    # macOS keeps the CLI beside scrcpy under Contents/Resources/tools.
    cli = 'Go-Scopy.app/Contents/Resources/tools/go-scopy-script' if system == 'darwin' else 'go-scopy-script' + suffix
    files = {app: b'app', cli: b'cli', 'licenses/MIT.txt': b'MIT'}
    for name in ('adb'+suffix, 'scrcpy'+suffix, 'fastboot'+suffix, 'scrcpy-server', 'source.json', 'go-scrcpy-agent.jar'):
        files[resources + 'tools/scrcpy/' + name] = b'tool'
    files['build-info.json'] = json.dumps(dict(os=system, arch=arch, version=VERSION, commit=COMMIT)).encode()
    files['SHA256SUMS.txt'] = ''.join(f'{hashlib.sha256(data).hexdigest()}  {name}\n' for name, data in files.items()).encode()
    return files


def write_tar(files, prefix, stream):
    with tarfile.open(fileobj=stream, mode='w') as archive:
        for name, data in files.items():
            entry = tarfile.TarInfo(prefix + name)
            entry.size, entry.mode = len(data), 0o755
            archive.addfile(entry, io.BytesIO(data))


class ReleaseChecks(unittest.TestCase):
    def test_windows_installer_checks_extracted_payload_without_running_setup(self):
        files = payload('windows', 'amd64')
        commands = []
        def sevenzip(args, **kwargs):
            commands.append(args)
            if args[1] == 'x':
                root = Path(next(arg[2:] for arg in args if arg.startswith('-o')))
                for name, data in files.items():
                    path = root/name
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_bytes(data)
            return subprocess.CompletedProcess(args, 0, stdout='Type = Nsis\nEverything is Ok\n')
        with patch.object(verify.shutil, 'which', return_value='7z'), patch.object(verify.subprocess, 'run', sevenzip):
            verify.verify_windows_installer(Path('setup.exe'), ('windows', 'amd64'), VERSION, COMMIT)
            with self.assertRaises(ValueError):
                verify.verify_windows_installer(Path('setup.exe'), ('windows', 'amd64'), VERSION, 'wrong-commit')
        self.assertTrue(all(command[0] == '7z' for command in commands))

    def test_macos_cli_is_checked_in_tools_and_cannot_be_omitted(self):
        files = payload('darwin', 'amd64')
        entries = {name: 0o755 for name in files}
        verify.verify_payload(entries, files.__getitem__, '', ('darwin', 'amd64'), VERSION, COMMIT)
        wrong = 'Go-Scopy.app/Contents/Resources/go-scopy-script'
        actual = 'Go-Scopy.app/Contents/Resources/tools/go-scopy-script'
        files[wrong] = files.pop(actual)
        # Give the misplaced file a valid checksum: placement must still fail.
        files['SHA256SUMS.txt'] = ''.join(f'{hashlib.sha256(data).hexdigest()}  {name}\n'
                                        for name, data in files.items() if name != 'SHA256SUMS.txt').encode()
        entries = {name: 0o755 for name in files}
        with self.assertRaisesRegex(ValueError, 'Unchecked required payload: .*tools/go-scopy-script'):
            verify.verify_payload(entries, files.__getitem__, '', ('darwin', 'amd64'), VERSION, COMMIT)

    def test_ten_targets_and_archive_contents(self):
        self.assertEqual(len(verify.targets(VERSION)), 10)
        with tempfile.TemporaryDirectory() as temporary:
            for system, arch in [('windows', 'amd64'), ('darwin', 'arm64'), ('linux', 'amd64')]:
                files = payload(system, arch)
                extension = '.tar.gz' if system == 'linux' else '.zip'
                name = f'Go-Scopy-{VERSION}-{system}-{arch}'
                path = Path(temporary) / (name + extension)
                if extension == '.zip':
                    with zipfile.ZipFile(path, 'w') as archive:
                        for relative, data in files.items():
                            entry = zipfile.ZipInfo(name + '/' + relative)
                            entry.external_attr = 0o100755 << 16
                            archive.writestr(entry, data)
                else:
                    with tarfile.open(path, 'w:gz') as archive:
                        for relative, data in files.items():
                            entry = tarfile.TarInfo(name + '/' + relative)
                            entry.size, entry.mode = len(data), 0o755
                            archive.addfile(entry, io.BytesIO(data))
                verify.verify_archive(path, (system, arch), VERSION, COMMIT)
                with self.assertRaises(ValueError):
                    verify.verify_archive(path, (system, arch), VERSION, 'wrong-commit')

    def test_payload_rejects_corruption_permissions_and_traversal(self):
        files = payload('linux', 'arm64')
        entries = {name: 0o755 for name in files}
        verify.verify_payload(entries, files.__getitem__, '', ('linux', 'arm64'), VERSION, COMMIT)
        entries['Go-Scopy'] = 0o644
        with self.assertRaises(ValueError):
            verify.verify_payload(entries, files.__getitem__, '', ('linux', 'arm64'), VERSION, COMMIT)
        entries['Go-Scopy'] = 0o755
        files['Go-Scopy'] = b'corrupted'
        with self.assertRaises(ValueError):
            verify.verify_payload(entries, files.__getitem__, '', ('linux', 'arm64'), VERSION, COMMIT)
        files['SHA256SUMS.txt'] = (('a'*64) + '  ../outside\n').encode()
        with self.assertRaises(ValueError):
            verify.verify_payload(entries, files.__getitem__, '', ('linux', 'arm64'), VERSION, COMMIT)

    def test_dmg_report_is_bound_to_artifact_and_commit(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'app.dmg'
            path.write_bytes(b'disk image')
            report = dict(os='darwin', arch='arm64', version=VERSION, commit=COMMIT,
                          artifact=path.name, sha256=verify.digest(path),
                          verification='readonly-mount-payload-checksums-codesign')
            Path(str(path)+'.verification.json').write_text(json.dumps(report))
            verify.verify_dmg(path, ('darwin', 'arm64'), VERSION, COMMIT)
            path.write_bytes(b'tampered')
            with self.assertRaises(ValueError):
                verify.verify_dmg(path, ('darwin', 'arm64'), VERSION, COMMIT)

    def test_deb_installer_without_installing(self):
        if not shutil.which('dpkg-deb'):
            self.skipTest('native Linux DEB check runs on Linux runners')
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / 'payload'
            for relative, data in payload('linux', 'amd64').items():
                path = root / relative
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(data)
                path.chmod(0o755)
            output = Path(temporary) / 'test.deb'
            # Only ELF dependency probing is simulated for the synthetic files;
            # the real dpkg-deb builder, extractor and release verifier run.
            native_run = installers.run
            def fixture_run(*args, **kwargs):
                if args[0] == 'readelf':
                    return subprocess.CompletedProcess(args, 0, stdout='')
                return native_run(*args, **kwargs)
            with patch.object(installers, 'run', fixture_run):
                installers.build_deb(root, output, json.loads((root/'build-info.json').read_text()))
            verify.verify_deb(output, ('linux', 'amd64'), VERSION, COMMIT)

    def test_deb_metadata_and_notes(self):
        control = installers.deb_control(dict(version=VERSION, arch='arm64'), 'libc6 (>= 2.39)', 100)
        self.assertIn('Architecture: arm64\n', control)
        self.assertIn('Depends: libc6 (>= 2.39)', control)
        notes = verify.release_notes(VERSION, COMMIT)
        for feature in ['右键菜单', '多选上传', '紧凑卡片', '安装时间', '滚动条',
                        '平板横屏', '每台设备', '手机输入法', '打开目录',
                        "What's new", 'DMG', 'DEB', VERSION, COMMIT]:
            self.assertIn(feature, notes)


if __name__ == '__main__':
    unittest.main()
