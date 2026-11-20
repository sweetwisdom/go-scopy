"""Read device information and capture the current screen without clicking."""
import json
import goscrcpy

device = goscrcpy.connect()
print(json.dumps(device.info, ensure_ascii=False, indent=2))
path = goscrcpy.output_path("screen.png")
device.screenshot(str(path))
print(f"Screenshot saved: {path}")
