# Smoke test: launches OKBrowser.exe on a real Windows machine and verifies
# that the window appears, the title is right, the WebView2 engine starts and
# the process stays alive. Used by CI; also handy to run manually.
param(
    [string]$Exe = (Join-Path $PSScriptRoot "..\dist\OKBrowser.exe")
)

$ErrorActionPreference = "Stop"
if (-not (Test-Path $Exe)) { throw "Executable not found: $Exe" }
$full = (Resolve-Path $Exe).Path
Write-Host "[smoke] launching $full"

$proc = Start-Process -FilePath $full -PassThru
try {
    $sawWindow = $false
    $title = ""
    $deadline = (Get-Date).AddSeconds(30)
    while ((Get-Date) -lt $deadline) {
        if ($proc.HasExited) { throw "[smoke] FAIL: process exited early (code $($proc.ExitCode))" }
        $proc.Refresh()
        if ($proc.MainWindowHandle -ne 0) {
            $sawWindow = $true
            $title = $proc.MainWindowTitle
            break
        }
        Start-Sleep -Milliseconds 250
    }
    if (-not $sawWindow) { throw "[smoke] FAIL: no main window appeared within 30 seconds" }
    Write-Host "[smoke] main window is up, title: '$title'"
    if ($title -notlike "*OK Browser*") { throw "[smoke] FAIL: unexpected window title '$title'" }

    # Give the engine time to spin up fully, then confirm stability.
    Start-Sleep -Seconds 5
    if ($proc.HasExited) { throw "[smoke] FAIL: process exited after startup" }

    $wv = @(Get-Process msedgewebview2 -ErrorAction SilentlyContinue)
    Write-Host "[smoke] WebView2 engine processes: $($wv.Count)"
    if ($wv.Count -eq 0) { throw "[smoke] FAIL: no WebView2 engine processes started" }

    Write-Host "[smoke] PASS: window, title and web engine all OK."
    exit 0
}
finally {
    if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
}
