# Device terminal WebView

The terminal reuses escrcpy's Vue/xterm.js hook and GitHub light/dark themes.
The Electron IPC surface is replaced with the existing Viva WebView bridge.
Pinned upstream files and versions are recorded in `sources.json`; licenses
are retained in the repository's `licenses/` directory.

## Scope

- xterm.js 6.0.0, FitAddon 0.11.0, Vue 3.5.24.
- Shell and read-only Logcat; history survives tab/device switches.
- Responsive terminal grid with ResizeObserver + FitAddon; hidden panes are
  fitted again on activation. No custom VT parser, line editor or renderer.
- Optional, collapsible reference sidebar; copying never executes a command.
- No CodeMirror/editor autocomplete is shipped. The already-imported Fig ADB
  spec and candidate-query helpers are reserved for deferred work; neither is
  requested by this page or included in its frontend bundle.

## Build

Generated `assets/` files are committed and embedded by Go, so a normal Go
build and release packaging do **not** require Node, npm or network access.
After editing frontend sources, regenerate the bundle:

```powershell
npm ci --prefix internal/terminalview
npm run build --prefix internal/terminalview
npm test --prefix internal/terminalview
```

The lockfile pins dependencies. No runtime CDN, local HTTP server, Electron,
Python or Node process is used by the desktop terminal. `browser.test.mjs`
uses a temporary loopback HTTP server solely for browser regression tests.

## Process and message lifecycle

`tools.DeviceShell` runs `adb -P 5037 -s SERIAL shell -tt` with ordinary pipes,
matching escrcpy's device provider. Windows launches it without a console
window. The existing bounded PTY `Shell` I/O queue is shared with this pipe
backend; the device-terminal path does not instantiate local Windows ConPTY.
Windows Enter/paste CRs are converted to CRLF before writing to adb's pipe.

The page receives device/theme/visibility context on the UI thread. Process
output is forwarded as base64-encoded bytes in chunks of at most 32 KiB. A
session gets its next chunk only after xterm's `write` callback ACKs the prior
one, preserving UTF-8 across chunks and bounding Logcat backlog. Device input
must match both the selected serial and an existing session while the device
terminal page is active. Logcat rejects all write requests. Reload, disconnect,
normal close and programmatic window close release the owned child processes;
the shared adb server is never stopped.

### Sizing boundary

FitAddon resizes the **visible browser terminal grid** to its actual pane.
Like escrcpy's `DeviceTerminalProvider.resize`, the pipe provider does not
propagate live SIGWINCH/terminal dimensions into Android. Full-screen remote
programs may therefore retain the device shell's initial dimensions. This is
not claimed as remote TUI resize support; automatic `stty` injection was not
added because it would modify an interactive input stream and differ from
the reference provider.

## Verification

```powershell
go test ./... -count=1
go vet ./...
# Uses the repository's existing local Playwright installation. Alternatively
# set GOSCOPY_PLAYWRIGHT to an installed playwright/index.mjs absolute path.
node internal/terminalview/browser.test.mjs

$env:GOSCOPY_LIVE_TERMINALVIEW='1'
$env:GOSCOPY_LIVE_ADB=(Resolve-Path 'resources/extra/win/scrcpy/adb.exe').Path
$env:GOSCOPY_LIVE_SERIAL='127.0.0.1:5555'
$env:GOSCOPY_TERMINAL_REPORT=(Join-Path (Get-Location) 'artifacts/terminal-web/native-verification.json')
go test ./internal/desktop -run '^TestTerminalWorkspaceLiveWebView$' -count=1 -v
```

The native test uses the actual WebView2, xterm textarea/key/paste handlers and
adb child processes, not a mock transport. It runs printf markers and a short
interruptible sleep, reads Logcat without saving device log contents, checks
fitting/history, then closes all sessions. Browser-only tests cover 420/852/
1280 px panel widths, light/dark themes and 1x/1.5x device scales. Browser
screenshots use synthetic output and are not presented as device captures.
