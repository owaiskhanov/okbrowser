# Smoke test: launches OKBrowser.exe on a real Windows machine and verifies
# that (1) the window appears, (2) the WebView2 engine starts, and (3) a REAL
# navigation succeeds - the app is launched with https://example.com and the
# window title must become the page title. Writes a diagnostics report next
# to the exe so CI can publish it even when this script fails.
param(
    [string]$Exe = (Join-Path $PSScriptRoot "..\dist\OKBrowser.exe"),
    [string]$DiagFile = ""
)

$ErrorActionPreference = "Continue"
if ($DiagFile -eq "") { $DiagFile = Join-Path (Split-Path $Exe -Parent) "smoke-diagnostics.txt" }
$diag = New-Object System.Collections.Generic.List[string]
function Log($m) { Write-Host "[smoke] $m"; $diag.Add("$m") }

$fail = ""
$proc = $null
$errFile = Join-Path (Split-Path $Exe -Parent) "smoke-stderr.txt"
try {
    if (-not (Test-Path $Exe)) { throw "Executable not found: $Exe" }
    $full = (Resolve-Path $Exe).Path
    Log "launching $full https://example.com"
    Log ("OS: " + [System.Environment]::OSVersion.VersionString)

    # Report the installed WebView2 runtime from the registry.
    $regPaths = @(
        "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}",
        "HKCU:\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
    )
    foreach ($rp in $regPaths) {
        if (Test-Path $rp) {
            $pv = (Get-ItemProperty $rp -ErrorAction SilentlyContinue).pv
            Log "WebView2 runtime registry ($rp): pv=$pv"
        } else {
            Log "WebView2 runtime registry missing: $rp"
        }
    }

    $outFile = Join-Path (Split-Path $Exe -Parent) "smoke-stdout.txt"
    $proc = Start-Process -FilePath $full -ArgumentList "https://example.com" -PassThru -RedirectStandardError $errFile -RedirectStandardOutput $outFile
    $sw = [System.Diagnostics.Stopwatch]::StartNew()

    # Phase 1: window appears.
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

    # Phase 2: the real navigation must complete - the window title becomes
    # the page title ("Example Domain"). This is the end-to-end test that a
    # URL actually loads and reports back.
    $navOk = $false
    $deadline = (Get-Date).AddSeconds(35)
    while ((Get-Date) -lt $deadline) {
        if ($proc.HasExited) { $fail = "process exited during navigation (code $($proc.ExitCode))"; break }
        $proc.Refresh()
        $title = $proc.MainWindowTitle
        if ($title -like "*Example Domain*") {
            $navOk = $true
            Log "navigation OK after $($sw.ElapsedMilliseconds) ms, title: '$title'"
            break
        }
        Start-Sleep -Milliseconds 300
    }
    if (-not $navOk -and $fail -eq "") {
        $fail = "navigation to https://example.com did not complete; title stayed: '$title'"
    }

    Start-Sleep -Seconds 2
    $proc.Refresh()
    if ($proc.HasExited -and $fail -eq "") { $fail = "process exited after startup (code $($proc.ExitCode))" }

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
Log "PASS: window, web engine and real navigation all OK."
$diag | Set-Content -Encoding UTF8 $DiagFile
exit 0
