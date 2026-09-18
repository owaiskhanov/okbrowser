#!/usr/bin/env bash
# Cross-compiles OKBrowser.exe for Windows (x64) from Linux or macOS.
# Nothing but Go itself is required - all dependencies are vendored and the
# Windows resource file (icon/manifest/version) is committed to the repo.
set -euo pipefail
cd "$(dirname "$0")/.."

export CGO_ENABLED=0 GOOS=windows GOARCH=amd64
mkdir -p dist
go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/OKBrowser.exe ./cmd/okbrowser

echo "Built dist/OKBrowser.exe ($(du -h dist/OKBrowser.exe | cut -f1 | tr -d ' '))"
