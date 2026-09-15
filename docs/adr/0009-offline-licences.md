# 9. A licence is a signed statement, not a question asked of a server

Date: 2026-09-15

## Status

Accepted. Built and inert: `licence.Sells()` answers no, and while it does every part
of the program is free.

## Context

The plan is to sell the application once, for about 29 euros, with the engine staying
AGPL-3.0 and free. Two things then cost money — writing the archive out, and moving a
history onto an iPhone — and something has to decide whether a given computer has paid
for them.

Every ordinary way of doing that is a network call: a licence server, an activation, a
periodic check. This program makes no network calls at all. That is not a preference,
it is the first of the four guarantees the whole design is arranged around, there is a
browser test that fails the build if the page asks for anything outside the machine it
is served from, and it is most of the reason somebody is willing to point this at their
entire private correspondence.

A licence check would be the one exception. Exceptions to that rule are how a program
that promised silence starts talking.

## Decision

A licence is a **signed statement, verified offline**.

The seller holds an Ed25519 private key. A licence is a short JSON statement — what it
covers, the day it was issued, the last day of the twelve months of updates, and the
order reference — signed with that key and handed to the buyer as one pasteable line.
The program carries the public half and verifies the signature itself, with no network
at purchase or ever after.

- **`licence.Sells()`** is the one place that says whether there is a shop. It answers
  no today, every gate is open, and turning it on is that function plus a screen.
- **`licence.Allows(grant)`** is the one question a gate asks. There are four call
  sites: the export and the migration, in the command line and in the server.
- **The key is kept** in the operating system's own settings directory, one file, owner
  only. Not in the workspace: an archive folder gets copied to an external disk and
  handed to a relative, and the licence belongs to the buyer rather than to the archive.
- **No personal data is in a key.** No name, no email. An order reference, so a lost key
  can be reissued, and nothing else.
- **Twelve months of updates, then the build you had keeps working.** The statement
  carries the last covered day and the build carries the day it was made; an older
  build is never judged against a newer licence, and a build that does not say when it
  was made — which is every build somebody compiled themselves — is always covered.

## Consequences

**A key can be copied, and the program will never know.** Two people with the same key
both pass. This is accepted rather than solved: counting machines needs a server, which
is the thing being avoided, and the honest deterrent is that most people would rather
pay 29 euros than be the sort of person who does not.

**A key can be removed entirely**, because the engine is AGPL-3.0 and anybody can
compile it without the gate. That is the licence working as intended. What is sold is
the application, the signing, the support and the updates — not the capability, which
was always free to anybody willing to build it.

**A lost private key is every licence reissued.** It never enters this repository and
the tool that uses it, `cmd/licencer`, is not part of the released program.

**Refunds are a merchant question, not a program one.** Nothing here can revoke a key,
which is a consequence of having no server: a refunded licence goes on working. Thirty
days, no questions, and the arithmetic of somebody refunding and keeping a 29-euro tool
is not worth a server and a permanent network connection to prevent.
