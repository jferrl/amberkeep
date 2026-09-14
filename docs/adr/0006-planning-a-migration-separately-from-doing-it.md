# 6. Planning a migration separately from doing it

- Status: accepted
- Date: 2026-09-14

## Context

The Python prototype in the repository this project came from does the thing it was
written for: it merged a person's entire Android history into their iPhone's WhatsApp
store, first time, with no duplicates. It is the only implementation in existence
that is known to have produced the right answer on real data, so it is the oracle
this port is measured against.

It also works out what to do and does it in the same pass. `inject.py` opens both
sides, decides which conversation matches which, which messages are already there and
which cannot be carried across, and writes them — and only then prints what it did.
There is no way to see the answer without also getting it.

For a script the author runs on their own machine that is a reasonable trade. For the
one operation in this product that ends with somebody restoring a backup onto a phone
they depend on, it is backwards. The safety rules say a destructive action needs a dry
run and a typed confirmation; a report that can only be read afterwards is neither.

## Decision

The two halves are separate calls. `migrate.Build` opens both sides read-only, decides
every question, and returns a `Plan` without writing a byte. Applying that plan is a
second call, against a copy, and the wizard will not offer it until the report has
been on screen.

The plan is written to be read by the person deciding rather than by a developer:
every message in the source is accounted for in exactly one of three ways — it would
be added, it is already there, or it cannot be carried across — so the three add up to
the source's own total and a missing message is a bug rather than a rounding. A test
asserts that sum on every fixture and on real archives.

### What it refuses to do quietly

Three things are left out by default, each of which the report names and counts:

- **Groups.** A group arrives without its members' own history and is the part most
  likely to look wrong afterwards, so including it is a choice.
- **Conversations that exist only behind a hidden identifier.** WhatsApp increasingly
  files people under an opaque `@lid` rather than a number. One that this archive
  holds no phone number for cannot be matched against anything on the iPhone, so it
  would always be created rather than merged, which is how somebody ends up with the
  same person twice.
- **Everything that is not conversation**: call history, and the notices WhatsApp
  writes into a chat about itself. Carrying those across would mean inventing iPhone
  rows for events that did not happen on the iPhone.

A conversation left out silently is one somebody discovers is missing weeks later, on
a phone they can no longer compare against, so each carries a sentence saying why.

### The same person under two names

The single most valuable thing the target store gives up is WhatsApp's own record of
which hidden identifiers belong to which phone numbers. Without it, a conversation
filed under a number on one phone and behind an identifier on the other looks like two
different people, and a merge becomes a duplicate. With it, they are one conversation.
It is optional, because a device that never recorded one is not a device this should
refuse.

### Running it twice is safe by construction

A message is recognised by WhatsApp's own identifier for it, which is the same on both
phones. A second run over a phone that already has everything plans to add nothing.
That is not a convenience: it is what makes a migration something a person can think
about, stop, and come back to.

### The wording is not written twice

A photograph cannot follow into the iPhone store, so it arrives as a line of text
saying what was sent. Those words already existed — the text export writes them and
the viewer shows them — so `export.Line` is exported and the migration uses it. A
photograph described one way on a phone and another way in an export is the sort of
difference that makes somebody doubt both.

## Consequences

- Checked against the oracle on real data, and it agrees exactly: the same messages
  added, the same arriving as placeholders, the same not carried across. The first
  comparison appeared to disagree badly, and the cause was that the oracle's report
  came from the first of a two-step run; the differential test now sums a sequence and
  says why in a comment, because that mistake cost an afternoon.
- On a real archive of 1,121,482 messages the plan takes 34 seconds and would add
  1,093,822 messages to 850 conversations. It took 285 seconds until the database was
  prepared, which was the missing indexes rather than anything in this package — the
  reason to measure before optimising, since the profile would have sent somebody
  looking in the wrong place.
- Writing into a store is fussier than reading one. A reader tolerates a missing
  column, because that is a detail it cannot show; the writer refuses, because a store
  whose shape has moved is a store where a guess ends up on somebody's phone. The two
  therefore have different ideas of what an acceptable store is, deliberately.
- Still to come, in this order: applying a plan to a copy, the invariants that check
  the result against the original, putting the result back into a copy of a backup,
  and the guided flow that hands the restore to Finder. Nothing here touches a device
  and nothing later will either.
