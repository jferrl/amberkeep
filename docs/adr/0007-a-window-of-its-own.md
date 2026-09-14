# 7. A window of its own, in a module of its own

- Status: accepted
- Date: 2026-09-14

## Context

`amberkeep serve` opens a port on the loopback address, prints a URL, and opens the
system browser. That is a good way to ship a viewer and a poor way to ship an
application. The person this program is for has a dead phone and no terminal; asking
them to keep one open, and to understand that the browser tab is the program, is
asking them to hold a shape in their head that the program should be holding for them.

It is also the reason the worst sentence in the product existed. Every path the wizard
needs is something on the person's own disk, and in a browser the only honest way to
ask is a text field. The iPhone backup folder is now picked from a list the program
builds itself, but the Android database, the pairing file and the address book are
still typed. A browser cannot open a file picker on the user's behalf. A window can.

The plan written at the start of this project deferred a native shell to after launch,
on the grounds that the HTTP API is the only UI contract and a wrapper could be added
later without touching business logic. That reasoning still holds. What changed is
that the wizard is now finished, so there is something worth wrapping.

## Decision

A Wails v2 application in `desktop/`, which is **a separate Go module**.

The separation is the whole decision. Wails needs cgo on macOS and Linux, and a
program that needs cgo cannot be cross-compiled from one laptop to six platforms.
Keeping it in its own module means:

- `cmd/amberkeep` stays a single static binary, `CGO_ENABLED=0`, cross-compiling to
  every platform from anywhere. That is the artefact people download and it does not
  change at all.
- `go build ./...` and `go test ./...` at the repository root never touch Wails, so
  the Linux CI job — which has no WebKitGTK development packages and should not need
  them — is unaffected.
- The root `go.mod` gains nothing. Checked, not assumed: no Wails module reaches it.

A module under the same path prefix may import `internal/`, because Go's rule is about
import paths rather than modules. `github.com/jferrl/amberkeep/desktop` is inside the
tree rooted at the parent of `internal`, so the window reaches the same engine the
command does.

The window is served the whole `api.Server` through Wails' asset server:

```go
AssetServer: &assetserver.Options{Handler: server}
```

With `Assets` left nil every GET falls through to that handler, which already serves
the built pages as well as the API. So the page talks to Amberkeep exactly as it does
in a browser — same addresses, same shapes, one contract — and nothing in `web/` knows
which of the two it is running in. There is no second implementation of anything, and
no port: the webview calls the handler in-process.

That also removes the launch secret rather than weakening it. The secret exists
because `serve` listens on a port that anything else on the machine could reach.
Nothing is listening here, so there is no port to guess and no request to forge, and
`Token` is deliberately empty.

The file pickers are the point of the exercise. A `Picker` struct is bound into the
page, and `web/src/lib/desktop.ts` feature-tests for it at run time rather than at
build time: one bundle, both front doors, and a browser gets the text field it always
had instead of a button that does nothing. The bindings are checked rather than
asserted, so a future version of the window that binds something different degrades
to typing rather than throwing.

## Consequences

The desktop application must be built on the platform it runs on — but only on macOS.
Wails reaches Windows through WebView2 with no cgo at all, so the Windows window
cross-compiles from a Mac like everything else. Linux is not built: Apple ships no
Finder, iTunes or Apple Devices there, so the window could not restore a backup and
the command already does everything on Linux that a window would.

Three things about building it are not obvious and cost an afternoon between them:

- **`-tags desktop,production` is not optional.** Without them Wails compiles to a
  binary that starts, prints "Wails applications will not build without the correct
  build tags", and exits. A build that produces something unrunnable is worse than one
  that fails, so CI passes the tags and the bundle script does too.
- **macOS needs the `.app` bundle to have a window at all.** A bare executable is an
  accessory process: no Dock icon, cannot be brought to the front, and the window it
  opens never appears. `build/bundle.sh` assembles the bundle with nothing but `go
  build` and a copy; the `wails` CLI is not required anywhere in this repository.
- **Wails 2.16 does not link `UniformTypeIdentifiers`** although it uses `UTType` for
  dialog filters, which fails to link against the macOS 15 SDK. One cgo line in
  `link_darwin.go` names the framework; it is inert elsewhere and disappears when
  Wails fixes it upstream.

Nothing here is signed or notarised. That remains deliberate and unchanged.

## Alternatives considered

**Wails v3.** Still in beta (`v3.0.0-beta.22` at the time of writing). For the shell
around somebody's entire message history, the stable line wins.

**Letting Wails serve the frontend and binding Go methods for everything.** This is
the conventional Wails layout and it would mean rewriting the page's API layer away
from `fetch`, forking it from the one `serve` uses. Two implementations of the same
requests is exactly what the seam in `internal/api` exists to prevent.

**A build tag instead of a separate module.** It would keep one module, and it would
put Wails in the root `go.mod` where `go build ./...` and the Linux CI job would have
to reckon with it. The module boundary is what makes the cgo cost stay in the one
place that pays it.

**Shelling out to `osascript` and PowerShell for file dialogs, staying in the
browser.** Cheaper, and it would have delivered most of the practical benefit without
cgo. It is also a per-platform shell-out on the path where somebody hands over their
entire history, and it still leaves them with a browser tab and a terminal.
