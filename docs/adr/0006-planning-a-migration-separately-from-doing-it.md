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
- The checks that verify a result came next, before the thing that produces one. A
  checker written after the writer can only be tested by running the writer and
  watching it pass, which shows the two agree and nothing about whether a fault would
  be noticed. Written first, each of the twenty-odd checks is tested by damaging a
  store in exactly that way and insisting it fails. Two of them earned their keep
  immediately: "nothing was removed" compared row totals, so a deletion hid behind the
  insertions, and it now looks for each original row by its own identifier; and
  checking a store created a `-shm` beside it, so the second run of the checks failed
  the first check on a store the first run had called sound.
- Those checks pass on the migration the prototype actually performed and that was
  restored onto a phone still in use — all thirty-one of them. One had to be relaxed
  to get there, and the reasoning is worth keeping: the prototype's own validator
  demanded the message counter be strictly above the highest position in a
  conversation, and the store it produced and restored has them equal. Direct evidence
  that equality is safe beats an inference about what the field means, and a check
  that blocks work already known to be good is worse than no check, because it teaches
  people to ignore the report.
- The writer came third, and is held to the plan rather than trusted: it counts what
  it writes and refuses to hand back a store whose totals differ from the plan
  somebody agreed to, deleting it rather than reporting it. That refusal caught a real
  bug within the hour — the plan's running totals were added on each pass over a
  conversation, so a conversation counted twice had its earlier pass counted again.
  The plan is now totalled once, from its own conversations.
- It writes only into a copy, in one transaction, and never over anything that already
  exists: the file most likely to be at that path is the last attempt, which somebody
  may still need. Identifiers come from Core Data's own counter, or from the highest
  row actually present when the counter has fallen behind it — a store where those
  disagree would otherwise have this hand out an identifier already in use, and the
  row written with it replaces one that was there.
- The values WhatsApp fills its own rows with — flags, statuses, a spotlight code —
  are sampled from the store being written into rather than written down here. They
  are documented nowhere, and the store was written by the version of WhatsApp that
  will read it back. On a real store eight of them are copied; the fallbacks are what
  the prototype found in a 2026 store and are a last resort, not a default.
- On the real archive: 1,093,822 messages into 850 conversations, 829 of them new, in
  1 minute 21 seconds, and all 3,427 checks pass.
- Two things only real data produced. The check for a message appearing twice asked
  the question of the whole store, and WhatsApp's message identifier is unique to a
  conversation rather than globally: 32,664 identifiers are shared between exactly two
  conversations each, and none is repeated inside one. It asks per conversation now.
  And the planner matched a conversation on its resolved address while the writer filed
  it under the raw one, so a person the iPhone knows by a hidden identifier and the
  Android knows by number was created a second time under the identifier the phone was
  already using. Both now use the resolved address.
- That second fix moves a harder problem into the open rather than solving it. Two
  source conversations that are one person are now folded into one, which is what the
  archive showed: on 4,286 conversations there was one such pair. But when the iPhone
  files somebody under a hidden identity and WhatsApp's own record of which identity is
  which number was not supplied, the two cannot be matched at all, and the person
  arrives twice under two different addresses — where no check can see it, because
  nothing about the result is wrong. That is now a warning on the plan, named and
  counted, because it is the one failure here that looks exactly like success.
- Still to come: putting the result back into a copy of a backup, and the guided flow
  that hands the restore to Finder. Nothing here touches a device and nothing later
  will either.
