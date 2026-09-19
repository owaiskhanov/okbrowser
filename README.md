# OK Browser

A **very light and very fast** web browser for **Windows 10 and later**.

- **~4 MB** single `OKBrowser.exe` — no installer, no framework, no bloat
- Renders with **Microsoft's Edge WebView2 engine** (the Chromium engine built into Windows) — full modern web support with native speed
- **Native Win32 chrome** — one toolbar, one address bar, instant startup
- **No telemetry, no accounts, no background services**

![OK Browser](res/icon-src.png)

## Download

Download `OKBrowser.exe` from the [Releases](../../releases) page, or build it
yourself in one command (see below). Every release is built **and smoke-tested
on a real Windows machine** by CI before it is published. Releases include a
SHA-256 checksum and GitHub provenance; when the repository's Authenticode
secrets are configured, CI also signs, timestamps and verifies the EXE before
smoke testing and publication (see [`docs/AUTHENTICODE.md`](docs/AUTHENTICODE.md)).

## Requirements

| | |
|---|---|
| OS | Windows 10 (1607+) or Windows 11, 64-bit |
| Runtime | Microsoft Edge WebView2 Runtime — **pre-installed** on Windows 11 and all updated Windows 10 PCs. If missing, OK Browser detects it and offers to open Microsoft's official download page (small, free installer). |

## Features

**Liquid Glass UI, frameless** — the native title bar is gone; the app is
just content:

- **Immersive, content-first**: the page fills the whole window — nothing
  sits on top of your content while you browse. The glass bar (tabs, `+`,
  address bubble) **with the min / max / close capsule** hides away and
  glides back together the instant your mouse touches the top edge, and
  reveals itself on `Ctrl+T`, `Ctrl+L` and tab switches. It also responds
  continuously to cursor proximity: tabs and window controls begin gliding
  into view within 160 px of the top (or 210 px during a fast upward gesture),
  becoming fully interactive before the pointer arrives; it slides away again
  when you leave it. Drag the bar's empty middle to move the window,
  double-click it to maximize; the top edge resizes the window while the
  bar is hidden. No native title bar in any
  state (borderless `WS_POPUP | WS_THICKFRAME | WS_CAPTION` style +
  `WM_NCCALCSIZE`, DWM shadow kept alive) — CI hit-tests the real window in
  both states to prove no native caption or button area exists, and
  verifies the bar stays flush with the top after maximize → restore
- **The address bar is a tiny glass bubble right beside the `+` button** in
  the top bar. It stays out of sight; hover or click it (or press `Ctrl+L`)
  and it liquidly expands in place into the full address capsule with back /
  forward / reload / go — collapse happens automatically when you leave it
- Everything renders inside the web engine's GPU compositor with real
  `backdrop-filter` blur of the page beneath the glass
- Light and dark glass follow Windows automatically; everything hides during
  fullscreen video and printing

- **Liquid new tabs, zero flash**: a new tab stays completely hidden while
  its engine starts and paints — you keep seeing the previous tab — then
  appears as a soft translucent layer and liquidly fades to full. The
  engine's pre-paint background and every window erase are **dark** (not
  WebView2's default white), so nothing can ever flash white — including
  in dark mode
- **Tabs with favicons**: each tab shows the site's own icon (letter
  avatar as fallback). Pills **auto-collapse to favicon-only** when the tab
  strip gets crowded and grow back when there's room; hover a collapsed
  pill for the full title. Click to switch, `+` for a new one, middle-click
  or ✕ to close, `Ctrl+Tab` / `Ctrl+Shift+Tab` to cycle. **Drag to
  reorder**; **right-click** for duplicate / pin (favicon-only) / close
  others. Every tab is its own web engine instance sharing one engine
  process, so tabs stay isolated and cheap
- **Liquid link-edge gestures**: drag any link toward an edge and a springy,
  blurred drop surface flows in to meet the cursor. Drop on the **left** to queue
  it as a background tab for later, or on the **right** for a Peek preview that
  opens into a real side-by-side Split View. Split View uses a second WebView—not
  an iframe—keeps browser controls available across both panes, and includes a
  compact controls to resize, swap, promote, or close panes, a clear active-pane
  highlight with pane-aware keyboard commands, tab-to-pane actions, and full
  Split View session restore. Drag an existing tab to the left or right edge to
  place it directly into that side of a new two-pane layout
- **Site identity and permissions**: the lock control reports whether the
  current connection uses HTTPS and offers per-site Ask / Allow / Block choices
  for camera, microphone, location, notifications, clipboard and sensors;
  choices persist by origin, with one-click permission reset and per-site
  cookie/storage clearing
- **Windows-protected autofill**: password saving, passkeys, addresses and
  payment autofill are delegated directly to the WebView2 profile; OK Browser
  never reads or stores credential values itself
- **Sleeping tabs**: inactive background pages freeze after a configurable
  delay, wake instantly when selected, and visibly dim while asleep; pinned
  tabs, audio-playing tabs, tabs with unsaved forms or active downloads, and
  both Split View panes stay live. Right-click supports Sleep now and Never
  sleep this site; severe Windows memory pressure triggers early sleeping
