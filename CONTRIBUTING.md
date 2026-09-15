# Contributing

The most useful contribution is a **schema report**: the structure of a WhatsApp
database from a version nobody here has seen. WhatsApp changes its layout every few
months, and those reports are what keep the readers working for everybody else. There
is [a template](.github/ISSUE_TEMPLATE/schema-report.yml) that asks for structure and
refuses content.

## The one rule that is never bent

**No real message content, database, backup or key enters this repository.** Not in a
test, not in a fixture, not in an issue, not in a screenshot. Fixtures are synthetic or
structure-only; tests that need real data read a path from an environment variable and
skip without it. CI fails the build if a file that looks like private data is
committed, and that guard is mechanical because remembering is not good enough.

## The most useful thing you can send without writing any code

A database shape. WhatsApp changes its schema every few months and this program can
only report what it has seen, so a version nobody here has run against is worth more
than most patches:

```sh
amberkeep canary --db msgstore.db --emit \
  --whatsapp 2.26.36.1 --os "Android 16" --device SM-A566B > android-2.26.36.1.json
```

That file is table names, column names and the version you typed — the rule above
holds, and it holds mechanically: the report is built from the database's catalogue
and from counting rows, and a test fails if a value ever reaches it. Read it before
you send it. Then open an issue, or a pull request adding it to
`internal/canary/corpus/` and a row to
[docs/COMPATIBILITY_MATRIX.md](docs/COMPATIBILITY_MATRIX.md).

## Before you write anything

Read [AGENTS.md](AGENTS.md), then [docs/PRINCIPLES.md](docs/PRINCIPLES.md). They are
binding rather than advisory, and they explain the four guarantees the whole design is
arranged around: zero network, originals never modified, secrets never on disk, and
nothing destructive without a dry run first.

If you are making a decision rather than a change, write an ADR in
[docs/adr/](docs/adr/) in the same pull request.

## Running the checks

```sh
gofmt -w . && go build ./... && go test ./... -race -count=1 && golangci-lint run ./...
```

And if anything under `web/` changed:

```sh
cd web && npm run typecheck && npm run lint && npm test && npm run doctor && npm run build
cd web && npm run e2e:build
```

`npm run build` writes into `internal/viewer/dist`, which is committed, so a viewer
change is not finished until that output is rebuilt and staged with it. CI checks that
the committed build matches its source.

The linter configuration is strict on purpose and every exemption in it carries a
written reason. Add to that list rather than weakening a rule, and say why in the same
change.

## Commits

[Conventional Commits](https://www.conventionalcommits.org), and a sign-off:

```sh
git commit -s
```

The sign-off is a [Developer Certificate of Origin](https://developercertificate.org):
it says the work is yours to give under this licence. There is no separate agreement
to sign.

A commit message explains what was learned, not what was typed — the bug that was
found, the measurement that settled an argument, the thing that turned out to be
wrong. The history here is the only record of why this program is shaped the way it is.

## Licence

Contributions are under [AGPL-3.0](LICENSE), the same as everything else here.
