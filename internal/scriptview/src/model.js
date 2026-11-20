export const templates = {
  blank: '# -*- coding: utf-8 -*-\n',
  context: `import goscrcpy

# Use only the device explicitly selected in this workspace.
print("Selected devices:", goscrcpy.device_ids(), flush=True)
print("Output directory:", goscrcpy.output_path("example.txt").parent, flush=True)
`,
  model: `import os
import subprocess
import goscrcpy

# Read-only ADB query. Does not start UI automation services.
serial = goscrcpy.device_ids()[0]
adb = os.environ["GOSCRCPY_ADB_PATH"]
result = subprocess.run(
    [adb, "-s", serial, "shell", "getprop", "ro.product.model"],
    check=True, capture_output=True, text=True, encoding="utf-8",
)
print("Device:", serial, flush=True)
print("Model:", result.stdout.strip(), flush=True)
`,
  screenshot: `import goscrcpy

# uiautomator2 may initialize its on-device automation service.
d = goscrcpy.connect()
path = goscrcpy.output_path("screen.png")
d.screenshot(str(path))
print("Screenshot saved:", path, flush=True)
`,
}
export function activeRun(run) { return run && ['running', 'stopping'].includes(run.status) }
export function runLabel(status) { return ({ running: '运行中', stopping: '正在停止', success: '已完成', failed: '失败', timeout: '已超时', cancelled: '已取消', interrupted: '中断' })[status] || status }
export function canRun({ path, target, devices, trusted, busy, runs, active }) {
  return !!path && !!target && devices.some(d => d.serial === target) && trusted && !busy && active && !runs.some(r => activeRun(r) && r.request.serial === target)
}
