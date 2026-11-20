# 第三方声明

Go-Scopy 的应用代码采用 MIT 许可证。以下组件和资源保留各自许可证；许可全文位于 `licenses/`。

| 项目 | 用途 | 许可证／声明 |
| --- | --- | --- |
| [GoIM](https://github.com/Ezreal-byte/GoIM) | 三栏布局、配色、原生 Viva 组织方式、Actions 矩阵与打包辅助函数的参考及改编 | MIT，Copyright (c) 2026 Ezreal-byte；`GoIM-MIT.txt` |
| [escrcpy](https://github.com/viarotel-org/escrcpy) | 连接与录屏工作流参考，跨平台工具资源来源；设备终端 xterm.js hook 和 GitHub 深浅色主题移植（`internal/terminalview/src`，Go 通信与生命周期适配） | Apache-2.0；`escrcpy-Apache-2.0.txt` |
| [Viva](https://github.com/vivaui/viva), v0.2.3 | 原生 UI 与测试驱动 | Apache-2.0；`Viva-Apache-2.0.txt`、`Viva-NOTICE.txt`、`Viva-third-party.md` |
| [purego](https://github.com/ebitengine/purego), v0.11.1 | Viva 的平台互操作依赖 | Apache-2.0；`purego-Apache-2.0.txt` |
| motty（`internal/term` 与 `internal/tools/pty` 移植及文件图标矢量） | VT/xterm 终端仿真器、PTY 实现及文件图标 | Apache-2.0；`motty-Apache-2.0.txt`；上游 github.com/motty/motty；图标另见下行 |
| [Material Icon Theme](https://github.com/material-extensions/vscode-material-icon-theme) | 文件类型图标（Motty 图标子集） | MIT；`Material-Icon-Theme-MIT.txt` |
| [xterm.js](https://github.com/xtermjs/xterm.js)，6.0.0；FitAddon 0.11.0 | WebView 交互式终端与面板尺寸自适应 | MIT；`xterm-MIT.txt`、`xterm-addon-fit-MIT.txt` |
| [Vue](https://github.com/vuejs/core)，3.5.24 | 复用 escrcpy 终端 hook 的响应式与组件生命周期 | MIT；`Vue-MIT.txt` |
| [Fig Autocomplete](https://github.com/withfig/autocomplete)，2.692.3 | 保留 ADB 命令规格供后续补全使用，当前终端未启用或打包此规格 | MIT；`Fig-Autocomplete-MIT.txt` |
| [Go](https://go.dev/) | 工具链与标准库 | BSD；`Go-BSD.txt` |
| [scrcpy](https://github.com/Genymobile/scrcpy) | 原样分发的客户端与 Android server | Apache-2.0；`scrcpy-Apache-2.0.txt` |
| [Android adb](https://android.googlesource.com/platform/packages/modules/adb/) | 原样分发的 Android 调试工具与驱动 DLL | Android Open Source Project，Apache-2.0 及其第三方声明；见 `adb-NOTICE.txt` |
| [SDL](https://github.com/libsdl-org/SDL) | scrcpy 原样附带的 SDL3.dll | zlib；`SDL-zlib.txt` |
| [FFmpeg](https://ffmpeg.org/) | scrcpy 原样附带的 avcodec、avformat、avutil、swresample 动态库 | LGPL-2.1-or-later；`FFmpeg-LGPL-2.1.txt` |
| [libusb](https://github.com/libusb/libusb) | scrcpy 原样附带的 libusb 动态库 | LGPL-2.1-or-later；`libusb-LGPL-2.1.txt` |
| Go-Scrcpy Agent（`resources/extra/common/go-scrcpy-agent.jar`） | Android 端应用信息与图标辅助服务，源码位于独立子项目 | 参考既有 PandaKit 协议与分析材料；公开分发前仍须核对源材料许可 |
| CPython（用户自行安装，不随当前发行包分发） | PATH 或指定路径的独立脚本解释器 | PSF 及其自身第三方许可；由用户所用发行版提供 |
| uiautomator2 3.7.1（用户自行安装） | Python UI 自动化 | MIT；安装发行版内保留 dist-info/LICENSE |
| adbutils、Pillow、lxml、requests 等锁定依赖（不随当前包分发） | 用户 Python 自动化依赖 | 版本与哈希见 requirements.lock；许可由安装的各 wheel 提供 |

Go-Scopy 没有迁入 escrcpy 的 Electron 宿主、私有扩展或手机控制协议实现。设备终端移植了其公开的 Vue/xterm.js hook 与主题，并适配 Go/WebView 通信；来源提交与文件清单见 `internal/terminalview/sources.json`。离线导入仅原样复制清单中的公开工具资源，应用不链接这些工具；macOS 打包时重新进行 ad-hoc 签名，其余工具内容保持原样。Viva 的上游声明覆盖其内含的图标、文字数据等第三方材料。

## 工具来源与对应源码

- 发布包 `tools/scrcpy/source.json` 记录实际使用的版本、来源和校验值。
- 官方模式：Genymobile/scrcpy v5.0.1，归档下载地址和 SHA-256 见 `scripts/tools.lock.json`。
- 0.1.5 默认发布资源：完整复制源仓库 `desktop/electron/resources/extra` 到 `resources/extra`，来源与全部 SHA-256 见 `resources/manifest.json`。发布包仅包含对应平台 scrcpy 目录及共用 server，macOS 位于 `.app/Contents/Resources/tools/scrcpy`。`fastboot` 随平台目录一同分发，遵循 AOSP 许可证；源码：<https://android.googlesource.com/platform/system/core/+/refs/heads/main/fastboot/>。
- 原 Windows 本地离线模式：viarotel-org/escrcpy 提交 `3a73e0d4de58a4674dddd61fc43956b4882c8bec` 中附带的 scrcpy 4.1；各文件 SHA-256 见 `scripts/escrcpy-tools.lock.json`。
- [scrcpy v4.1 源码及构建说明](https://github.com/Genymobile/scrcpy/tree/v4.1)、[v5.0.1](https://github.com/Genymobile/scrcpy/tree/v5.0.1)。依赖版本可通过包内 `scrcpy --version` 查看。
- FFmpeg 对应源码：<https://github.com/FFmpeg/FFmpeg>；scrcpy 的 FFmpeg 构建配置／源码版本见对应标签下的 [release scripts](https://github.com/Genymobile/scrcpy/tree/v4.1/release)。动态库保持可替换，应用本身不链接这些库。
- SDL 对应源码：<https://github.com/libsdl-org/SDL/tree/release-3.4.12>；libusb：<https://github.com/libusb/libusb/tree/v1.0.30>。
- Android adb 源码：<https://android.googlesource.com/platform/packages/modules/adb/>，官方平台工具及声明：<https://developer.android.com/tools/releases/platform-tools>。


## Python 脚本编辑器（CodeMirror 6）

独立脚本工作台复用 CodeMirror 官方基础设置、Python 语言支持、补全/代码片段和 one-dark 主题，未接入语言服务器。入口依赖版本锁定于 `internal/scriptview/package-lock.json`：`codemirror` 6.0.2、`@codemirror/lang-python` 6.2.1、`@codemirror/autocomplete` 6.20.4、`@codemirror/commands` 6.11.1、`@codemirror/view` 6.43.15、`@codemirror/state` 6.7.6、`@codemirror/language` 6.13.1、`@codemirror/theme-one-dark` 6.1.3。

- CodeMirror / Lezer 官方源码：https://github.com/codemirror 、https://github.com/lezer-parser 。
- CodeMirror、Lezer 等包采用 MIT；依赖各自的原始声明和版权文本在构建时收集到 `internal/scriptview/assets/licenses.txt` 和 `licenses/CodeMirror-editor-licenses.txt`，随 Go 二进制内嵌及发行包分发；esbuild 额外保留 `workspace.js.LEGAL.txt`。
- 补充的 goscrcpy/uiautomator2 提示是本项目静态模板，不是从其他编辑器或 AI 服务复制的语言服务器。


## Windows 安装包

复用 Viva v0.2.3 的 `tools/winpkg.Script` NSIS 模板（Viva MIT 声明见既有许可），仅扩展完整工具/文档 payload 的逐文件安装与卸载清单。编译器采用 NSIS 3.12 官方 Windows ZIP，来源和下载件 SHA-256 见 `scripts/nsis.lock.json`；没有安装到系统，也未修改 NSIS 源码。NSIS 原始许可在 `licenses/NSIS-COPYING.txt`；上游源码分发地址见锁定文件的 `sourceArchive`。安装包不捆绑 NSIS 编译器，不注册或修改用户的 `.md` 默认关联。
