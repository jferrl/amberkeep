Read, search and export your own WhatsApp history from backups you already have, on
your own machine. It makes no network calls, never writes to a file you point it at,
and keeps your decryption key out of its own output.

**Free, and staying free.** There is no licence, no key and no tier: reading a history
back off a phone that died, searching it, writing it out, moving it onto another
phone — all of it, with no account and nothing switched off. One line in the program
offers to buy the author a coffee, at the two moments when something that mattered has
just worked, and that is the whole of it.

## What is new since v0.3.0

**Your photographs.** A WhatsApp database records where every picture was and keeps
at most a thumbnail of it: on the archive this was built from, 92,941 of 99,041
attachments record a path and 12,510 have a thumbnail. So an archive read on its own
has always shown about one picture in eight, at the size of a stamp, and every voice
note was a line of text saying a recording was sent. It now shows the photographs
themselves, plays the videos and plays the recordings.

- **From the phone, over the cable.** The screen that takes a backup off an Android
  phone offers to bring the files with it, and says how many gigabytes that is before
  you agree to wait — 5.7 GB on a real device. It copies one kind at a time, says
  which, and a copy that is interrupted picks up from the kind it reached rather than
  starting the gigabytes again. It is off unless you ask: it is the longest part and
  it happens before any messages appear.
- **Or from a folder you already have.** Put the phone's `WhatsApp` folder beside the
  database and there is nothing to configure — it is looked for beside the file, in a
  `WhatsApp` folder next to it, and in the folder above, which is where the phone
  itself keeps the two. If it is somewhere else, there is a field for it, and
  `--whatsapp-folder` on the command line.
- **They travel with an export.** Web pages now point at the files themselves, copied
  out under the paths the phone recorded. The folder as a whole still works with the
  network switched off, from a USB stick, in ten years — which is the promise that
  matters. It is a tick box, because it is usually much the largest part of an export,
  and `--media=false` turns it off.
- **Nothing is opened that should not be.** Files come off somebody's phone and are
  served from the same address as the program's own page, so only pictures, video and
  audio are shown as themselves; everything else, an HTML file or an SVG included,
  arrives as bytes to be saved. Recorded paths are refused if they try to climb out of
  the folder they name — refused on the way in and again on the way out, because
  reading somebody's disk and writing to it are different operations.
- **A folder with nothing in it says so.** Naming the wrong one stops and explains
  which to name instead, rather than opening an archive with no pictures in it and
  leaving you to conclude they are gone.

This is the Android side. An iPhone backup still gives up its thumbnails only.

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
- **Export** — web pages, text and structured data; 55 seconds for the whole archive,
  longer when it is carrying the photographs out with it.
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
