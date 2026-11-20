"""Prepare a relocatable, platform-specific Python runtime at build time.

Requires uv on PATH (or --uv). Does not modify the system Python installation.
"""
import argparse
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--uv", default="uv")
    parser.add_argument("--version", default="3.12.13")
    args = parser.parse_args()
    base = ROOT / "resources" / "python"
    managed = base / "runtime" / "managed"
    managed.mkdir(parents=True, exist_ok=True)
    env = {**os.environ, "UV_PYTHON_INSTALL_DIR": str(managed)}
    uv = shutil.which(args.uv)
    if not uv:
        raise SystemExit("uv is required. Install uv first, or pass --uv PATH.")
    subprocess.run([uv, "python", "install", "--no-bin", "--no-registry", args.version], env=env, check=True)
    python = Path(subprocess.check_output(
        [uv, "python", "find", "--managed-python", "--no-project", args.version],
        env=env, text=True, encoding="utf-8").strip()).resolve()
    if managed.resolve() not in python.parents:
        raise SystemExit("Refusing to use an interpreter outside the managed runtime")
    packages = base / "runtime" / "packages"
    subprocess.run([uv, "pip", "install", "--python", str(python), "--target", str(packages),
                    "--require-hashes", "--only-binary", ":all:",
                    "-r", str(base / "requirements.lock")], env=env, check=True)
    check_env = {**env, "PYTHONPATH": os.pathsep.join([str(base / "lib"), str(packages)])}
    subprocess.run([str(python), "-c", "import goscrcpy,uiautomator2; import importlib.metadata as m; assert m.version('uiautomator2')=='3.7.1'; print('Script runtime ready')"], env=check_env, check=True)
    # Metadata includes every installed distribution's license and source.
    report = subprocess.check_output([str(python), "-c", "import importlib.metadata as m,json; print(json.dumps([{'name':d.metadata['Name'],'version':d.version,'license':d.metadata.get('License-Expression') or d.metadata.get('License'),'home':d.metadata.get('Home-page')} for d in m.distributions()],ensure_ascii=False,indent=2))"], env=check_env, text=True, encoding="utf-8")
    (base / "runtime" / "distributions.json").write_text(report, encoding="utf-8")
    target_platform = {"Windows": "windows", "Darwin": "darwin", "Linux": "linux"}[platform.system()]
    target_arch = {"amd64": "amd64", "x86_64": "amd64", "arm64": "arm64", "aarch64": "arm64"}[platform.machine().lower()]
    manifest = {"executable": python.relative_to(base.resolve()).as_posix(), "platform": target_platform,
                "arch": target_arch, "python": args.version, "uiautomator2": "3.7.1"}
    # Publish only after installation and the import check succeed.
    path = base / "runtime.json"
    temp = path.with_suffix(".json.tmp")
    temp.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    temp.replace(path)
    print(f"Prepared {target_platform}/{target_arch}: {path}")


if __name__ == "__main__":
    main()
