# Go-Scopy Python 脚本开发指南

更新：2026-11-20。适用于独立「脚本」工作台。

> 脚本在**电脑本机的 Python 子进程**中运行，通过 ADB / uiautomator2 操作你明确选择的 Android 设备。它不是手机上的 Python，也不是安全沙箱。只运行你理解并信任的脚本。

## 1. 第一次使用：准备 Python

应用不附带 Python 解释器和第三方依赖，也不会在点击「运行」时自动下载或安装。

1. 安装 Python 并加入 PATH，或在工作台「运行设置」中选择已有解释器。优先使用你已有的、支持锁定依赖的环境。
2. 在**发行包根目录的电脑终端**中运行（不要在 Android Shell 中运行）：

   ```powershell
   python --version
   python -m pip install --require-hashes -r tools/python/requirements.lock
   python -c "import importlib.metadata as m; print(m.version('uiautomator2'))"
   ```

   当前运行器要求 **uiautomator2 == 3.7.1**。源码目录开发时，把依赖清单路径替换为 `resources/python/requirements.lock`。
3. 如果使用 `python3`、虚拟环境或指定路径，安装依赖和工作台运行必须使用**同一个解释器**：

   ```powershell
   # 示例：替换为你自己的解释器路径。
   & "C:\path\to\venv\Scripts\python.exe" -m pip install --require-hashes -r tools/python/requirements.lock
   ```

   不要把上述示例路径直接复制到运行设置；请选择真实存在的可执行文件。
4. 连接手机，开启 USB 调试并授权。工作台设备下拉框只显示已连接、已授权设备。无线设备须先在设备页连接。

解释器选择顺序：运行设置中的手动路径 > `GOSCRCPY_PYTHON` 环境变量 > PATH。Windows 默认寻找 `python`，其他平台默认 `python3`，找不到时尝试另一命令。旧版本遗留的便携 runtime 不会被自动选用。

## 2. 工作台怎么用

- 点击应用左侧 **脚本**，不必先打开某台设备的详情。
- 左侧 **新建**：输入 `.py` 文件名，可选择只读设备上下文、ADB 读取型号、截图或空白模板。
- 新脚本默认保存到应用配置目录的 `scripts` 文件夹；Windows 通常为 `%APPDATA%\Go-Scopy\scripts`。以工作台左下角显示的实际目录为准。
- **打开 .py**：通过系统文件选择器打开已有本地脚本，**原位保存，不会复制到默认文件夹**。外部脚本路径会保留在列表中。
- `Ctrl+S` 保存到真实的 UTF-8 本地文件；文件名后的「未保存」标记表示编辑尚未落盘。切换文件会提示保存、不保存或取消。
- 如果磁盘文件被其他编辑器修改，保存会报冲突而不是覆盖。请先复制备份当前编辑内容，再重新打开磁盘版本进行合并。
- 选择 **目标设备**，阅读并勾选「我信任此脚本」，点击 **运行**。有未保存修改时先保存，再执行；再次编辑会清除信任勾选。
- 下方 **运行输出** 显示 stdout、stderr、状态和退出码；执行记录可以切换。**停止** 会终止该次本机脚本进程树；**日志与产物** 打开该次运行目录。
- **开发指南 ↗** 将本指南导出到本机帮助目录，调用系统默认 Markdown 关联打开。未设置 `.md` 关联时，请在系统中选择 Markdown 阅读器、编辑器或记事本后重试；应用不安装额外阅读器。

> 当前同一台设备不能同时运行两份脚本。脚本运行期间切换应用页面不会自动停止它；退出应用会停止运行器管理的脚本。不要同时手动操作或让其他自动化工具控制同一台手机。

## 3. 编辑器与代码提示

编辑器使用 CodeMirror 6：Python 高亮、缩进、行号、代码折叠、撤销/重做、查找替换、内置函数及当前文件局部名称补全。输入时出现建议，`Ctrl+Space` 手动补全，`Ctrl+F` 查找，`Tab` 缩进，`Ctrl+S` 保存（macOS 使用 Command 对应快捷键）。

附加了 `goscrcpy_connect`、`goscrcpy_screenshot`、`u2_click_text` 代码片段，以及 `goscrcpy.` / `d.` 常用 API 提示。**标为「预置」的是静态模板**，默认 `d` 是你连接的设备变量，不是 Python 类型推断。当前不提供语言服务器、跨文件推断、完整成员枚举或安装包分析，也不包含 AI 助手。

## 4. 最小示例：确认目标设备（不启动 UI 自动化）

```python
import goscrcpy

print("所选设备：", goscrcpy.device_ids(), flush=True)
print("本次产物目录：", goscrcpy.output_path("example.txt").parent, flush=True)
```

`goscrcpy` 由运行器加入脚本的 PYTHONPATH，不需要从 PyPI 安装同名包。运行器即使执行普通脚本也会先校验锁定的 uiautomator2 依赖。

## 5. 只读 ADB 示例：获取设备型号

```python
import os
import subprocess
import goscrcpy

serial = goscrcpy.device_ids()[0]
adb = os.environ["GOSCRCPY_ADB_PATH"]
result = subprocess.run(
    [adb, "-s", serial, "shell", "getprop", "ro.product.model"],
    check=True,
    capture_output=True,
    text=True,
    encoding="utf-8",
)
print("设备：", serial)
print("型号：", result.stdout.strip())
```

这里明确传 `-s serial`，避免多设备时误操作。使用参数列表，不拼接 `shell=True` 命令字符串。普通只读 ADB 查询不需要调用 `goscrcpy.connect()`。

## 6. UI 自动化：连接、读取与截图

