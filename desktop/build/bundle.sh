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

CGO_ENABLED=1 go build -C "$here" -tags desktop,production -trimpath \
	-ldflags "-s -w" -o "$out/Contents/MacOS/amberkeep" ./...

echo "built $out"
