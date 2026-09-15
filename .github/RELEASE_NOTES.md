Read, search and export your own WhatsApp history from backups you already have, on
your own machine. It makes no network calls, never writes to a file you point it at,
and keeps your decryption key out of its own output.

**Free, and staying free.** There is no licence, no key and no tier: reading a history
back off a phone that died, searching it, writing it out, moving it onto another
phone — all of it, with no account and nothing switched off. One line in the program
offers to buy the author a coffee, at the two moments when something that mattered has
just worked, and that is the whole of it.

## What is new since v0.2.0

- **`amberkeep canary`** says what a database holds that this build has never seen:
  tables and columns no known shape has, and message type codes nothing here has a
  meaning for, with how many rows carry each. It reads the catalogue and counts rows,
  so it takes seconds on a million messages, and what it prints is names, integers and
  counts — safe to paste into an issue without reading it first. It is the early
  warning for the next time WhatsApp changes its schema, and
  [what has been run against what](https://github.com/jferrl/amberkeep/blob/main/docs/COMPATIBILITY_MATRIX.md)
  now has a page.
- **It is free, on purpose and in writing.** The plan was to sell this; the reasoning
  for not doing so is in ADR 10. Nothing in the program asks for money, and the only
  thing that mentions it is a coffee button at the foot of the screen.
- **A way back to the start.** The Android walkthrough is six screens, and somebody who
  realises on the fifth that they picked the wrong thing no longer has to press Back
  five times to say so.
- **The screen that writes an archive out** has the window it is in: the promise at the
  top, the notices at the bottom, and its first line no longer under the close,
  minimise and zoom buttons.
- **Prose set as prose.** Every sentence about a failure or a restore step is written
  in a Go file and printed in a terminal, so it arrived hard-wrapped at seventy
  characters and was shown that way. Paragraphs are paragraphs now; what is indented —
  a command, a menu, a list — is left alone.

## Which download

- **`Amberkeep_*_macos.dmg`** — the application, for every Mac. One disk image with
  both architectures in it, so it runs natively on Apple silicon and on an Intel Mac.
  macOS 14 or later.
- **`Amberkeep_*_windows_x64.zip`** — the application for 64-bit Windows 10 or
  later. On a Windows machine with an ARM processor it runs under emulation.
- **`amberkeep_*.tar.gz` / `.zip`** — the command-line tool, one static binary, for
  macOS, Windows and Linux on both architectures. It needs nothing installed.

## Getting started

Open the application, or from a terminal:

```sh
amberkeep serve
```

Either one finds the backups on this computer, brings the messages out of one, builds
the search index and shows them. From an Android phone it takes `msgstore.db.crypt15`
and the 64-digit key and does the same. It writes to `~/Amberkeep` and reads
everything else without touching it.

`amberkeep --help` lists the commands for doing it one step at a time.

## Nothing here is signed

macOS will say the application cannot be checked for malicious software, and Windows
SmartScreen will warn. Code signing costs a few hundred a year and is not worth paying
for until somebody wants this; that is an honest trade rather than an oversight.

- macOS: `xattr -dr com.apple.quarantine /Applications/Amberkeep.app`
- Windows: SmartScreen → **More info** → **Run anyway**
- The command-line tool: `xattr -d com.apple.quarantine ./amberkeep`

`SHA256SUMS` is the only integrity story an unsigned download has, and it is worth
checking. Everything here is built by the workflow in this repository from the tagged
commit, with `-trimpath`, so the same source produces the same bytes.

## What is verified, and on what

Everything below was measured on one real archive of 4,286 conversations and
1,121,482 messages, and on a real iPhone store of 554 conversations and 161,026
messages:

- **crypt15 decryption** — byte for byte against `wa-crypt-tools`.
- **Android and iPhone readers** — including replies and pictures recovered from an
  iPhone store that holds only the paths to them.
- **Export** — web pages, text and structured data; 55 seconds for the whole archive.
- **Search** — the whole archive in under 10 milliseconds.
- **The viewer** — a 92,180-message conversation opens in 0.12 seconds.

## What is not

**No backup Amberkeep has produced has ever been restored to a phone.** It moves an
Android history into a copy of an iPhone backup, and it is held to an unusual
standard: it agrees exactly with the prototype that performed the original migration,
all 3,427 consistency checks pass on a full run of 1,093,822 messages, and putting the
result into a real 4.6 GB backup changes 2 files out of 27,352 and leaves the original
byte-identical.

None of that is the same as a phone having accepted one. Until somebody has done that
on a spare device, treat it as unproven. It will not write anything without a word
typed out in full, and it never touches the phone itself: restoring is Finder's job,
and the guide is what says how.

The Windows application has been built and tested by machines, and never yet run by a
person.

## Licence

AGPL-3.0. The engine is free software so that anybody asked to trust it with their
entire message history can read what it does with it.

Not affiliated with, endorsed by, or connected to WhatsApp LLC or Meta Platforms, Inc.
