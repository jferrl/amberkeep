# Development principles

Binding, not advisory. Continuous integration enforces most of this; review enforces
the rest. A change that breaks one of these is wrong even if it works.

## Style and design

- **[Google Go Style Guide](https://google.github.io/styleguide/go/)** is the primary
  reference. **[Uber's Go Style Guide](https://github.com/uber-go/guide)** fills the
  gaps (error naming, struct initialisation, functional options). Google wins on conflict.
- **Tell, don't ask.** Types own their invariants and expose behaviour, not state. A
  session appends a message and renumbers itself; callers never read a counter, decide
  something, and write it back. Readers return whole domain objects. Writers accept
  commands, not bags of fields. Getter-then-decide chains are a design smell.
- **Simple layered packages** under `internal/`, with dependencies pointing inward
  only. No framework, no hexagonal ceremony. `cmd/` stays thin.
- **Errors** wrap with `%w`, one typed error set per package, no panics in library
  code. Every user-facing error carries a stable guidance identifier so the interface
  can show an explanation instead of raw text. Identifiers are part of the public
  contract: never renumber or reuse one.
- **Logging** through `log/slog` only. Key material lives in a type whose every
  rendering method redacts, and it is never marshalled.
- **Concurrency**: a `context.Context` on every input/output boundary, no
  fire-and-forget goroutines.

## Dependencies

Standard library first. Go already provides the cryptography, compression, plist-free
encoding, HTTP and embedding this project needs.

Every third-party package requires an architecture decision record explaining why the
standard library is insufficient. `CGO_ENABLED=0` always, so a single static binary
cross-compiles to macOS and Windows without a toolchain.

Currently justified: a pure-Go SQLite driver, an Apple property-list parser, and a
reader for encrypted iPhone backups. Nothing else.

## Testing

- **Table-driven by default.** A slice of cases with `t.Run` subtests, each named for
  the rule it proves rather than the input it uses.
- **Golden files** for every serialised format, refreshed behind an `-update` flag and
  reviewed in the diff like any other change.
- **Differential tests** against a reference implementation wherever one exists. The
  Python prototypes this project grew from are kept precisely for this.
- **Fuzz tests** on every parser that reads untrusted bytes.
- **Fixtures are synthetic or structure-only.** Real message content never enters the
  repository, and no test may require it to pass.
- `go test -race` in CI. Coverage gate of 85% on `internal/`, near total on any path
  that writes to a backup.

A change is done when: tests are added or extended, golden diffs are reviewed, lint is
clean, an architecture decision record exists if a decision was made, and the guidance
text is updated if user-visible behaviour changed.

## Privacy and safety

Non-negotiable. These are the product, not features of it.

- **Zero network by default.** No calls at all. Update checks and crash reporting are
  explicit opt-ins, off by default, and visible in settings.
- **Originals are never modified.** Sources are opened read-only or copied into a
  workspace the user can see and delete.
- **Secrets never touch logs or disk unencrypted.** Keys and passwords stay in memory,
  in a type that redacts itself.
- **Destructive actions require a dry run and a typed confirmation.** Anything that
  writes to a backup shows a validation report first.

## Frontend

React with TypeScript in strict mode, Vite, and shadcn/ui on Tailwind. Components are
copied into the repository rather than pulled from a content delivery network, which
keeps the zero-network guarantee intact and lets them be adapted to the guided flow.

Blocking gates in CI: [React Doctor](https://www.react.doctor/) above a minimum score,
ESLint with the React Hooks and React Compiler rules, TypeScript strict, and
accessibility checks. Component tests use the same table-driven style as the Go code.

## Workflow

- `golangci-lint` and `gofmt` block merges.
- **Conventional Commits** drive semantic versioning and a generated changelog.
- **Architecture decision records** in `docs/adr/`, in [MADR](https://adr.github.io/madr/) format.
- CI runs on macOS, Windows and Linux. Release artefacts are built, signed and
  notarised in CI only, never from a laptop.
- Contributions carry a DCO sign-off.

## Compatibility

WhatsApp changes its databases every few months. The readers are built for that:

- **Introspect, never assume.** Probe the schema and adapt; no hard-coded column sets
  or entity numbers.
- **Degrade out loud.** When something is unrecognised, say what will happen before
  doing it, and report counts of anything skipped. Never drop data silently.
- **Publish the truth.** The compatibility matrix states which combinations are
  verified, which are best-effort and which are unsupported.
