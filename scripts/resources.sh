#!/usr/bin/env bash
# Regenerates cmd/okbrowser/resource.syso (app icon, manifest, version info)
# from res/. Only needed when you change the icon or version.
#
# Requires goversioninfo once:
#   go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
set -euo pipefail
cd "$(dirname "$0")/../res"
exec goversioninfo -o ../cmd/okbrowser/resource.syso versioninfo.json
