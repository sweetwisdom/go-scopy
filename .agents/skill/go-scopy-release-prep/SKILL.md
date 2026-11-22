---
name: go-scopy-release-prep
description: 在 Go-Scopy 功能开发完成、准备新版时，根据实际改动同步中英文 README、CHANGELOG 和 scripts/verify-release.py 中的 Release 文案，按用户要求更新版本及暂存代码。适用于“更新发布说明”“准备发新版”等请求，默认只准备文件，不构建或发布。
---

# Go-Scopy 发布准备

与用户用中文交流。延续仓库现有文档结构，用用户能理解的简短语言说明最终功能。

## 确定本次更新范围

- 在当前 Go-Scopy 工作区执行；本机常用路径为 `D:\dev\2026-github\go-scopy`，以当前工作区为准。
- 读取适用的 AGENTS.md、`git status --short`、暂存区及工作区 diff、最新 CHANGELOG 和当前版本。用户已经手改的 README、截图、文案都作为现有输入保留。
- 结合当前会话及必要的提交历史确认上个发布版本之后的功能，不只依据尚未提交的 diff，避免漏掉已提交的新功能或重复记录旧功能。不要只凭界面名称推断行为。
- 默认只更新说明文件。用户明确要求更新版本时才改版本；指定版本优先，否则按现有版本策略确定目标并告知。若已有本次待发版本，继续完善同一版本，不重复递增；无法判断已发布还是待发布时先询问。

## 同步说明

| 文件 | 更新内容 |
| --- | --- |
| `README.md` | 英文功能说明、用法、安装示例和截图 |
| `README.zh-CN.md` | 对应中文说明，与英文功能、限制和版本一致 |
| `CHANGELOG.md` | 在本次版本下记录简洁更新；保留历史条目；同版本已存在则合并，不重复建条目 |
| `scripts/verify-release.py` | 更新 `release_notes(version, commit)` 返回的发布文案；保留动态版本、源码提交及下载包/校验信息 |

- 更新现有相关功能条目，避免每次在 README 累积一整份更新日志。CHANGELOG 和 Release 使用简短项目符号描述用户可见变化，不罗列内部文件名或实现过程。
- 中英文描述保持一致；Release 沿用中英双语，中文在前，英文在后。
- 只写有实现依据的功能，不把编译通过写成真机验收通过。例如平板预设不保证 App 启用平板布局，外接键盘配合 Android 输入法不等于电脑中文输入法直接透传。相关限制放在对应功能附近。
- README 当前采用三张图片：第一张单独一行，后两张放在两列 HTML 表格中并排。仅有新截图或用户要求调整时更新图片，保持相对路径和有意义的 alt；新图片要纳入 Git。
- README 的本地资源应随现有打包逻辑分发；需要时检查 `cmd/package/main.go` 的复制目录，避免离线阅读出现失效图片。不要额外加入开发截图或忽略的临时报告。
- 只修改发布文案和必要的相关断言，不改变 `verify-release.py` 的产物完整性、版本、来源提交及哈希校验逻辑。

## 按要求同步版本

更新版本时检查这些入口，并搜索其他生效的版本默认值：

- `internal/buildinfo/version.go`：应用和 Go 打包器默认版本。
- `.github/workflows/release.yml`：手动工作流版本默认值。
- `scripts/build.ps1`：本地构建脚本版本默认值。
- 两份 README 的版本标题、安装包命令及本次 CHANGELOG 标题。

不要全仓替换版本字符串；历史 CHANGELOG、旧文档记录和测试夹具版本不必跟随升级。`resources/manifest.json` 的版本属于外部工具资源，不是 Go-Scopy 应用版本。

## 验证与交付

- 文档和发布文案改动运行 `python -m unittest discover -s scripts -p 'test_*.py'` 及 `git diff --check`。若 `scripts/test_release.py` 对旧文案有断言，随新发布内容更新；不要为了通过测试恢复过期说明。
- 检查两份 README 的图片路径存在、主要功能和版本一致。仅当 Go 代码也有改动且本轮未验证时，按变化范围补充必要检查；单纯文案改动不要求全量 Go 测试或构建。
- **默认不执行构建和打包。** 不运行 `go run ./cmd/package`、`scripts/build.ps1` 或触发 GitHub Actions，除非当前任务另有明确要求。测试发布脚本可以运行本地 unittest，不实际执行 `verify-release.py` 或 `publish-release.py` 的发布入口。
- 用户明确要求“全部加到暂存区”时，先检查新增文件及忽略规则，再执行 `git add -A`，用 `git diff --cached --check` 和 `git status --short` 确认。不要强制加入 dist、artifacts 或忽略的本地文档。
- 准备说明及暂存不包含提交、打标签、推送或创建远程 Release 的授权；这些操作只在用户明确要求时执行。
- 最后简述目标版本、同步的说明及暂存/验证结果，明确仍需用户处理的真实问题。不要把文件已准备好表述为版本已发布。
