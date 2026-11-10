# Go-Scopy

使用 **Go + 原生 Viva** 构建的 Android 投屏桌面工具。界面参考 GoIM，设备操作参考 escrcpy，投屏和鼠标键盘控制复用官方 scrcpy 独立窗口。

## 使用

完整解压 `Go-Scopy-0.1.0-windows-x64.zip`，运行其中的 `Go-Scopy.exe`。保留同目录的 `tools` 文件夹，无需单独安装 adb 或 scrcpy。

1. 在 Android 手机上开启开发者选项和 USB 调试，用数据线连接电脑，并在手机上允许 USB 调试。
2. 左侧选择设备，点击「开始投屏」；鼠标键盘直接在独立投屏窗口中控制手机。
3. 要录屏，先停止已有会话，再点击「投屏并录屏」。点击「停止会话」或关闭投屏窗口完成保存。
4. 「保存截图」「安装 APK」直接作用于选中的设备，执行结果在「任务」中查看。

无线连接：电脑与手机处于同一局域网，打开「无线连接」。Android 11 及以上可先输入手机配对弹窗的地址和六位配对码，再使用无线调试主页面的**连接地址**连接。配对端口与连接端口通常不同。旧设备可先通过 USB 连接，再点击「切换无线」，应用会启用 TCP 5555 并尝试连接手机局域网地址。

每台设备可同时运行一个投屏会话，多台设备可分别投屏。投屏参数在下一次启动时生效，可保存到当前设备或设为全局默认。截图／录像目录、工具路径和浅色／深色主题在「设置」中管理。更改工具路径后重新启动应用。

首版交付 Windows x64；不包含主界面内嵌画面、群控、文件管理、自动化或自动更新。

## 源码运行

需要 Go 1.26 或更新版本、PowerShell 7。当前验证工具链为 Go 1.27.3。

```powershell
# 下载并校验固定版本的官方工具（scrcpy 5.0.1）
pwsh -File scripts/fetch-tools.ps1

# 或直接复用本地 escrcpy 仓库的已固定资源（scrcpy 4.1）
pwsh -File scripts/fetch-tools.ps1 -EscrcpyRoot E:\01.Dev\github-back\escrcpy

pwsh -File scripts/start.ps1
```

离线导入按 `scripts/escrcpy-tools.lock.json` 逐文件检查 SHA-256，并复制同版本的 `scrcpy-server` 与所有必需 DLL；资源版本变化时需有意更新清单。官方下载模式支持 `-Archive <已下载的官方 ZIP>`，仍按 `scripts/tools.lock.json` 校验。应用运行期间不会下载工具。

直接使用 `go run` 时，需要指定工具根目录：

```powershell
go run ./cmd/go-scopy --tools-root .
```

工具查找顺序为设置中的可执行文件路径、`<工具根目录>/tools/scrcpy`、`<工具根目录>/tools`、PATH。发布程序的工具根目录默认为可执行文件所在目录。应用统一使用本机 adb 服务端口 5037，不会主动关闭共享 adb 服务。

配置默认保存在 `%APPDATA%\Go-Scopy\settings.json`；可用 `--data-dir <目录>` 指定隔离目录。配对码不会写入配置。截图和录像默认保存在用户目录的 `Videos\Go-Scopy` 下。

## 检查与打包

```powershell
go vet ./...
go test ./... -count=1

# 生成原生无头截图，截图使用合成设备元数据
$env:GOSCOPY_ARTIFACT_DIR = "$PWD/artifacts/screenshots"
go test ./internal/desktop -run TestLayoutsAndScreenshots -count=1
Remove-Item Env:GOSCOPY_ARTIFACT_DIR

# 无可见窗口的真实工具启动检查
pwsh -File scripts/smoke-headless.ps1

# 下载官方固定版本后打包，或加 -EscrcpyRoot 使用本地资源
pwsh -File scripts/build.ps1 -EscrcpyRoot E:\01.Dev\github-back\escrcpy
```

输出位于 `dist/Go-Scopy-0.1.0-windows-x64.zip`。包内包含工具来源清单、第三方许可和逐文件 `SHA256SUMS.txt`。

自动测试覆盖命令行参数边界、配置持久化、设备状态与监听协议、子进程错误、截图、APK 路径、界面交互与多尺寸布局。合成设备截图与子进程替身不代表真机已通过验证；当前具体检查结果和限制见 [实现记录](docs/implementation.md)。

## 常见问题

- **等待授权**：解锁手机并允许 USB 调试；弹窗未出现时重新插拔数据线，必要时撤销手机的 USB 调试授权后重新连接。
- **没有设备**：检查数据线是否支持数据传输、手机 USB 模式和厂商驱动，点击「刷新设备」。
- **无线连接失败**：核对连接端口，确认手机无线调试页面保持打开，且局域网允许设备互通。
- **录屏没有声音**：音频转发依赖手机系统和应用支持；不支持时关闭「传输音频」重试。
- **应用启动失败**：查看配置目录中的 `startup.log`；配置损坏时先备份配置文件再修正或移走，应用不会自动覆盖损坏配置。

## 开源项目

[GoIM](https://github.com/Ezreal-byte/GoIM) · [escrcpy](https://github.com/viarotel-org/escrcpy) · [Viva](https://github.com/vivaui/viva) · [scrcpy](https://github.com/Genymobile/scrcpy)

本项目代码采用 MIT 许可证。第三方代码和二进制遵循各自许可证，见 [第三方声明](THIRD_PARTY_NOTICES.md)。
