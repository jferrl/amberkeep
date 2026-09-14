# 5. The recorded replies

- Status: accepted
- Date: 2026-09-14

## Context

`web/src/api/types.ts` is written by hand and nothing checks it at runtime. That is
deliberate and it is stated at the top of the file: the program that serves the page
is the program that built it, so a reply that does not match those types is a bug in
this repository rather than something a browser should defend against. Adding a
schema validator to the client would cost bundle size and startup time to guard
against a class of failure that cannot happen in a shipped binary.

It is the right trade right up until the two halves drift apart, and during the
import wizard they did, three times over, in one afternoon:

- `Backup.last_backup` became RFC 3339 in universal time on the Go side while the
  page still expected something already formatted.
- `BackupList.backups` is never null and was written down as possibly null.
- `Backup.encrypted` is always sent, including when it is false, and was written down
  as optional — which would have made an encrypted backup indistinguishable from one
  whose encryption was unknown, on the one field that decides whether a backup can be
  opened at all.

None of the three was visible. The types compiled, the tests passed, and the page
would have been wrong the first time somebody ran it.

## Decision

`internal/api/contract_test.go` records every reply the page can meet — the archive,
a page of conversations, a page of messages, a message carrying every optional field
at once, search results, the four wizard states, and both answers to a request for
the backups — from the real handlers, over a real archive, with a real search index.
`web/src/api/contract.test.ts` then assigns each recording to the type the page
declares.

The recordings are generated. After changing a handler:

    go test ./internal/api -run TestTheRecordedRepliesStillMatch -update

and the diff in `web/src/api/contract/` is the page's contract changing. It belongs
in the same commit as the change that caused it.

### They are TypeScript, not JSON

A JSON import widens every string to `string`, which would leave the unions that
matter most — a conversation's kind, a message's kind, the wizard's stage and step —
entirely unchecked. `as const` keeps them literal, so a kind this server invents and
the page has never heard of is a compile error rather than a surprise at runtime.

The cost is that a Go test writes a TypeScript file. It says so at the top of every
recording.

### Both directions, and where each is caught

The constraint on the recording catches everything missing or of the wrong type, at
every depth. A second, phantom parameter catches the other direction: it exists only
when a recording carries a field the page never declared, so the compiler says an
argument for `theServerAlsoSends` was not provided. Which field that is, is in the
recording's own diff, in the same commit.

All four drift directions were checked against the unfixed code before this was
called done: a field removed, a field's type changed, a field added, and a union
member the page had never heard of.

### One recording carries everything

No real message carries a poll and a call and a deletion at once, and
`message-with-everything.ts` does. A recording only checks the fields that are in it,
so a field nothing exercises would be free to be wrong; one message that carries all
of them costs one recording instead of twenty. A test on each side lists the optional
fields of a message and fails if any of them stops appearing.

## Consequences

- The Go tests now write into `web/`. That is the one place the two halves meet, and
  the alternative — putting the recordings outside the page's own tree — means
  fighting the bundler's file-system rules for no benefit.
- This is a check, not a runtime guard. The page still validates nothing at runtime
  and still should not: the failure it would catch cannot survive a build.
- Anybody changing a response shape now has to say so twice, once in Go and once in
  TypeScript. That is the point.
