# 3. Frontend toolchain

- Status: accepted
- Date: 2026-09-13

## Context

The engine already writes static web pages, and `internal/api` already serves an
archive over loopback HTTP. The desktop application needs an interface on top of
that: something a person can open, search, scroll and read, on a machine that may be
offline and holding the only copy of a conversation with somebody who has died.

Two facts shape every decision below. The first is rule 1: nothing this program does
reaches the network. In Go that is provable by reading the imports. In a browser it
is not, because a single attribute in a single file is enough to break it. The second
is size: a real archive here is 1,121,482 messages, and one conversation in it runs to
roughly 90,000. Neither the DOM nor a naive data layer survives that.

## React 19 and TypeScript, strict

**Decision.** React 19 with TypeScript in strict mode, built by Vite 7.

**Why.** The interface is a long list, a search box and a reader — a shape React is
good at and has the deepest pool of solved problems for. TypeScript earns its place
because the interesting mistakes here are about absence rather than logic: a chat with
no messages, a message with no text, a contact with no name, a media item whose file
was never in the backup. Strict mode makes the type system say which of those can
happen, and the lint configuration is type-checked so the linter can see it too.

**Consequence.** A Node toolchain is needed to develop the interface, which somebody
building only the engine should not have to install. That is settled below.

## Tailwind 4, and shadcn-style components copied in

**Decision.** Tailwind 4 for styling. Components in the shadcn/ui style are copied
into `web/src/components` as source, not installed from a component package.

**Why.** Tailwind 4 compiles to a stylesheet containing only the classes the build
actually used, with no runtime and no external font. A component library installed
from npm is a dependency that decides for us what a message bubble does on a narrow
screen, and that we would fight for the rest of the project's life. Copied source is
ours: it can be read in review, fixed in place, and made accessible without waiting
for anybody. This also follows the standard-library-first rule in ADR 1 — the same
argument, applied to a different package manager.

**Consequence.** Upstream fixes do not arrive on their own. The trade is deliberate:
a few hundred lines we own beat a dependency we cannot change.

## Every asset is vendored; no CDN and no web font

**Decision.** Fonts, icons and images are part of the build. No stylesheet, script,
font or image is loaded from another host, and there is no analytics of any kind.

**Why.** A web font from a font host is a network request that tells that host
somebody is reading a WhatsApp archive, when, and from which address. A script from a
content delivery network is worse: it can change tomorrow, in the middle of a page
holding somebody's private conversations. "Zero network" that stops at the Go boundary
is not zero network — the browser is part of this program.

**Consequence.** `web/eslint.config.js` makes this mechanical rather than a matter of
remembering. `no-restricted-syntax` and `no-restricted-globals` reject absolute URLs
in `fetch`, `XMLHttpRequest`, `WebSocket`, `EventSource`, `sendBeacon`, remote dynamic
imports, and any JSX `src` or `href` pointing at another host. Each message says what
the request would cost rather than only that it is banned. The rules apply to test
files too, because a test is the first place somebody reaches for a real URL. What the
linter cannot see is CSS: an `@import url(...)` in a stylesheet is not JavaScript, so
that case stays covered by the same test the exporter already has — the built output
is asserted to contain no external reference.

`dangerouslySetInnerHTML` is banned outright, with `innerHTML`, `outerHTML` and
`insertAdjacentHTML` beside it. A message is text somebody else wrote, years ago, with
no idea it would ever be rendered in a browser. React escapes text by default, and
there is no feature here worth turning that off for.

## TanStack Query and TanStack Virtual, and nothing else at runtime

**Decision.** The only non-UI runtime dependencies are `@tanstack/react-query` and
`@tanstack/react-virtual`.

**Why.** A conversation of 90,000 messages cannot be in the DOM. Virtualisation is not
a performance improvement here, it is the difference between the page working and the
tab dying, and it is far more subtle than it looks once rows have different heights,
because a message can be a line or a paragraph. React Query exists for the same reason
in the other direction: paging, caching and invalidating requests against the loopback
API by hand is a few hundred lines of stale-data bugs, and stale data in this
application looks exactly like data loss to the person reading it. Both are small,
have no transitive network behaviour, and do one thing.

**Consequence.** No state management library, no router until there is a second
screen that needs one, no date library. Anything else added at runtime needs its own
record here, as ADR 1 requires of the Go side.

## `dist/` is committed

**Decision.** The built interface, at `internal/viewer/dist`, is committed to the repository and is
not ignored.

**Why.** The Go binary embeds it. Somebody who clones this repository to build the
engine, audit the crypt15 implementation or package it for a distribution should get a
working binary from `go build ./...` alone, with no Node, no npm and no network. That
is what the AGPL promise in ADR 1 is worth in practice: an engine anybody can build.
Making the build depend on a second toolchain would quietly narrow that to people who
already have one.

**Consequence.** Neither `.gitignore` excludes it, and both say so. Diffs
contain generated files, which is noise, and a stale `dist/` is a real hazard: the
build must be rerun and committed with any change under `web/src`. The noise is the
price of the property, and it is the smaller of the two costs.

## Quality gates

**Decision.** The `web` job in CI blocks on a typecheck, ESLint with no warnings
allowed, tests at 85% line coverage, a clean build, and a React Doctor score of at
least 85.

**Why.** The 85 is the same number the Go module already has to hit, so there is one
threshold to remember rather than two to argue about. ESLint runs the type-checked
strict and stylistic presets, the React Hooks plugin including its React Compiler
rules, and `eslint-plugin-jsx-a11y`.

Accessibility is in that list as a gate and not a nicety, and the reason is specific
rather than general. People come to this software at the worst moments of their lives:
a bereavement, a court case, a phone that died with the only record of something on
it. Some of them read with a screen reader every day. Some are doing it at four in the
morning with their hands shaking. An interface that needs a mouse, or that renders a
button as an unlabelled `div`, fails exactly the person this exists for.

**Consequence.** React Doctor has no minimum-score flag of its own — its `--blocking`
gate fires on diagnostic severity — so the score gate is a shell comparison against
`--score`, in the same shape as the coverage gates. The score is computed by
react.doctor's API, and `--no-telemetry` is an alias for `--no-score`, so the number
cannot be had without the request; rule names and counts leave the CI runner, code and
findings do not. That is a decision about a build machine, not about anything anybody
installs, and it is recorded here rather than left for somebody to discover.

## The frontend is a nested Go module

**Decision.** `web/go.mod` exists, declares a module nothing imports, and contains no
Go code. The build output goes to `internal/viewer/dist` rather than beside its own
source.

**Why.** `go build ./...` walks `node_modules`, and npm dependencies occasionally ship
Go packages of their own: one in this tree does today. Without the marker those get
built, tested and linted along with this project, which is noise at best and a way for
somebody else's code to break this build at worst. A nested module is excluded from the
parent's package patterns, which is exactly what is wanted. The output moves into the
Go tree because `embed` cannot reach across that boundary, and because a Go package
should not have to look inside a directory the Go tool is deliberately kept out of.

**Consequence.** Two directories instead of one, and a `go.mod` that looks odd until
its comment is read. Building the frontend writes outside its own directory, which is
stated in `vite.config.ts` where somebody changing the output path will see it.
