Read, search and export your own WhatsApp history from backups you already have, on
your own machine. It makes no network calls, never writes to a file you point it at,
and keeps your decryption key out of its own output.

**This is the release where it becomes an application.** The first one was a
command-line tool; this one is a window with a dock icon, a menu bar and the
operating system's own file pickers, and the same program underneath.

## What is new since v0.1.0

- **A window of its own**, on macOS and Windows, around the same engine. The
  command-line tool is still here and still one static binary.
- **The wizard finds your backups** instead of asking you to type a path. It lists
  what Finder, iTunes or Apple Devices have already made on this computer, says which
  are encrypted and why that stops it, and takes a typed path anyway.
- **It takes the backup off an Android phone for you.** Plug the phone in and
  Amberkeep copies the encrypted backup across and decrypts it. Every command it runs
  is a read; it never writes to the phone. Without the Android tools installed it says
  how to install them, and the four steps to copy the file by hand still work.
- **Keeping a copy is on the screen**, not only at a prompt: web pages, plain text and
  structured data, written to a folder you choose.
- **Moving an Android history onto an iPhone is a guided flow**, one deliberate step
  at a time — what can be checked, what would move, a word typed out in full, and the
  sixteen things to do afterwards that nobody tells you.
- **It speaks Spanish everywhere a person reads.** Not the screens alone: the restore
  guide, the running commentary while it works, what it says when something fails and
  what to do about it, and what every check looked at and found. In the terminal too,
  which reads the locale it is run in.

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
