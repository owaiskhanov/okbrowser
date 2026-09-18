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

**Liquid Glass UI, frameless** — the native title bar is gone; the app is
just content:

- **Tabs live in the topmost window bar**: sleek glass pills in the title-bar
  strip. Drag the empty strip to move the window, double-click to maximize;
  our own Windows min / max / close buttons sit at the top right — the native
  title bar and its buttons are fully removed (WM_NCCALCSIZE frameless
  technique), so ours is the only top bar
- **The address bar is a tiny glass bubble** floating at the bottom center
  (iOS-style). It stays out of sight; hover or click it (or press `Ctrl+L`)
  and it liquidly expands into the full address capsule with back / forward /
  reload / go — collapse happens automatically when you leave it
- Everything renders inside the web engine's GPU compositor with real
  `backdrop-filter` blur of the page beneath the glass
- Light and dark glass follow Windows automatically; everything hides during
  fullscreen video and printing

- **Tabs**: click to switch, `+` for a new one, middle-click or ✕ to close,
  `Ctrl+Tab` / `Ctrl+Shift+Tab` to cycle. Every tab is its own web engine
  instance sharing one engine process, so tabs stay isolated and cheap
- Address bar with smart parsing: `example.com` → `https://example.com`, plain words → **Google** search, `localhost:3000` / `192.168.x.x` → `http://`
- The address bar updates **the instant** you navigate (not after the page loads)
- Back / forward / reload appear as tiny glyphs that light up only when usable (real history state)
- `target="_blank"` links, middle-click on links and `window.open()` open a **new tab**; `Ctrl+N` opens a new window
- Zoom (`Ctrl+ +` / `Ctrl+-` / `Ctrl+0`), print (`Ctrl+P`), fullscreen (`F11`), mouse buttons 4/5 for back/forward
- Right-click context menus, F12 DevTools, hover link preview, downloads (the engine's download UI)
- High-DPI aware (crisp on any monitor, follows the window between screens); the bar reflows correctly at **any window size**
- Persistent profile: logins and cookies are kept in `%LOCALAPPDATA%\OKBrowser`
- Proper Windows app icon, version info and GUI subsystem (no console flash)

### Keyboard shortcuts

| Keys | Action |
|---|---|
| `Ctrl+L` / `Alt+D` | Focus the address bar (selects all) |
| `Enter` | Navigate to what you typed |
| `Alt+←` / `Alt+→` | Back / forward |
| Mouse button 4 / 5 | Back / forward |
| `F5` / `Ctrl+R` | Reload |
| `Alt+Home` | Start page |
| `Ctrl+T` | New tab (and focuses the address bubble) |
| Hover the bottom bubble | Expand the address bar |
| `Ctrl+N` | New window |
| `Ctrl+W` | Close tab (window when last tab) |
| `Ctrl+Tab` / `Ctrl+Shift+Tab` | Next / previous tab |
| Middle-click a tab | Close tab |
| `Ctrl+ +` / `Ctrl+-` / `Ctrl+0` | Zoom in / out / reset |
| `Ctrl+P` | Print |
| `F11` | Fullscreen |
| `F12` | DevTools |

## Diagnostics

Run `OKBrowser.exe --selftest` to check every navigation path end to end:
typed URL, plain link click, `window.open` via the bridge, native
`window.open` via the engine's NewWindowRequested event, and `target=_blank`
link clicks. It writes `selftest.txt` next to the
exe and exits 0 only when everything passes. CI runs this on every build.

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
  app.go              window, slim tab bar, layout, hotkeys, message loop
  tabs.go             tab lifecycle + per-tab engine wiring
  bridge.go           page ↔ host bridge (URL/title sync, new-tab requests)
  winx.go             a few raw Win32 calls lxn/win lacks
  (the UI bar itself lives in bridge.go as injected CSS/JS)
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
