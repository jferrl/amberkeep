# Working on Amberkeep

Amberkeep reads, searches and exports somebody's own WhatsApp history from backups
they already have, on their own machine. The engine is open source; a desktop
application will be built on top of it.

Read [docs/PRINCIPLES.md](docs/PRINCIPLES.md) before writing anything. It is binding,
not advisory. This file is the short version plus the things that are easy to get
wrong here.

## The four rules that are never traded away

1. **Zero network.** Nothing in this program makes a network call. There is no
   telemetry, no update check, no analytics, and no CDN in any page it writes.
   A test asserts that exported pages contain no external reference.
2. **Originals are never modified.** Sources are opened read-only. The one exception
   is `prepare`, which adds indexes to a decrypted database somebody names, and it
   says so before it does. A test hashes every row of every table to prove nothing
   else changes. Reading a file must not even leave a `-wal` beside it.
3. **Secrets never reach logs or disk.** A decryption key lives in a type whose every
   rendering redacts it, and no error message ever repeats what was passed in.
4. **Destructive actions need a dry run and a typed confirmation.** Nothing here
   writes to a phone yet; when it does, this is how.

## What the code is like

- **Google Go Style Guide first**, Uber's where Google is silent. Google wins on conflict.
- **Tell, don't ask.** A type owns its invariants and exposes behaviour. `Message`
  answers `IsFromMe()`, `HasText()`, `Displayable()`; callers never compare its
  `SourceType` to a number.
- **Comments say why, never what.** If a comment restates the code, delete it. If a
  line looks wrong and is not, that is what a comment is for. Numbers that were
  measured belong in the comment beside the thing they justify.
- **Introspection first.** WhatsApp renames and removes columns every few months.
  Never assume a column exists: ask the schema and adapt. Every reader does this and
  every reader must keep doing it.
- **Errors carry a stable guidance identifier.** Each package has one typed error set
  whose `Is` matches on that identifier. `cmd/amberkeep/guidance.go` turns each into
  several lines of advice, and a test fails the build if an error is added without
  any. Nobody should have to search the internet to get past a problem we understand.
- **Unrecognised is not the same as dropped.** A message of a kind this build does not
  know is carried through with its original type number and rendered as such. Never
  invent a meaning for a code no reliable source documents. Say UNKNOWN instead.

## Tests

- **Table-driven**, `t.Run` subtests, one table per behaviour, each case named for the
  rule it proves. The name is the specification.
- **Fixtures are synthetic or structure-only.** Real message content never enters this
  repository. Tests that need real data read a path from an environment variable and
  skip without it: `AMBERKEEP_REAL_MSGSTORE`, `AMBERKEEP_REAL_CHATSTORAGE`,
  `AMBERKEEP_REAL_BACKUP`, `AMBERKEEP_GOLDEN_CRYPT15`.
- **Fuzz every parser that reads untrusted bytes.** The crypt15 header, the iPhone
  reply protobuf, the MBFile plist. They read files this project did not write.
- **A regression test must be checked against the unfixed code.** Break the fix, watch
  the test fail, restore it. A regression test that would not have caught the bug is
  worse than none because it looks like cover.
- `go test ./... -race` and a coverage gate of 85% on the whole module. CI enforces both.

## Before you finish

```sh
gofmt -w . && go build ./... && go test ./... -race -count=1 && golangci-lint run ./...
```

And, if anything under `web/` changed:

```sh
cd web && npm run typecheck && npm run lint && npm test && npm run doctor && npm run build
```

The build writes into `internal/viewer/dist`, which is committed, so a viewer change
is not finished until that output is rebuilt and staged with it.

All of them clean, every time. The linter configuration is strict on purpose and its
exemptions each carry a written reason; add to that list rather than weakening a rule,
and say why in the same change.

Commits follow Conventional Commits and need a sign-off (`git commit -s`). A commit
message explains what was learned, not what was typed: the bug that was found, the
measurement that settled an argument, the thing that turned out to be wrong.

## Things that have already caught people out

- **A decrypted Android database has no indexes at all.** WhatsApp strips them from
  the backup. Every query scans the whole table until `prepare` puts them back. This
  was the difference between a five-minute export and a fifty-five-second one.
- **Do not order an iPhone conversation by date.** There is no index for it, so every
  page sorts the whole conversation. `ZSORT` is indexed with the conversation and
  agrees with the date exactly. This was fifteen seconds against a fifth of one.
- **Replies on an iPhone are not in `ZPARENTMESSAGE`.** That column is empty in every
  row of a real store. They are in a protobuf beside the message. See
  `internal/source/ios/context.go`.
- **SQLite creates a `-wal` and a `-shm` beside any database it opens in write-ahead
  mode, read-only or not.** Apple's backup index is in that mode. Opening it without
  `immutable` puts two new files inside somebody's backup.
- **Search markers cannot be the null character.** SQLite's `snippet()` builds its
  output with C string handling and silently drops it.
- **A message must never become markup.** Everything the page renders is a text
  node; the linter bans `dangerouslySetInnerHTML` and a test checks the outcome.
- **`go build ./...` walks `node_modules`.** `web/go.mod` exists only to stop it.
- **Measure before optimising, and measure again after.** A larger page cache was
  worth 25% before the indexes were restored and 2% after, for three times the memory.

## Where things are

| Path | What it holds |
|---|---|
| `internal/crypt15` | decrypting an Android backup |
| `internal/backupfs` | finding and reading iPhone backups |
| `internal/source/android` | the Android message database |
| `internal/source/ios` | the iPhone message store |
| `internal/source` | recognising which one a file is |
| `internal/model` | the shared domain types |
| `internal/contacts` | names from an address book |
| `internal/export` | text, structured data and web pages |
| `internal/search` | the full-text index |
| `internal/api` | the archive over HTTP, loopback only |
| `cmd/amberkeep` | the command-line tool |
| `web/` | the viewer: React, TypeScript, its own nested Go module marker |
| `internal/viewer` | the built viewer, embedded into the binary |
| `docs/formats/` | how each database is laid out and what is known about it |
| `docs/adr/` | decisions and why they were made |
