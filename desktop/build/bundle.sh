#!/usr/bin/env bash
# Assembles Amberkeep.app around the built binary.
#
# A bare executable on macOS is an accessory process: it has no Dock icon, it cannot
# be brought to the front, and the window it opens may never appear at all. What
# makes it an application is this directory layout and the Info.plist inside it, so
# the bundle is not packaging for later — it is the difference between a window and
# no window.
#
# Deliberately a script rather than the wails CLI. Nothing here needs a tool that is
# not already required to build the program.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="${1:-$here/Amberkeep.app}"

rm -rf "$out"
mkdir -p "$out/Contents/MacOS" "$out/Contents/Resources"
cp "$here/build/darwin/Info.plist" "$out/Contents/Info.plist"

# The icon, rendered from the brand mark rather than committed as a binary. It needs
# a browser to turn SVG into PNG, which the frontend already has; without one the
# bundle still builds and simply has no icon, so a machine that cannot render is not
# a machine that cannot build.
iconset="$(mktemp -d)/amberkeep.iconset"
if [ -d "$here/../web/node_modules/@playwright/test" ]; then
	(cd "$here/../web" && node scripts/app-icon.mjs "$here/../docs/brand" "$iconset")
	iconutil -c icns "$iconset" -o "$out/Contents/Resources/icon.icns"
	echo "icon: $out/Contents/Resources/icon.icns"
else
	echo "icon: skipped (no playwright in web/; run npm install there)" >&2
fi

CGO_ENABLED=1 go build -C "$here" -tags desktop,production -trimpath \
	-ldflags "-s -w" -o "$out/Contents/MacOS/amberkeep" ./...

echo "built $out"
