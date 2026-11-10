# Go-Scopy 实现记录

## 应用图标（2026-11-10）

新增钴蓝色圆角底、白色桌面显示器与薄荷青手机交叠的应用图标，手机内的右上箭头表达投屏连接。可编辑源文件为 `internal/brand/assets/icon.svg`，通过 `go run ./cmd/assets` 使用现有 Viva SVG 编译器和无头渲染器导出透明 PNG（16–1024 px）及多尺寸 ICO，不新增依赖。Viva 无头截图强制不透明，因此导出时通过白色轮廓遮罩恢复抗锯齿透明通道。

同一图标用于标题栏、左侧导航品牌位、原生窗口／任务栏及 `viva.toml`。Windows 打包使用已有的 Viva `winres` 为 EXE 写入图标与版本资源，包内同时提供 `Go-Scopy.ico`。这会改变下文首次交付的文件数量与体积记录，以当前包内哈希清单为准。

## 背景与确认范围

2026-11-10：用户确认开发 Go + Viva 的 escrcpy 类工具，参考本地 GoIM 的原生 UI 风格。首版为 Windows 优先的核心可用版，使用官方 scrcpy 独立投屏窗口，便携包附带 adb／scrcpy。后续用户补充本地 escrcpy 仓库，可直接复用必要资源。

## 关键决策

- Go 模块固定 Viva v0.2.3；不依赖 GoIM 的缺失同级 Viva checkout，不修改任何参考仓库。
- 应用只保留 Go 原生 UI、业务状态和命令适配。三栏尺寸与配色参考 GoIM，未迁入其聊天、存储、发现协议等业务。
- escrcpy 的工具定位、独立镜像和录屏参数作为实现参考；本地公开工具资源按 SHA-256 清单直接复用。
- 官方下载端点在本次环境连接失败，因此本次便携包使用本地 escrcpy 中已校验的 **scrcpy 4.1 + adb 37.0.0**。脚本同时提供官方 scrcpy 5.0.1 固定归档模式；该下载路径尚未在本机成功验证。
- 本地资源固定到 escrcpy 提交 `3a73e0d4de58a4674dddd61fc43956b4882c8bec`；GoIM 参考提交为 `262cb041388e6023aa02937b22d8d3e18eff4d43`。具体输入哈希在 `scripts/escrcpy-tools.lock.json`，实际交付来源在包内 `tools/scrcpy/source.json`。
- adb CLI 与 scrcpy 统一使用本机 5037 端口，屏蔽冲突的 adb 服务地址环境变量。通过 adb smart socket 的 `host:track-devices-l` 接收设备变化，空闲时阻塞读取，无界面重绘定时器。监听故障时以最多 15 秒间隔重试；重复状态不唤醒界面。
- 真实工具检查发现 adb 显式使用 `-H 127.0.0.1` 会禁止自动启动服务，已改为本机默认主机和显式端口；相关环境一致性已加入测试。
- 命令通过参数数组运行，中文／空格路径保持为独立参数。Windows 子进程隐藏控制台；scrcpy 投屏窗口由 SDL 自行创建。
- 每设备一个会话；Windows 停止时向本应用启动的 scrcpy 窗口发送 `WM_CLOSE`，等待其封装录像，8 秒未响应才强制终止。应用退出通过上下文取消统一清理自己的会话，不关闭共享 adb 服务。
- 配置采用原子替换写入；配对码仅用于当前请求并立即清空输入框。配置损坏时拒绝自动覆盖。任务保留在内存，最多保留最近 100 条已结束任务，退出后不保留历史。
- 后台只产生独立状态快照，通过 `Invalidator` 唤醒并在 `OnWake` 合并。窗口绑定完成后补一次唤醒，避免启动结果落在初次绑定之前。

## 已实现

USB 设备监听及授权／离线状态、设备搜索和连接类型过滤、无线 IP 连接、六位码配对、USB 切换 TCP/IP、按设备分别投屏、启动即录屏、PNG 截图、APK 安装、参数预设、任务结果、原生文件选择器、浅色／深色主题、运行环境诊断和隔离配置目录。

