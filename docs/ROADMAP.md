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

Nothing here charges anybody yet. The order matters: the licence has to exist before
there is anything to sell, and the shop has to exist before a price means anything.

| | What | State |
|---|---|---|
| 1 | Decide and publish what costs what | done — [PRICING_AND_LICENSING.md](PRICING_AND_LICENSING.md) |
| 2 | Offline licence keys: a key that verifies on a machine with no network, because this program makes no network calls and a licence is not going to be the exception | built, and inert until there is a shop |
| 3 | A merchant of record — Paddle — to take the money and handle VAT in every country somebody buys from | needs an account, which needs a person with a bank |
| 4 | Somewhere to buy it: a page that says what it does, what it costs, and what it will not do | after 3 |
| 5 | Turn the gate on: exports and migration ask for a licence, everything else stays free | deliberately last |

Step 5 is `licence.Sells()` returning true instead of false, plus the screen that
says what to do about it — the four places that ask the question already exist, and
so do the words, in both languages. It is last on purpose: until a licence can be
bought, a program that asks for one is a program that cannot be used.

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
