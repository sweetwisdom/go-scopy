# 第三方声明

Go-Scopy 的应用代码采用 MIT 许可证。以下组件和资源保留各自许可证；许可全文位于 `licenses/`。

| 项目 | 用途 | 许可证／声明 |
| --- | --- | --- |
| [GoIM](https://github.com/Ezreal-byte/GoIM) | 三栏布局、配色、原生 Viva 组织方式、Actions 矩阵与打包辅助函数的参考及改编 | MIT，Copyright (c) 2026 Ezreal-byte；`GoIM-MIT.txt` |
| [escrcpy](https://github.com/viarotel-org/escrcpy) | 连接与录屏工作流参考，跨平台工具资源来源 | Apache-2.0；`escrcpy-Apache-2.0.txt` |
| [Viva](https://github.com/vivaui/viva), v0.2.3 | 原生 UI 与测试驱动 | Apache-2.0；`Viva-Apache-2.0.txt`、`Viva-NOTICE.txt`、`Viva-third-party.md` |
| [purego](https://github.com/ebitengine/purego), v0.11.1 | Viva 的平台互操作依赖 | Apache-2.0；`purego-Apache-2.0.txt` |
| [Go](https://go.dev/) | 工具链与标准库 | BSD；`Go-BSD.txt` |
| [scrcpy](https://github.com/Genymobile/scrcpy) | 原样分发的客户端与 Android server | Apache-2.0；`scrcpy-Apache-2.0.txt` |
| [Android adb](https://android.googlesource.com/platform/packages/modules/adb/) | 原样分发的 Android 调试工具与驱动 DLL | Android Open Source Project，Apache-2.0 及其第三方声明；见 `adb-NOTICE.txt` |
| [SDL](https://github.com/libsdl-org/SDL) | scrcpy 原样附带的 SDL3.dll | zlib；`SDL-zlib.txt` |
| [FFmpeg](https://ffmpeg.org/) | scrcpy 原样附带的 avcodec、avformat、avutil、swresample 动态库 | LGPL-2.1-or-later；`FFmpeg-LGPL-2.1.txt` |
| [libusb](https://github.com/libusb/libusb) | scrcpy 原样附带的 libusb 动态库 | LGPL-2.1-or-later；`libusb-LGPL-2.1.txt` |

Go-Scopy 没有迁入 escrcpy 的 Electron／Vue 界面、私有扩展或手机控制协议实现。离线导入仅原样复制清单中的公开工具资源，应用不链接这些工具；macOS 打包时重新进行 ad-hoc 签名，其余工具内容保持原样。Viva 的上游声明覆盖其内含的图标、文字数据等第三方材料。

## 工具来源与对应源码

- 发布包 `tools/scrcpy/source.json` 记录实际使用的版本、来源和校验值。
- 官方模式：Genymobile/scrcpy v5.0.1，归档下载地址和 SHA-256 见 `scripts/tools.lock.json`。
- 0.1.5 默认发布资源：完整复制源仓库 `desktop/electron/resources/extra` 到 `resources/extra`，来源与全部 SHA-256 见 `resources/manifest.json`。发布包仅包含对应平台 scrcpy 目录及共用 server，macOS 位于 `.app/Contents/Resources/tools/scrcpy`。`fastboot` 随平台目录一同分发，遵循 AOSP 许可证；源码：<https://android.googlesource.com/platform/system/core/+/refs/heads/main/fastboot/>。
- 原 Windows 本地离线模式：viarotel-org/escrcpy 提交 `3a73e0d4de58a4674dddd61fc43956b4882c8bec` 中附带的 scrcpy 4.1；各文件 SHA-256 见 `scripts/escrcpy-tools.lock.json`。
- [scrcpy v4.1 源码及构建说明](https://github.com/Genymobile/scrcpy/tree/v4.1)、[v5.0.1](https://github.com/Genymobile/scrcpy/tree/v5.0.1)。依赖版本可通过包内 `scrcpy --version` 查看。
- FFmpeg 对应源码：<https://github.com/FFmpeg/FFmpeg>；scrcpy 的 FFmpeg 构建配置／源码版本见对应标签下的 [release scripts](https://github.com/Genymobile/scrcpy/tree/v4.1/release)。动态库保持可替换，应用本身不链接这些库。
- SDL 对应源码：<https://github.com/libsdl-org/SDL/tree/release-3.4.12>；libusb：<https://github.com/libusb/libusb/tree/v1.0.30>。
- Android adb 源码：<https://android.googlesource.com/platform/packages/modules/adb/>，官方平台工具及声明：<https://developer.android.com/tools/releases/platform-tools>。
