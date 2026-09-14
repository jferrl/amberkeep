Read, search and export your own WhatsApp history from backups you already have, on
your own machine. It makes no network calls, never writes to a file you point it at,
and keeps your decryption key out of its own output.

## Getting started

From an iPhone:

```sh
amberkeep serve
```

That opens a page that finds the backups on this computer, brings the messages out of
one, builds the search index, and shows them. From an Android phone it takes
`msgstore.db.crypt15` and the 64-digit key and does the same. It writes to
`~/Amberkeep` and reads everything else without touching it.

`amberkeep --help` lists the commands for doing it one step at a time.

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

**`amberkeep migrate` has never been restored to a phone.** It moves an Android
history into a copy of an iPhone backup, and it is held to an unusual standard: it
agrees exactly with the prototype that performed the original migration, all 3,427
consistency checks pass on a full run of 1,093,822 messages, and putting the result
into a real 4.6 GB backup changes 2 files out of 27,352 and leaves the original
byte-identical.

None of that is the same as a phone having accepted one. Until somebody has done that
on a spare device, treat the command as unproven, and read what it tells you before
running it with `--write`. It will not write anything without a word typed out in
full.

## These binaries are not signed

macOS will say the app cannot be checked for malicious software, and Windows
SmartScreen will warn. Code signing costs a few hundred a year and is not worth paying
for until somebody wants this; that is an honest trade rather than an oversight.

On macOS, after downloading:

```sh
xattr -d com.apple.quarantine ./amberkeep
```

Every archive's SHA-256 is in `SHA256SUMS`, and the binaries are built by the workflow
in this repository from the tagged commit, with `-trimpath` so the same source
produces the same bytes.

## Licence

AGPL-3.0. The engine is free software so that anybody asked to trust it with their
entire message history can read what it does with it.

Not affiliated with, endorsed by, or connected to WhatsApp LLC or Meta Platforms, Inc.