- **Crash isolation and recovery**: renderer, GPU and browser-process failures
  are detected per tab, logged locally and automatically reloaded; repeated
  crashes stop safely on a recovery page instead of entering a reload loop
- **Liquid loading line**: a thin blue hairline runs along the top edge
  while a page loads and sweeps away when it's done
- **Incognito** (`Ctrl+Shift+N`): a private window with a throwaway
  profile — history, cookies and session data go to a temp folder, never
  to disk history
- **Downloads** (`Ctrl+J`): native WebView2 transfer tracking with exact progress,
  speed, source domain, pause/resume/cancel, interruption recovery, open/show/
  remove actions, executable safety warnings and automatic live refresh
- Address bar with smart parsing: `example.com` → `https://example.com`, plain words → search (engine selectable in Settings: **Google** / Bing / DuckDuckGo), `localhost:3000` / `192.168.x.x` → `http://`
- **Suggestions while you type**: your history and bookmarks appear in a glass dropdown under the address bubble (↑/↓ to pick, Enter to go)
- **Bookmarks**: tap the ★ in the address bubble (or `Ctrl+D`); manage them on the bookmarks page (`Ctrl+Shift+O` or the ☰ menu)
- **History**: every visit is logged locally (`Ctrl+H`) with per-entry delete, search and clear-all
- **New Tab speed dial**: your most-visited sites as icon-only glass tiles (hover
  for the title), plus a big search field
- **Session restore**: your tabs and window position come back on the next start (toggle in Settings)
- **☰ menu** beside the minimize button: new tab, incognito, bookmarks,
  history, downloads and settings — all in glass. Settings actions confirm
  with a small glass toast
- The address bar updates **the instant** you navigate (not after the page loads)
- Back / forward / reload appear as tiny glyphs that light up only when usable (real history state)
- `target="_blank"` links, middle-click on links and `window.open()` open a **new tab**; `Ctrl+N` opens a new window
- Zoom (`Ctrl+ +` / `Ctrl+-` / `Ctrl+0`), print (`Ctrl+P`), fullscreen (`F11`), mouse buttons 4/5 for back/forward
- Right-click context menus, F12 DevTools, hover link preview, downloads (the engine's download UI)
- **Find in page** (`Ctrl+F`) with a match counter, next / previous and highlight
- **Accessible glass UI**: semantic tabs, buttons, menus and dialogs; complete
  keyboard tab/menu navigation, screen-reader loading announcements, visible
  focus, reduced-motion and forced-colors support, plus optional larger controls
- High-DPI aware (crisp on any monitor, follows the window between screens); the bar reflows correctly at **any window size**
- Persistent profile: logins and cookies are kept in `%LOCALAPPDATA%\OKBrowser`
- **Verified in-app updates**: Settings checks GitHub for a newer release,
  asks before downloading, verifies the published SHA-256 checksum and PE
  header, asks again before restart, then safely replaces the portable EXE;
  failures leave the current executable untouched
- Proper Windows app icon, version info and GUI subsystem (no console flash)

### Keyboard shortcuts

| Keys | Action |
|---|---|
| `Ctrl+L` / `Ctrl+K` / `Ctrl+E` / `Alt+D` | Focus the address bar (selects all) |
| `Enter` | Navigate to what you typed |
| `Alt+←` / `Alt+→` | Back / forward |
| Mouse button 4 / 5 | Back / forward |
| `F5` / `Ctrl+R` | Reload |
| `Alt+Home` | Start page |
| `Ctrl+T` | New tab (and focuses the address bubble) |
| Hover the bottom bubble | Expand the address bar |
| `Ctrl+N` | New window |
| `Ctrl+W` | Close tab (window when last tab) |
| `Ctrl+Shift+W` | Close window |
| `Ctrl+Shift+T` | Reopen last closed tab |
| `Ctrl+Tab` / `Ctrl+Shift+Tab` (or `Ctrl+PgDn` / `Ctrl+PgUp`) | Next / previous tab |
| `Ctrl+1` … `Ctrl+8` | Switch to tab 1 … 8 |
| `Ctrl+9` | Switch to the last tab |
| Middle-click a tab | Close tab |
| `Ctrl+F` | Find in page (`Enter` / `Shift+Enter` next / previous, `F3` / `Ctrl+G` / `Ctrl+Shift+G` also cycle, `Esc` closes) |
| `Ctrl+D` | Bookmark this page |
| `Ctrl+H` | History |
| `Ctrl+Shift+O` | Bookmarks |
| `Ctrl+J` | Downloads |
| `Ctrl+Shift+N` | New incognito window |
| `Ctrl+Shift+R` / `Shift+F5` | Hard reload (bypasses cache) |
| `F12` / `Ctrl+Shift+I` | DevTools |
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
- Web content is rendered by the **system's** WebView2 runtime — the engine is not shipped in the exe (that's why it's only ~4 MB) and is kept updated/patched by Windows Update
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
