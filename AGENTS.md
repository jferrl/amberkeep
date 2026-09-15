# Working on Amberkeep

Amberkeep reads, searches and exports somebody's own WhatsApp history from backups
they already have, on their own machine. The engine is open source; a desktop
application will be built on top of it.

Read [docs/PRINCIPLES.md](docs/PRINCIPLES.md) before writing anything. It is binding,
not advisory. This file is the short version plus the things that are easy to get
wrong here.

This is the only copy. Every coding tool looks for a file of its own — `CLAUDE.md`,
and others besides — and each of those points here rather than repeating any of it,
because two copies drift and the drift is invisible until something acts on the stale
one. `.claude/agents/` stays where it is: those are subagent definitions in one tool's
own format, with nothing neutral to move them to.

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
  whose `Is` matches on that identifier. `internal/app/guidance.go` turns an error
  into one of those identifiers; `internal/guide/words/*.json` turns the identifier
  into a heading and several lines of advice, in every language, for the terminal and
  the page alike. A test fails the build if an error is added without any, or if the
  advice exists in one language and not the other. Nobody should have to search the
  internet to get past a problem we understand, in any language.
- **A schema this build has never seen is a thing to report, not to fail on.**
  `internal/canary` compares a database against the shapes in `internal/canary/corpus`
  and says what is new. Its output is table names, column names, integer codes and
  counts, and a test fails if a value ever reaches it: the report is meant to be
  pasted into a public issue by somebody whose database is their private
  correspondence. A reader that learns a new message type extends `Known()` in the
  same commit, and a test walks every code a byte can hold to make sure it did.
- **A licence is a signed statement, never a question asked of a server.** Zero
  network has no exceptions, licences included; see `docs/adr/0009-offline-licences.md`.
  `licence.Sells()` is the one place that says whether there is a shop and it answers
  no, so every gate is open. `licence.Allows(grant)` is the one question a gate asks.
  Do not add a fifth call site without a reason, and never put a name or an address in
  a key.
- **Unrecognised is not the same as dropped.** A message of a kind this build does not
  know is carried through with its original type number and rendered as such. Never
  invent a meaning for a code no reliable source documents. Say UNKNOWN instead.

## Tests

- **Table-driven**, `t.Run` subtests, one table per behaviour, each case named for the
  rule it proves. The name is the specification.
- **Fixtures are synthetic or structure-only.** Real message content never enters this
  repository. Tests that need real data read a path from an environment variable and
  skip without it: `AMBERKEEP_REAL_MSGSTORE`, `AMBERKEEP_REAL_CHATSTORAGE`,
  `AMBERKEEP_REAL_BACKUP`, `AMBERKEEP_GOLDEN_CRYPT15`, `AMBERKEEP_REAL_LIDPAIRS`,
  `AMBERKEEP_ORACLE_REPORT`, `AMBERKEEP_REAL_ORIGINAL_STORE`,
  `AMBERKEEP_REAL_MIGRATED_STORE`, `AMBERKEEP_REAL_STORE_IN`, `AMBERKEEP_REAL_INTO`.
- **The migration is checked against the Python prototype.** It is the only thing that
  has ever produced the right answer on a real phone, so it is the oracle. Point
  `AMBERKEEP_ORACLE_REPORT` at one or more of its reports, comma-separated and in the
  order they were run — a single step of a multi-step run is a partial answer, and
  comparing against one looks exactly like a bug in this code.
- **Fuzz every parser that reads untrusted bytes.** The crypt15 header, the iPhone
  reply protobuf, the MBFile plist. They read files this project did not write.
- **A regression test must be checked against the unfixed code.** Break the fix, watch
  the test fail, restore it. A regression test that would not have caught the bug is
  worse than none because it looks like cover.
- **The page's types are checked against what the server really sends.** Every reply
  is recorded from the real handlers into `web/src/api/contract/`, and
  `web/src/api/contract.test.ts` assigns each recording to the type the page declares.
  Change a handler and re-record in the same commit:

  ```sh
  go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
  ```

  The diff is the page's contract changing. Do not hand-edit a recording, and do not
  re-record to make a failure go away without reading what moved.
- `go test ./... -race` and a coverage gate of 85% on the whole module. CI enforces both.

## Before you finish

```sh
gofmt -w . && go build ./... && go test ./... -race -count=1 && golangci-lint run ./...
```

And, if anything under `web/` changed:

```sh
cd web && npm run typecheck && npm run lint && npm test && npm run doctor && npm run build
cd web && npm run e2e:build   # rebuilds the binary, then drives it in a real browser
```

The build writes into `internal/viewer/dist`, which is committed, so a viewer change
is not finished until that output is rebuilt and staged with it.

All of them clean, every time. The linter configuration is strict on purpose and its
exemptions each carry a written reason; add to that list rather than weakening a rule,
and say why in the same change.

Commits follow Conventional Commits and need a sign-off (`git commit -s`). A commit
message explains what was learned, not what was typed: the bug that was found, the
measurement that settled an argument, the thing that turned out to be wrong.

## The migration

Four pieces, built in this order on purpose: plan, then the checks that verify a
result, then the writer, then putting it into a backup. The checks came before the
thing they check, because a checker written afterwards can only be tested by running
the writer and watching it pass. Each check is tested by damaging a store in exactly
that way.

Nothing in it touches a device, and nothing in it ever will. It produces a backup
folder; restoring that is Finder's job, and `internal/guide` is what tells somebody
how. The guide is data so that somebody who is not a programmer can correct a
sentence, and so the same words reach a terminal and a browser without drifting.

`docs/adr/0006-planning-a-migration-separately-from-doing-it.md` has the reasoning,
including four faults that only real data produced.

In a browser it is the same four pieces behind `api.Migrator`, and nothing moves from
one stage to the next on its own: checking does not begin planning, a plan does not
begin writing, and writing needs the word `migrate` typed out. A page somebody can
click through without reading is the one thing this must not be.

## The import wizard

`amberkeep serve` starts with no archive. `internal/api` holds a session with four
stages — empty, working, ready, failed — and six endpoints get somebody from a
backup to an archive; `GET /api/state` is the only thing the page polls, and every
endpoint that reads an archive answers 409 until one is open. `api.Importer` is the
seam: the server knows what to ask, `cmd/amberkeep/importer.go` knows how a backup
is decrypted and where Apple hides one. Do not reimplement any of that behind the
API — call the command's own helper, or there will be two answers to what a backup
is. The contract and the reasoning are in `docs/adr/0004-the-import-wizard.md`.

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
- **The 64-digit key must never come back out.** Not in the state the page polls, not
  in an error, not in an address. Errors are the thing most likely to be pasted into
  a bug report. Two tests fail if it appears in any response; do not defeat them.
- **A header set after `WriteHeader` is silently dropped.** This is why `write` and
  `writeStatus` are one function: a 202 built the other way round arrives as text.
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
| `internal/api` | the archive over HTTP, loopback only, and the import wizard |
| `internal/guide` | what somebody has to be told, and when; data, not code |
| `internal/migrate` | working out a migration, doing it, and checking the result |
| `cmd/amberkeep` | the command-line tool |
| `web/` | the viewer: React, TypeScript, its own nested Go module marker |
| `internal/viewer` | the built viewer, embedded into the binary |
| `docs/formats/` | how each database is laid out and what is known about it |
| `docs/adr/` | decisions and why they were made |
