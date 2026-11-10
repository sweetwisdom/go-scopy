# Go-Scopy

使用 **Go + 原生 Viva** 构建的 Android 投屏桌面工具。界面参考 GoIM，设备操作参考 escrcpy，投屏和鼠标键盘控制复用 scrcpy，Windows 支持原生内嵌窗口。

![alt text](./docs/.imgs/demo.png)

## 使用

从 [GitHub Releases](https://github.com/sweetwisdom/go-scopy/releases) 下载对应平台压缩包，完整解压。Windows 运行 `Go-Scopy.exe`，macOS 打开 `Go-Scopy.app`，Linux 运行 `./Go-Scopy`。保留附带工具，无需单独安装 adb 或 scrcpy。

1. 在 Android 手机上开启开发者选项和 USB 调试，用数据线连接电脑，并在手机上允许 USB 调试。
2. 左侧选择设备，点击「开始投屏」；Windows 下打开带工具栏的 Viva 投屏窗口，内嵌 scrcpy 画面，鼠标键盘直接控制手机。
3. 要录屏，先停止已有会话，再点击「投屏并录屏」。点击「停止会话」或关闭投屏窗口完成保存。
4. 「保存截图」「安装 APK」直接作用于选中的设备，执行结果在「任务」中查看。

无线连接：电脑与手机处于同一局域网，打开「无线连接」。Android 11 及以上可先输入手机配对弹窗的地址和六位配对码，再使用无线调试主页面的**连接地址**连接。配对端口与连接端口通常不同。旧设备可先通过 USB 连接，再点击「切换无线」，应用会启用 TCP 5555 并尝试连接手机局域网地址。

设备管理：在「无线连接」填写地址及可选备注，点击「保存无线设备」，离线时仍保留记录。选中设备点击「重命名」编辑本地备注，无线设备还可编辑地址；留空备注恢复原名称。备注按连接标识保存，USB 与 Wi-Fi 可分别命名。常用操作中可连接、断开、保存或删除无线记录；删除记录包含该地址的备注与设备参数，已连接设备仍由 ADB 自动发现，需要移出列表时先断开。编辑地址不会迁移正在运行的投屏或关闭旧连接。

USB 转无线会选择手机 Wi-Fi 接口地址并验证对应 ADB 通道。切换后选中 Wi-Fi 地址重新启动投屏，再拔掉 USB；正在运行的 USB 投屏不会自动迁移。

每台设备可同时运行一个投屏会话，多台设备可分别投屏。投屏参数在下一次启动时生效，可保存到当前设备或设为全局默认。截图／录像目录、工具路径和浅色／深色主题在「设置」中管理。更改工具路径后重新启动应用。

发布平台：Windows x64、macOS Intel/Apple Silicon、Linux x64/ARM64。Windows 使用内嵌投屏与悬浮工具栏，macOS/Linux 使用 scrcpy 独立窗口。

0.1.3 投屏窗口：画面按手机比例调整，右侧透明底板工具栏提供返回、桌面、最近任务、菜单、音量、截图、熄屏和唤醒。点击「收起 ›」缩成小按钮，点击「‹」展开；工具栏跟随窗口移动，右侧空间不足时浮在窗口右缘。关闭窗口先结束会话并保存录像。最大化时保留等比例画面，可能出现黑边。熄屏使用 Android 休眠按键，可能锁屏；菜单键的响应取决于手机前台应用。其他平台继续使用 scrcpy 独立窗口。0.1.2 的内嵌画面已经用户确认可用，新的悬浮栏与尺寸效果待用户体验。

## 源码运行

需要 Go 1.26 或更新版本；PowerShell 脚本需要 PowerShell 7。当前验证工具链为 Go 1.27.3。

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

工具查找顺序为设置中的可执行文件路径、`<工具根目录>/tools/scrcpy`、`<工具根目录>/tools`、macOS 应用包的 `Contents/Resources/tools/scrcpy`、PATH。发布程序的工具根目录默认为可执行文件所在目录。应用统一使用本机 adb 服务端口 5037，不会主动关闭共享 adb 服务。

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

# 使用仓库内已校验的资源打包，无需其他本地仓库
pwsh -File scripts/build.ps1 -Version 0.1.5
```

Windows 输出位于 `dist/Go-Scopy-0.1.5-windows-amd64.zip`。包内包含工具来源清单、第三方许可和逐文件 `SHA256SUMS.txt`。

0.1.4 修复投屏窗口生命周期：关闭投屏窗口或悬浮栏会结束对应会话，等待进程退出及录像保存后销毁两个窗口；手机断开或 scrcpy 自行退出也会自动销毁，错误和录屏结果仍在任务中心查看。

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

## 多平台发布（0.1.5）

GitHub Actions 的原生 runner 矩阵与归档实现复用 GoIM。推送 `v*` 标签，或手动执行 **Release** 并填写版本号，会构建五个平台；全部通过后自动发布三个 ZIP、两个 tar.gz 和 `SHA256SUMS`。

```sh
# 在目标平台原生打包；macOS 需要 Xcode Command Line Tools
# macOS 构建设置 CGO_ENABLED=1，CGO_CFLAGS/CGO_LDFLAGS=-mmacosx-version-min=13.0
go run ./cmd/package --version 0.1.5 --commit <source-commit>
```

工具完整复制自 escrcpy 的 `desktop/electron/resources/extra`，保存在 `resources/extra`。`resources/manifest.json` 固定来源提交与全部文件 SHA-256；构建会校验资源，再选择当前平台的 scrcpy 目录与共用 server。未使用的托盘、语言及其他工具资源不进入发布包。macOS 工具会重新进行 ad-hoc 签名，包内文件校验记录签名后的内容。

- Windows：64 位系统；应用和 scrcpy 为 x64，原版 adb/fastboot 为兼容运行的 x86。
- macOS：13.0+，分别提供 Intel 与 Apple Silicon 包；仅 ad-hoc 签名，未经 Apple 公证。首次打开可在系统设置「隐私与安全性」允许；手机授权与窗口交互仍需目标机器验收。
- Linux：需要图形桌面，以及 X11/XCB、xkbcommon、Vulkan、libudev 运行库。Ubuntu 24.04 可安装：`sudo apt-get install libx11-6 libx11-xcb1 libxcb1 libxkbcommon0 libxkbcommon-x11-0 libvulkan1 mesa-vulkan-drivers fonts-noto-cjk libudev1`。USB 设备访问需具备相应用户权限／udev 规则；Wayland 环境可能需要 XWayland。

CI 检查原生编译、工具架构兼容性、adb/scrcpy 版本运行、压缩包内容与 SHA-256，不代替各平台的手机实测。