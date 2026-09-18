# Smoke test: launches OKBrowser.exe on a real Windows machine and verifies
# that the window appears, the title is right, the WebView2 engine starts and
# the process stays alive. Writes a diagnostics report next to the exe so CI
# can publish it even when this script fails.
param(
    [string]$Exe = (Join-Path $PSScriptRoot "..\dist\OKBrowser.exe"),
    [string]$DiagFile = ""
)

$ErrorActionPreference = "Continue"
if ($DiagFile -eq "") { $DiagFile = Join-Path (Split-Path $Exe -Parent) "smoke-diagnostics.txt" }
$diag = New-Object System.Collections.Generic.List[string]
function Log($m) { Write-Host "[smoke] $m"; $diag.Add("$m") }

$fail = ""
try {
    if (-not (Test-Path $Exe)) { throw "Executable not found: $Exe" }
    $full = (Resolve-Path $Exe).Path
    Log "launching $full"
    Log ("OS: " + [System.Environment]::OSVersion.VersionString)

    # Report the installed WebView2 runtime from the registry.
    $regPaths = @(
        "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}",
        "HKCU:\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
    )
    $found = $false
    foreach ($rp in $regPaths) {
        if (Test-Path $rp) {
            $pv = (Get-ItemProperty $rp -ErrorAction SilentlyContinue).pv
            Log "WebView2 runtime registry ($rp): pv=$pv"
            if ($pv) { $found = $true }
        } else {
            Log "WebView2 runtime registry missing: $rp"
        }
    }
    if (-not $found) { Log "WARNING: no WebView2 runtime version found in registry" }

    $errFile = Join-Path (Split-Path $Exe -Parent) "smoke-stderr.txt"
    $outFile = Join-Path (Split-Path $Exe -Parent) "smoke-stdout.txt"
    $proc = Start-Process -FilePath $full -PassThru -RedirectStandardError $errFile -RedirectStandardOutput $outFile
    $sw = [System.Diagnostics.Stopwatch]::StartNew()

    $sawWindow = $false
    $title = ""
    $deadline = (Get-Date).AddSeconds(30)
    while ((Get-Date) -lt $deadline) {
        if ($proc.HasExited) {
            Log "process exited early after $($sw.ElapsedMilliseconds) ms, code $($proc.ExitCode)"
            throw "FAIL: process exited early (code $($proc.ExitCode))"
        }
        $proc.Refresh()
        if ($proc.MainWindowHandle -ne 0) {
            $sawWindow = $true
            $title = $proc.MainWindowTitle
            Log "main window up after $($sw.ElapsedMilliseconds) ms, title: '$title'"
            break
        }
        Start-Sleep -Milliseconds 250
    }
    if (-not $sawWindow) { $fail = "no main window appeared within 30 seconds" }

    if ($title -and $title -notlike "*OK Browser*") { $fail = "unexpected window title '$title'" }

    # Give the engine time to spin up fully, then confirm stability.
    Start-Sleep -Seconds 5
    $proc.Refresh()
    if ($proc.HasExited) { $fail = "process exited after startup (code $($proc.ExitCode))" }

    $wv = @(Get-Process msedgewebview2 -ErrorAction SilentlyContinue)
    Log "WebView2 engine processes: $($wv.Count)"
    if ($wv.Count -eq 0 -and $fail -eq "") { $fail = "no WebView2 engine processes started" }
}
catch {
    $fail = $_.Exception.Message
}
finally {
    if ($proc -and -not $proc.HasExited) {
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        Log "stopped test process"
    }
    if (Test-Path $errFile) {
        $errText = (Get-Content $errFile -Raw -ErrorAction SilentlyContinue)
        if ($errText) { Log "STDERR: $errText" } else { Log "STDERR: (empty)" }
    }
}

if ($fail -ne "") {
    Log "FAIL: $fail"
    $diag | Set-Content -Encoding UTF8 $DiagFile
    exit 1
}
Log "PASS: window, title and web engine all OK."
$diag | Set-Content -Encoding UTF8 $DiagFile
exit 0
