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

# The program first. The icon is worth having and is not worth failing over, so
# anything that can go wrong with it goes wrong after there is something to run.
CGO_ENABLED=1 go build -C "$here" -tags desktop,production -trimpath \
	-ldflags "-s -w" -o "$out/Contents/MacOS/amberkeep" ./...

# The icon is rendered from the brand mark rather than committed as a binary, which
# needs a browser — the frontend's, since it already has one. A machine without it
# still builds a working application; it just has the generic icon.
#
# node is looked for rather than assumed: it arrives through a version manager on
# most machines and is on nobody's PATH by default.
node="$(command -v node || true)"
if [ -z "$node" ]; then
	for candidate in "$HOME"/.local/share/fnm/node-versions/*/installation/bin/node \
		"$HOME"/.nvm/versions/node/*/bin/node /opt/homebrew/bin/node /usr/local/bin/node; do
		[ -x "$candidate" ] && node="$candidate" && break
	done
fi

if [ -z "$node" ]; then
	echo "icon: skipped — no node found, so the mark could not be rendered" >&2
elif [ ! -d "$here/../web/node_modules/@playwright/test" ]; then
	echo "icon: skipped — run npm install in web/ to render the mark" >&2
else
	iconset="$(mktemp -d)/amberkeep.iconset"
	(cd "$here/../web" && "$node" scripts/app-icon.mjs "$here/../docs/brand" "$iconset")
	iconutil -c icns "$iconset" -o "$out/Contents/Resources/icon.icns"
	echo "icon: $out/Contents/Resources/icon.icns"
fi

# An ad-hoc signature over the whole bundle.
#
# This is not the code signing this project has deliberately not done: there is no
# certificate, no Apple Developer account, no identity and no notarisation. It is the
# minimum that makes the bundle internally consistent — without it the only signature
# present is the one Go's linker puts on the inner binary, the bundle's own seal is
# missing, and macOS refuses even to assess it: "code has no resources but signature
# indicates they must be present".
#
# A downloaded copy is still quarantined and still refused, because an ad-hoc
# signature proves nothing about who built it. That is what Developer ID and
# notarisation are for, and until they are done the README says how to clear the
# quarantine by hand.
codesign --force --deep --sign - "$out"
echo "signed: ad-hoc, no identity — see README on Gatekeeper"

echo "built $out"
