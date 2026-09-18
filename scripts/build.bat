@echo off
rem Builds OKBrowser.exe on Windows. Only Go itself is required.
cd /d "%~dp0.."
setlocal
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
if not exist dist mkdir dist
go build -trimpath -ldflags "-s -w -H windowsgui" -o dist\OKBrowser.exe .\cmd\okbrowser
if errorlevel 1 exit /b 1
for %%A in (dist\OKBrowser.exe) do echo Built dist\OKBrowser.exe ^(%%~zA bytes^)
