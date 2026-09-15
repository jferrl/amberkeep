<img src="docs/brand/mark.svg" alt="" width="76" />

# Amberkeep

[![CI](https://github.com/jferrl/amberkeep/actions/workflows/ci.yml/badge.svg)](https://github.com/jferrl/amberkeep/actions/workflows/ci.yml)
[![Licence: AGPL-3.0](https://img.shields.io/badge/licence-AGPL--3.0-A64B08)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.27-A64B08)](go.mod)
[![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Windows%20%7C%20Linux-A64B08)](#status)

Read, search and export your own WhatsApp history from backups you already have, on
your own computer. Later, move that history between an Android phone and an iPhone.

Amber preserves an insect perfectly for fifty million years. That is the idea.

> Amberkeep is not affiliated with, endorsed by, or connected to WhatsApp LLC or Meta
> Platforms, Inc. It reads backup files you already possess, using keys you already
> have, and never contacts WhatsApp's services.

## Status

The engine and the command-line tool work end to end on real archives: decrypt, read,
search and export. The iPhone side and the desktop application are being built.

| Component | State |
|---|---|
| `crypt15` decryption | working, golden-tested against `wa-crypt-tools` |
| Android message reader | working, including hidden identities and every content table |
| iPhone message reader | working, including replies and pictures no other tool recovers |
| Export: web pages, text, JSON | working, in the browser and on the command line |
| Full-text search | working, accent-insensitive |
| Local viewer (`serve`) | working, React 19 and TypeScript, built into the binary |
| Command-line tool | working |
| Import wizard | working, in the browser and on the command line |
| iPhone backup access | working, including backups Finder will not show you |
| Desktop application | working on macOS and Windows; a window, a dock icon, a menu bar and native file pickers |
| English and Spanish | working |
| Android to iPhone migration | working in the browser and by `amberkeep migrate`, but see below |

### Opening it the first time

**Nothing here is signed, so both operating systems will refuse it the first time.**
That is not a bug in the download and not a claim about the download: it is what
happens to any application whose publisher has not paid for an identity, and this
one has not. Signing is a deliberate not-yet rather than an oversight.

On **macOS**, the disk image mounts and the application refuses to open. Clear the
quarantine flag the browser attached and it will run:

```sh
xattr -dr com.apple.quarantine /Applications/Amberkeep.app
```

On **Windows**, SmartScreen shows "Windows protected your PC". Choose **More info**,
then **Run anyway**.

If you would rather not do either, build it yourself — `go build ./cmd/amberkeep` for
the command line needs nothing but Go, and produces the same program.

**The migration needs a Mac or a Windows PC**, because restoring a backup onto an
iPhone is done by Apple's own software — Finder, iTunes or the Apple Devices app —
and Apple ships none of it for Linux. On Linux, Amberkeep reads Android backups,
searches and exports them, and opens an iPhone backup folder copied from another
machine; it will also produce a changed backup, but something else has to restore it.

**No backup Amberkeep has produced has ever been restored to a phone.** The migration
plans, writes and checks itself — twenty-one consistency checks over the result, and a
copied backup differing from the original in exactly the two files it meant to change —
but the last step, restoring that backup with Finder onto a real iPhone, has not been
done. Everything before it is tested; that is not the same thing. The program says so on
every screen and on every run, and says it here too.

Measured on one real archive of 4,286 conversations and 1,121,482 messages, on a laptop:

| Step | Time |
|---|---|
| decrypt 236 MB to 466 MB | 3.6 s, 350 MB of memory |
| export every conversation as web pages | 55 s |
| build the search index | 36 s |
| a search across the whole archive | under 10 ms |
| open a 92,180-message conversation in the viewer | 0.12 s |

And on a real iPhone store of 554 conversations and 161,026 messages:

| Step | Result |
|---|---|
| take the store and its pictures out of a backup | 2.3 s, 9,430 pictures |
| read every message | 0.9 s |
| export every conversation as web pages | 3 s, 9,404 pictures shown |
| page backwards through a 32,935-message conversation | 0.19 s |

## Installing it

From [Releases](https://github.com/jferrl/amberkeep/releases):

- **`Amberkeep_*_macos.dmg`** — the application, for every Mac. One disk image with
  both architectures in it, so it runs natively on Apple silicon and on an Intel Mac.
- **`Amberkeep_*_windows_amd64.zip`** — the application for 64-bit Windows.
- **`amberkeep_*.tar.gz` / `.zip`** — the command-line tool, one static binary, for
  macOS, Windows and Linux on both architectures. Unpack it and run `amberkeep`. No
  installer, nothing to uninstall.

Nothing is signed, so macOS says it cannot check the application and Windows
SmartScreen warns. [Opening it the first time](#opening-it-the-first-time), above,
is how to get past both. Every archive's SHA-256 is published beside it.

Or build it yourself, which needs nothing but Go:

```sh
go build ./cmd/amberkeep
```

## Using it

If you do not already have a decrypted database, start here:

```sh
amberkeep serve
```

That opens a page with nothing in it yet. It finds the iPhone backups on this
computer, or takes an Android `msgstore.db.crypt15` and the 64-digit key, brings the
messages out, builds the search index, and shows them. It writes to `~/Amberkeep`
and reads everything else without touching it. Every step says what it is doing
while it runs, and every failure says what to do about it and lets you try again.

The commands below are the same thing one step at a time, for anyone who would
rather see each one.

```sh
amberkeep decrypt --key key.txt --in msgstore.db.crypt15 --out msgstore.db
amberkeep prepare --db msgstore.db          # only if you decrypted it elsewhere
amberkeep inspect --db msgstore.db --full
amberkeep serve   --db msgstore.db --contacts contacts.vcf --country 34
amberkeep export  --db msgstore.db --contacts contacts.vcf --country 34 --out archive/
amberkeep search  --db msgstore.db "whatever you remember"
```

A WhatsApp backup arrives with none of its indexes: the decrypted database has not
one, on any of the twenty tables a reader touches, so every query for a
conversation scans the whole table. `decrypt` puts them back on the file it just
wrote, which takes about a second and adds six per cent to its size. That took
exporting a real archive from five and a half minutes to fifty-five seconds. Run
`prepare` if
you decrypted the database some other way; it adds only indexes, which hold no
information that is not already in the file.

`serve` opens the archive in a browser, or starts the wizard when you give it no
`--db`. It listens on the loopback address only, and
every request carries a secret made fresh at each launch, so nothing else on the
computer can read the archive by finding the port. A conversation opens at its end
and loads earlier messages as you scroll, which is what makes a conversation of
ninety thousand messages open at all.

`export` writes one web page per conversation and an index to open them from. Each page
is a single file with the stylesheet, the script and every recovered picture inside it,
so it works with the network switched off and will keep working.

`--contacts` takes a vCard export from the phone's address book. Without it,
conversations are labelled by phone number, which is the single most noticeable way an
archive can disappoint.

Every failure this tool understands comes with what to do about it, in the output,
rather than an error to search the internet for.

## What it does differently

Four things the paid tools in this space do not do:

- **Merges instead of overwriting.** History is added to the chats already on the
  destination phone, deduplicated by message id, rather than replacing them.
- **Verifies before touching anything.** A dry run reports exactly what will change,
  with a checksum of what would be written, before any destructive step.
- **Keeps up with current formats.** A published compatibility matrix says plainly
  which WhatsApp, Android and iOS versions are verified, best-effort or unsupported.
- **Stays on your machine.** No account, no upload, no telemetry. The engine is open
  source so that claim can be checked rather than trusted.

## Guarantees

These are enforced by tests and by review, not just intended:

- **Zero network by default.** Update checks and crash reporting are opt-in and off.
- **Originals are never modified.** Sources are opened read-only or copied first.
- **Secrets never touch logs or disk.** Keys live in a type whose every rendering
  method redacts; a test asserts it.
- **Destructive actions require a dry run and a typed confirmation.**

## Requirements

Decrypting an Android backup needs the **64-digit key**, not a passphrase. In
WhatsApp: Settings, Chats, Chat backup, End-to-end encrypted backup, then choose the
64-digit key option and save the key somewhere safe.

Backups protected by a **passkey** cannot be decrypted outside WhatsApp, and
`crypt12`/`crypt14` backups need a key file that only root access can reach.
Amberkeep detects both and explains what to do instead.

## Building

Go 1.24 or newer. No cgo, no external toolchain, and **no Node.js**: the viewer is
built into the repository already.

```sh
go build ./...
go test ./... -race
golangci-lint run ./...
```

Changing the viewer needs Node 24. It lives in `web/` and builds into
`internal/viewer/dist`, which the binary embeds and which is committed.

```sh
cd web
npm ci
npm run typecheck && npm run lint && npm test && npm run doctor
npm run build
```

`npm run doctor` is React Doctor, run with its own severity gate and without its
score, because obtaining the score means a request to somebody else's server and
this project makes none.

The golden test runs against a real backup when you point it at one. Real backups and
real keys must never be committed:

```sh
AMBERKEEP_GOLDEN_CRYPT15=/path/to/msgstore.db.crypt15 \
AMBERKEEP_GOLDEN_KEY=/path/to/key.txt \
AMBERKEEP_GOLDEN_EXPECTED=/path/to/msgstore.db \
  go test ./internal/crypt15/ -run TestDecryptGolden
```

## How the formats work

WhatsApp publishes nothing about how it stores messages. These document what we
established, with the evidence for each claim and an honest account of what is still
unknown:

- [docs/formats/android.md](docs/formats/android.md) — the crypt15 backup and `msgstore.db`
- [docs/formats/ios.md](docs/formats/ios.md) — iPhone backups and `ChatStorage.sqlite`

## Contributing

Read [docs/PRINCIPLES.md](docs/PRINCIPLES.md) first; it is binding, not advisory.
Commits follow the Conventional Commits format and need a DCO sign-off (`git commit -s`).

See [CONTRIBUTING.md](CONTRIBUTING.md) for the checks and the sign-off, and
[SECURITY.md](SECURITY.md) if you have found something that should not be said in
public.

Schema reports are the most useful contribution. WhatsApp changes its databases every
few months, and a **structure-only** dump from a version we have not seen keeps the
readers working for everyone. Never send message content.

## Licence

Copyright © 2026 Jorge Ferrero. AGPL-3.0-or-later; the full text is in
[LICENSE](LICENSE), verbatim from [gnu.org](https://www.gnu.org/licenses/agpl-3.0.txt).

The engine is free software so that anyone can verify what it does with their private
messages. That is the whole reason for the choice: a program that asks to be trusted
with somebody's entire history has no business being unreadable, and the AGPL is the
licence that keeps it readable even when it is run as a service rather than shipped.

`amberkeep serve` puts a browser in front of that engine, so the two notices the
licence and Meta's trademark rules ask for are on screen in the program itself, not
only here. The desktop application built on top of the engine is a separate commercial
product.

## Prior art

Standing on the shoulders of [wa-crypt-tools](https://github.com/ElDavoo/wa-crypt-tools),
[WhatsApp-Chat-Exporter](https://github.com/KnugiHK/Whatsapp-Chat-Exporter) and
[watoi](https://github.com/residentsummer/watoi), which documented these formats first.
