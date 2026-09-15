# What is next

Two tracks. The program is ahead of the business, and neither finishes without the
other: an application nobody can buy is a hobby, and a shop in front of a program
nobody trusts is worse than nothing.

Everything here is ordered by what blocks what, not by what is interesting.

## The program

| | What | State |
|---|---|---|
| 1 | Give the unrecognised message types their meanings, from evidence rather than guesswork | next |
| 2 | Media: an Android archive shows the small copies WhatsApp keeps inside the database — 12,510 of about 99,000 attachments on a real device — and nothing else. The files are on the phone, and `amberkeep` already knows how to read from one | decided against for iPhone backups, open for Android, and a product decision rather than a task |
| 3 | The compatibility matrix fills up as shapes arrive | continuous |

The first one is bounded and evidence-led. About 1,265 of 161,032 messages on a real
iPhone store carry a type code that means nothing on its own; the media type recorded
beside each one settles most of them, which is exactly how the codes that are known
came to be known. `amberkeep canary` already finds them.

## The money

There is none, and that is settled: Amberkeep is free, with one line offering a coffee
at the two moments when something that mattered has just worked. The reasoning is in
[ADR 10](adr/0010-a-tip-jar.md) and what it means for anybody using it is in
[WHAT_IT_COSTS.md](WHAT_IT_COSTS.md).

| | What | State |
|---|---|---|
| 1 | Decide what this costs | done — nothing |
| 2 | Find out what receiving money would actually oblige | **the blocker**, and not a programming one |
| 3 | Somewhere to say thanks: a Ko-fi page, suggested ten euros | after 2 |
| 4 | Put its address in `thanks.Address` | one line, after 3 |
| 5 | Signing certificates, if any ever arrives | the first thing worth buying, because it protects the person downloading rather than the revenue |

Step 2 is why the rest is waiting, and it is arithmetic rather than reluctance.

A voluntary contribution with nothing given in return is outside VAT, which is why the
line offers nothing in exchange and never will. What it is *not* outside is income tax.
Spain then asks two questions that have no settled answer: whether money received for a
published program is a rendimiento de actividad económica in IRPF or a donation under
ISD, and whether receiving it habitually obliges the author into RETA — whose minimum
contribution is larger than this is ever likely to earn in a month.

So the position is: take nothing until that is answered by somebody qualified to answer
it, which is an hour with a gestor rather than an afternoon of reading. `thanks.Address`
stays empty, both places that would show the line show nothing, and the program is
finished either way. Nothing here is waiting on the money.

## The legal side

| | What | State |
|---|---|---|
| 1 | Non-affiliation, trademark posture, what the program does with somebody's data | written — [LEGAL_NOTICES.md](LEGAL_NOTICES.md) |
| 2 | Who is selling, from where, under which law: the seller's own details, which no program can fill in for them | outstanding |
| 3 | A trademark search on the name before it is on an invoice | outstanding |

## Not on either track

**Restoring a produced backup onto a real phone.** It has not been done. Everything
before it is tested and the program says so on every screen that leads to it. It is
one person, one spare phone, one afternoon, and it is not something any amount of code
here can substitute for.

**Signing.** Deliberately switched off: both operating systems refuse an unsigned
application the first time and the README says how to get past it. The workflow to sign
and notarise is written and dormant, waiting for a certificate that costs money per
year — which is a decision for after the first sale rather than before it.
