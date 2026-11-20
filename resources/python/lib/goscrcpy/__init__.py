"""Go-Scrcpy script context. Native uiautomator2 objects are returned unchanged."""
import json
import os
from pathlib import Path

__all__ = ["connect", "device_ids", "output_path"]


def device_ids():
    """Return explicitly configured devices; never silently select a device."""
    value = os.environ.get("GOSCRCPY_DEVICE_IDS", "")
    if value:
        ids = json.loads(value)
        if not isinstance(ids, list) or not all(isinstance(x, str) and x.strip() for x in ids):
            raise ValueError("GOSCRCPY_DEVICE_IDS must be a JSON array of device serials")
        return list(dict.fromkeys(ids))
    serial = os.environ.get("GOSCRCPY_DEVICE_ID") or os.environ.get("ANDROID_SERIAL")
    return [serial] if serial else []


def connect(serial=None):
    """Connect to an explicit device and return a native uiautomator2.Device."""
    import uiautomator2 as u2

    if not serial:
        ids = device_ids()
        if len(ids) != 1:
            raise ValueError("Specify a device serial when zero or multiple devices are configured")
        serial = ids[0]
    adb = os.environ.get("GOSCRCPY_ADB_PATH")
    if adb:
        os.environ["ADBUTILS_ADB_PATH"] = adb
    device = u2.connect(serial)
    device.settings["wait_timeout"] = 20
    return device


def output_path(name):
    """Create an artifact path inside this run's directory."""
    directory = os.environ.get("GOSCRCPY_RUN_DIR")
    if not directory:
        raise RuntimeError("GOSCRCPY_RUN_DIR is not configured")
    root = Path(directory).resolve()
    path = (root / name).resolve()
    if path == root or root not in path.parents:
        raise ValueError("Artifact path must remain inside the run directory")
    path.parent.mkdir(parents=True, exist_ok=True)
    return path
