# OK Browser

A **very light and very fast** web browser for **Windows 10 and later**.

- **~3.7 MB** single `OKBrowser.exe` — no installer, no framework, no bloat
- Renders with **Microsoft's Edge WebView2 engine** (the Chromium engine built into Windows) — full modern web support with native speed
- **Native Win32 chrome** — one toolbar, one address bar, instant startup
- **No telemetry, no accounts, no background services**

![OK Browser](res/icon-src.png)

## Download

Download `OKBrowser.exe` from the [Releases](../../releases) page, or build it
yourself in one command (see below). Every release is built **and smoke-tested
on a real Windows machine** by CI before it is published.

## Requirements

| | |
|---|---|
| OS | Windows 10 (1607+) or Windows 11, 64-bit |
| Runtime | Microsoft Edge WebView2 Runtime — **pre-installed** on Windows 11 and all updated Windows 10 PCs. If missing, OK Browser detects it and offers to open Microsoft's official download page (small, free installer). |

## Features

- Address bar with smart parsing: `example.com` → `https://example.com`, plain words → DuckDuckGo search, `localhost:3000` / `192.168.x.x` → `http://`
- Back, forward, reload, home buttons + an offline, instant start page
- `target="_blank"` links, middle-click and `window.open()` open a **new OK Browser window** (one window per process — each stays feather-light)
- Per-window web engine isolation; a crashed page never takes the browser down
- Right-click context menus, F12 DevTools, Ctrl+mouse-wheel zoom, hover link preview — the browser basics you expect
- High-DPI aware (crisp on any monitor, follows the window between screens)
- Persistent profile: logins and cookies are kept in `%LOCALAPPDATA%\OKBrowser`
- Proper Windows app icon, version info and GUI subsystem (no console flash)

### Keyboard shortcuts

| Keys | Action |
|---|---|
| `Ctrl+L` / `Alt+D` | Focus the address bar (selects all) |
| `Enter` | Navigate to what you typed |
| `Alt+←` / `Alt+→` | Back / forward |
| `F5` / `Ctrl+R` | Reload |
| `Alt+Home` | Start page |
| `Ctrl+N` / `Ctrl+T` | New window |
| `Ctrl+W` | Close window |
| `Ctrl+` / `Ctrl-` / Ctrl+wheel | Zoom (wheel is handled by the engine) |
| `F12` | DevTools |

## Build from source

All dependencies are vendored under `third_party/` — the repo builds **fully
offline** with nothing but the Go toolchain installed.

**On Windows:**

```bat
scripts\build.bat
```

**On Linux or macOS (cross-compile):**

```sh
scripts/build.sh
```

The result is `dist/OKBrowser.exe` in both cases.

If you change the icon or version, regenerate the Windows resource file with
`scripts/resources.sh` (needs `goversioninfo`; see the script header).

## Architecture notes

OK Browser deliberately trades “features” for **lightness and speed**:

- **Pure Go** (~4k lines including a vendored Win32 binding) — no Electron, no CEF, no .NET
- The whole UI is **raw Win32**: a toolbar of native controls and a host window. There is no UI framework to load, so the window appears instantly
- Web content is rendered by the **system's** WebView2 runtime — the engine is not shipped in the exe (that's why it's 3.7 MB) and is kept updated/patched by Windows Update
- **One window per process**: `Ctrl+N` (or a `_blank` link) starts a fresh, tiny process. Windows are isolated; closing one frees everything
- Address-bar parsing lives in `internal/nav` and is pure Go with unit tests, so the URL logic is verified on every platform

```
cmd/okbrowser/        the browser (Windows-only)
  main.go             entry point + WebView2 runtime check
  app.go              window, toolbar, layout, hotkeys, message loop
  bridge.go           page ↔ host bridge (URL/title sync, new-window requests)
  winx.go             a few raw Win32 calls lxn/win lacks
  resource.syso       icon + manifest + version info (compiled resource)
internal/nav/         pure, unit-tested URL parsing + start page
res/                  icon, manifest, versioninfo
scripts/              build + smoke-test scripts
third_party/          vendored dependencies (see each LICENSE)
```

## Third-party components

| Component | License | Purpose |
|---|---|---|
| [go-webview2](https://github.com/jchv/go-webview2) | MIT | Pure-Go WebView2 (Edge engine) embedding |
| [go-winloader](https://github.com/jchv/go-winloader) | MIT | Loads the WebView2 bootstrapper from memory (no DLL shipping) |
| [lxn/win](https://github.com/lxn/win) | BSD-3 | Win32 API bindings |
| [golang.org/x/sys](https://github.com/golang/sys) | BSD-3 | Low-level Windows syscalls |
| Microsoft Edge WebView2 Runtime | Microsoft | System rendering engine (installed with Windows) |

One small patch is applied to the vendored go-webview2 (marked with an
`OK Browser patch` comment in `third_party/go-webview2/pkg/edge/chromium.go`):
the upstream code echoes every host message back into the page, which could
confuse pages that listen for their own `postMessage` events. The echo is
removed so host<->page communication stays strictly one-way.
