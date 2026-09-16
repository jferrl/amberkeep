# What is next

Two tracks. The program is ahead of the business, and neither finishes without the
other: an application nobody can buy is a hobby, and a shop in front of a program
nobody trusts is worse than nothing.

Everything here is ordered by what blocks what, not by what is interesting.

## The program

| | What | State |
|---|---|---|
| 1 | Give the unrecognised message types their meanings, from evidence rather than guesswork | next |
| 2 | Media: an Android archive reads the phone's own folder — found beside the database or named on the page or the command line — shows the photographs, videos and voice notes themselves rather than the stamp-sized copies, and carries them out when the archive is exported. What is left is pulling the folder off the phone over adb, so that nobody has to copy several gigabytes by hand first | done, except getting the folder off the phone |
| 3 | The compatibility matrix fills up as shapes arrive | continuous |

The second is worth saying plainly, because it is the largest visible difference a
person will notice. A database records where a photograph was and keeps at most a
thumbnail of it: on a real device 92,941 of 99,041 attachments record a path and
12,510 have a thumbnail, so an archive read on its own shows about one picture in
eight, and every voice note is a line of text saying a recording was sent. With the
phone's folder beside it the archive shows the photographs and plays the recordings.
Nothing is copied and nothing is written while the archive is open: the files are read
where they lie. An export is the one thing that copies them, because a copy of
somebody's history with the pictures taken out is not a copy of it.

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
| 2 | Somewhere to say thanks | done — <https://ko-fi.com/jferrl> |
| 3 | Put its address in `thanks.Address` | done |
| 4 | Ask somebody qualified what receiving it obliges | outstanding, and not a programming question |
| 5 | Signing certificates, if enough ever arrives | the first thing worth buying, because it protects the person downloading rather than the revenue |

Step 4 is the one left and it is worth an hour of somebody's professional time rather
than an afternoon of reading. A voluntary contribution with nothing given in return is
outside VAT — which is why the line offers nothing in exchange and never will — but it
is not outside income tax, and Spain then asks two questions with no settled answer:
whether money received for a published program is a rendimiento de actividad económica
in IRPF or a donation under ISD, and whether receiving it habitually obliges the author
into RETA.

None of that is in the way of anybody using this. The program asks for nothing, shows
one line at two good moments, and works the same whether or not a single coffee ever
arrives.

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