```python
import json
import goscrcpy

d = goscrcpy.connect()  # 只使用本次选择的设备，不自动选第一台。
print(json.dumps(d.info, ensure_ascii=False, indent=2), flush=True)

path = goscrcpy.output_path("screen.png")
d.screenshot(str(path))
print("截图已保存：", path, flush=True)
```

`connect()` 返回原生 `uiautomator2.Device`，不是另一套封装 API。首次连接可能初始化/部署设备端自动化服务；这不同于单纯读取 ADB 属性。uiautomator2 的服务不是 scrcpy 投屏服务，开启投屏不等于 UI 自动化环境已经准备好。

### 使用文本或 resourceId 选择控件

以下为**会操作手机的示例**，请替换目标应用和控件后再执行，不要盲目复制运行：

```python
import goscrcpy

d = goscrcpy.connect()
# d(text="设置").click(timeout=10)
# d(resourceId="com.example:id/input").set_text("示例内容")
# d.press("back")
```

优先使用稳定的 resourceId 或文本，尽量避免写死屏幕坐标。等待目标控件并设置有限超时，避免无限循环。

### 保存页面结构，辅助定位

```python
import goscrcpy

d = goscrcpy.connect()
path = goscrcpy.output_path("hierarchy.xml")
path.write_text(d.dump_hierarchy(), encoding="utf-8")
print(path)
```

页面结构、截图和日志可能包含个人信息；分享前请脱敏。

## 7. 日志、文件和执行上下文

```python
import json
import goscrcpy

result = {"devices": goscrcpy.device_ids(), "ok": True}
path = goscrcpy.output_path("results/report.json")
path.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
print("结果文件：", path, flush=True)
```

- `goscrcpy.device_ids()`：当前任务明确配置的设备序列号列表。工作台每次运行选择一台。
- `goscrcpy.connect()`：连接当前配置的唯一设备；零台或多台时不会猜测。
- `goscrcpy.output_path(name)`：返回本次执行目录内的 `pathlib.Path`，创建所需父目录；不允许通过 `../` 逃出产物目录。
- 脚本工作目录是脚本所在文件夹，方便导入同目录辅助模块；**不要把相对路径产物和每次运行产物目录混淆**。
- 运行器提供 `GOSCRCPY_DEVICE_ID`、`GOSCRCPY_DEVICE_IDS`、`ANDROID_SERIAL`、`GOSCRCPY_ADB_PATH`、`GOSCRCPY_RUN_ID`、`GOSCRCPY_RUN_DIR` 等环境变量。
- 每次执行单独保存 `output.log`、`run.json` 和脚本产物。实时界面保留有界日志尾部，磁盘日志有大小限制，避免大量输出拖垮界面；完整性要求高的数据应单独写入产物文件。
- `print(..., flush=True)` 适合进度信息。未捕获异常会写入输出，任务标为失败并显示非零退出码。
- 默认超时 600 秒，可设置 1–86400 秒；超时/停止会终止受管理的本机进程树，**不会回滚已经对设备或本机做过的操作**。

## 8. 常见问题

| 现象 | 检查方法 |
| --- | --- |
| PATH 中未找到 Python | 安装 Python 并加入 PATH，或在「运行设置」指定实际解释器路径。 |
| 未安装 uiautomator2 / 版本不符 | 用工作台选中的同一个 Python 安装包内 `requirements.lock`，要求 3.7.1。 |
| 终端能 import，工作台不能 | 比较解释器路径；系统 Python、虚拟环境和旧便携环境可能不是同一个。 |
| 找不到 goscrcpy | 从工作台/配套 CLI 启动；直接双击 .py 不会注入项目 SDK 和设备上下文。不要安装来路不明的同名包。 |
| 没有可选设备 | 回设备页连接，确认 USB 调试授权；离线或 unauthorized 不会出现在下拉框。 |
| 设备已有脚本运行 | 等待完成或切到对应执行记录停止，不能同设备并发。 |
| 点击「运行」不可用 | 检查是否打开了文件、选中了在线设备、勾选了信任，以及该设备是否忙。 |
| 保存提示外部修改 | 不会覆盖磁盘；复制备份当前编辑内容，重新打开后手动合并。 |
| 控件找不到 / 超时 | 检查前台页面、resourceId/文本、系统弹窗及设备自动化权限；不要只增加无限等待。 |
| .md 无法打开 | 设置操作系统 `.md` 默认应用；可选记事本或本地 Markdown 编辑器。 |

## 9. 在电脑终端执行（可选）

发行包根目录执行，替换为真实设备序列号和脚本路径：

```powershell
.\go-scopy-script.exe -serial 127.0.0.1:5555 -script "C:\your-scripts\example.py" -tools-root . -timeout 5m
```

源码环境：

```powershell
go run ./cmd/go-scopy-script -serial 127.0.0.1:5555 -script resources/python/examples/device_info.py -tools-root . -timeout 5m
```

这会直接执行脚本，不是远程控制桌面界面的 API。请先安装依赖并理解脚本动作。

## 10. 开发建议和边界

- 先在测试设备运行只读示例，再逐步增加实际操作；先日志、后动作。
- 不在脚本中硬编码密码、令牌和真实个人信息，不把日志/截图直接发到公共仓库。
- 不停止共享 ADB server，不把设备序列号省略后交给工具自行挑选。
- “信任脚本”不是沙箱；脚本可以使用当前用户的文件、网络和进程权限，必须人工审阅。
- 补全仅帮助输入，不保证 API 存在或脚本正确；以安装的锁定版本和真实设备结果为准。

参考资料：uiautomator2 官方仓库 `https://github.com/openatx/uiautomator2`（按当前项目锁定版本核对 API）；项目 SDK `resources/python/lib/goscrcpy/__init__.py`；运行机制说明 `docs/python-scripting.md`。
