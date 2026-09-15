# Legal notices

Not legal advice, and written by the people who wrote the program rather than by a
lawyer. It is here so that what this project believes about its own position is
written down and can be corrected by somebody who knows better.

## Not affiliated with WhatsApp or Meta

Amberkeep is **not affiliated with, endorsed by, or connected to WhatsApp LLC or Meta
Platforms, Inc.** WhatsApp is their trademark. This program is not their product and
nothing in it comes from them.

The notice appears in the application, on every screen of the wizard, in the archives
it writes out, and in every release.

## The name, and the rules the name follows

Meta's brand guidelines forbid using "WhatsApp" or a phonetic variant in a product
name — Wutsapper was made to rename — and forbid imitating their trade dress. So:

- The program is never named after theirs, in any spelling, and no domain, package or
  binary carries a variant of it.
- No green palette and no speech-bubble mark.
- Compatibility is described in plain words — "works with WhatsApp backups" — which is
  nominative use: naming a thing to say truthfully what this works with.
- The non-affiliation notice sits wherever the name is used.

## What this program does with somebody's data

Nothing, and that is testable rather than promised:

- **It makes no network calls.** Not for updates, not for licences, not for telemetry.
  A browser test fails the build if the page asks for anything outside the machine it
  is served from.
- **It never writes to what it is given.** Backups, databases and message stores are
  opened read-only; everything it produces is written somewhere else.
- **Keys and passwords stay in memory.** A decryption key is never written to a file
  and never appears in a log.
- **Nothing destructive happens without a dry run and a word typed out in full.**

Those four are the project's own rules, they are in
[PRINCIPLES.md](PRINCIPLES.md), and CI enforces what can be enforced mechanically.

## The law it operates under

The user decrypts **their own data** with **their own key** on **their own machine**.
No server of anybody else's is contacted, no account is accessed, no protection is
circumvented: WhatsApp gives the key to the person whose messages these are, and this
program uses it the way the phone would.

Tools that do this have been sold openly for over a decade. That is not a guarantee of
anything — it is the evidence available.

Deliberately out of scope, because each would change that position: rooting a phone,
downgrading the app to extract a key, defeating a passkey, or anything that reads
another person's account.

## Licences

- **The engine and the command-line tool are AGPL-3.0.** Free software, on purpose:
  anybody asked to trust a program with their entire correspondence should be able to
  read what it does with it, and anybody who improves it has to publish that too.
- Third-party components and their licences are listed in the source tree; every
  dependency is recorded in an architecture decision record with the reason the
  standard library was not enough.

## Still outstanding

1. **A trademark search** on the name in class 9, at EUIPO and USPTO. Less urgent
   than it was — nothing is being sold, so there is no invoice with the name on it —
   and still worth doing before the name is on anything public for long.
2. **Nothing else.** [Amberkeep is free](WHAT_IT_COSTS.md): there is no sale, no
   licence and no VAT, so the trading name, the address and the jurisdiction whose
   consumer law applies are questions this project no longer has to answer. A
   voluntary contribution given for nothing in return is not a sale.
