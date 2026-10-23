#!/bin/bash
# Build the M0 prototype on a Mac.
# Prefers `wails build` (produces DeskCanvas.app); falls back to a plain
# `go build` binary, which runs the same code and is enough for M0 —
# the window-layer behaviour does not depend on the bundle.
set -euo pipefail
cd "$(dirname "$0")"

if command -v wails >/dev/null 2>&1; then
  wails build -clean
  echo ""
  echo "Built: build/bin/DeskCanvas.app"
  echo "Run:   open build/bin/DeskCanvas.app"
else
  go build -o build/bin/deskcanvas .
  echo ""
  echo "Built: build/bin/deskcanvas (no wails CLI found, plain binary)"
  echo "Run:   ./build/bin/deskcanvas"
  echo "Tip:   install the CLI with: go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0"
fi
