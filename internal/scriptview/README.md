# Python script workspace

Offline CodeMirror 6 notebook embedded in the shared native WebHost. The application rail owns the standalone entry; the WebView owns its file list, editor, device picker and output scrolling. No localhost server, CDN, Python runtime download, AI service or language server is used in production.

## Reuse

- CodeMirror 6 `basicSetup`, official Python language support (built-ins + local names), autocomplete/snippet API, one-dark theme.
- Existing `domain.ScriptBackend` / `automation.Manager` for process lifecycle, per-device concurrency, bounded output, timeout and persisted runs.
- Existing shared WebHost and system file/open-path APIs.
- `scriptstore` persists real UTF-8 `.py` files under the application data directory's `scripts` folder. The native picker can register external files for in-place editing. Save checks a SHA-256 revision token to reject detected external changes, writes through a same-directory temporary file, and never overwrites on create. This is not an OS-level lock against simultaneous external writers.
- The development guide is maintained in `docs/python-script-development.md`, copied into embedded assets on build and exported to `scripts/.help/Python脚本开发指南.md` when requested. The native `app.OpenPath` delegates to the OS `.md` association. Release archives also include `tools/python/DEVELOPMENT.zh-CN.md`.

## Build and tests

```powershell
npm ci --prefix internal/scriptview
npm run build --prefix internal/scriptview
npm test --prefix internal/scriptview
node internal/scriptview/browser.test.mjs
go test ./internal/scriptstore ./internal/scriptview ./internal/desktop ./cmd/package -count=1
```

Commit generated `assets` and `package-lock.json`; ordinary Go builds use those assets without npm. Build collects the production packages' license texts into `assets/licenses.txt` and esbuild's linked notices.

The browser regression reuses local Playwright under `artifacts/fileview-browser/node_modules/playwright` (or `GOSCOPY_PLAYWRIGHT`) and Edge. Its loopback server and synthetic devices exist only in the test.

Opt-in real Windows WebView/Python/ADB test:

```powershell
$env:GOSCOPY_LIVE_SCRIPTVIEW='1'
$env:GOSCOPY_LIVE_ADB=(Resolve-Path 'resources/extra/win/scrcpy/adb.exe').Path
$env:GOSCOPY_LIVE_SERIAL='127.0.0.1:5555'
$env:GOSCOPY_LIVE_PYTHON='C:\path\to\python.exe'
$env:GOSCOPY_SCRIPT_REPORT=(Join-Path (Get-Location) 'artifacts/script-workspace/native-verification.json')
go test ./internal/desktop -run '^TestScriptWorkspaceLiveWebView$' -count=1 -v
```

The chosen interpreter must already provide uiautomator2 3.7.1 and the locked dependencies. The native test reads `getprop ro.product.model`, saves/executes a temporary local file and stops a sleeping local process. It does not initialize UI automation on the phone. The guide opener is intercepted to verify a real Markdown file without opening a user application during unattended testing.

## Boundaries

Python files are not sandboxed. Trust is cleared after editing; target devices must be explicitly chosen and online at execution. Runs continue across page changes. Unsaved edits warn on file switching and normal application exit; they are not a crash-recovery database. Supplementary `d.` completions are labeled static presets, not type inference. Large documents over 1 MiB and non-UTF-8 content are rejected. No rename/delete/batch-device execution is included in this first iteration.