设备与连接、任务中心、设置均通过原生 Viva 绘制，包含小窗口滚动和关闭会话确认。没有模拟设备混入正常运行流程。

## 验证记录

环境：Windows x64，Go 1.27.3，Intel Core i5-10400，Viva v0.2.3。

- `gofmt`、`go vet ./...`、`go test ./... -count=1` 通过。
- 配置测试：中文路径、连续覆盖保存、设备预设、无效参数拒绝写入、损坏配置保护。
- 工具／协议测试：USB 与无线状态解析、adb 长度帧和截断数据、完整订阅握手和取消、地址校验、环境变量一致性、参数边界及有界日志。
- 后端子进程测试：adb 零退出码失败、配对错误隐藏秘密、PNG 输出、中文及空格 APK 路径、scrcpy 启动失败后释放会话、未授权设备拦截、快照隔离和配置落盘。子进程替身不连接真实设备。
- Viva 无头交互测试：设备切换与正确序列号、录屏参数、禁用未授权操作、配对码清除和端口分离、设置保存、主题与错误结果展示。
- 界面布局检查覆盖浅色／深色与 1000×700、1240×820、1600×1000；修复初版内容高度撑出窗口的问题。
- `scripts/smoke-headless.ps1` 使用真实 adb／scrcpy、真实业务后端和隔离配置目录验证启动、订阅成功、运行环境页面、主题保存、布局、截图及正常退出。真实 adb 当前返回 **0 台设备**。
- 将发布 ZIP 解压到含中文与空格的目录后，逐项核对包内 **29 个文件**的 SHA-256，并使用解压后的工具重新运行无头冒烟，全部通过。
- `BenchmarkPaint` 优化后，三台合成设备、1240×820 的强制重绘约 **81.8 μs/op、0 B/op、0 allocs/op**。这是应用与 Viva Driver 的指定场景基准，不代表所有页面或原生 GPU 帧耗时。优化前为约 85.3 μs/op、296 B/op、13 allocs/op；将展示字符串与标识缓存移至状态更新阶段。
- 无头启动耗时与进程内存记录在 `artifacts/headless/measurements.json`，仅代表带真实工具的无头模式，不代表原生窗口的启动／内存基准。
- 最终 stripped 应用大小为 5,796,864 字节（约 5.53 MiB），含完整运行工具的 ZIP 约 13.5 MiB；这是新建项目，没有先前发行版作为大小基线。
- 尝试 race 检查时，当前工具链报告 `-race requires cgo`；本机未配置 C 编译器，未完成 race 检查。

## 交付与复现

```powershell
pwsh -File scripts/fetch-tools.ps1 -EscrcpyRoot E:\01.Dev\github-back\escrcpy
pwsh -File scripts/smoke-headless.ps1
pwsh -File scripts/build.ps1 -EscrcpyRoot E:\01.Dev\github-back\escrcpy
```

便携包：`dist/Go-Scopy-0.1.0-windows-x64.zip`。包内保留同级 `tools`、完整第三方许可、来源清单和逐文件哈希。应用源码、工具源和参考源保持独立。

截图：`artifacts/screenshots/devices-light.png` 与 `devices-dark.png` 使用明确的合成设备元数据；`artifacts/headless/real-tools-light.png` 与 `real-tools-dark.png` 来自真实后端，当前无连接手机。

## 待真机验收及边界

尚无连接的 Android 设备，不能声明真实投屏、鼠标键盘控制、音频、录屏文件可播放、物理拔线恢复、无线配对和 APK 实际安装已通过验证。需使用真实 Android 手机完成这些验收，尤其确认 Windows 的 `WM_CLOSE` 正常封装 MP4。

未打开可见原生窗口，原生 IME、高 DPI、窗口管理器交互未实测。macOS／Linux 保留平台边界，但不属于本次发布与验证范围。未实现群控、嵌入式画面、文件管理、自动化和自动更新。
