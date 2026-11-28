#!/usr/bin/env bash
# Build a complete macOS app from source.
#
# `go build ./...` in shell/mygo produces a binary without an embedded frontend;
# only the pinned MyGo CLI generates the embed overlay. Never use that binary
# for packaging.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

echo "== preflight =="
test -f frontend/index.html || { echo "missing frontend/index.html" >&2; exit 1; }
test -f shell/mygo/mygo.json || { echo "missing shell/mygo/mygo.json" >&2; exit 1; }
test -f shell/mygo/resources/icon.png || { echo "missing production icon" >&2; exit 1; }
test -f shell/mygo/resources/icon-dev.png || { echo "missing dev icon" >&2; exit 1; }
node --version >/dev/null
go version >/dev/null

echo "== frontend =="
npm ci
npm run build
test -f frontend-dist/index.html || { echo "frontend build produced no index.html" >&2; exit 1; }

echo "== pinned MyGo CLI =="
bin="$(mktemp -d)"
trap 'rm -rf "$bin"' EXIT
GOBIN="$bin" go install github.com/egoist/mygo/cmd/mygo@v0.10.0
mygo="$bin/mygo"

echo "== app bundle =="
(cd shell/mygo && "$mygo" build)

echo "== verify =="
app="build/darwin-arm64/Sasmita.app"
test -f "$app/Contents/Resources/AppIcon.icns" || { echo "missing AppIcon.icns" >&2; exit 1; }
test "$(plutil -extract CFBundleIconFile raw "$app/Contents/Info.plist")" = "AppIcon.icns" || {
  echo "Info.plist does not point to the app icon" >&2
  exit 1
}
hdiutil verify build/darwin-arm64/*.dmg >/dev/null
echo "OK: build/darwin-arm64/ contains Sasmita.app and a verified DMG"
