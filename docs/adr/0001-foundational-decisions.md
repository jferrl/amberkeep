# 1. Foundational decisions

- Status: accepted
- Date: 2026-09-13

These decisions came out of a working prototype: a full WhatsApp history was recovered
from an Android phone, made searchable, then merged into an iPhone's existing WhatsApp
database and restored successfully. They are recorded together because they were taken
together, before any production code existed.

## Go, not Rust or TypeScript

**Decision.** Go for the engine, the command line tool and the desktop backend.

**Why.** It produces one static binary per platform, cross-compiles without a toolchain
zoo, and embeds a web interface with nothing but the standard library. Rust would suit
this problem at least as well and has a better desktop story through Tauri, but it
roughly doubles the effort for a developer who is fluent in Go and not in Rust, and
velocity decides whether this ships at all. Electron ships a browser per download.
Swift has no Windows story.

**Consequence.** `CGO_ENABLED=0` everywhere, which rules out the common SQLite driver
and settles the next decision.

## Pure-Go SQLite

**Decision.** `modernc.org/sqlite`.

**Why.** It is a transpiled SQLite with FTS5 and JSON1 compiled in, and it needs no C
toolchain, so Windows builds and macOS notarisation stay simple. It runs roughly one
and a half to three times slower than the C library on processor-bound work, which for
a one-off full-text index build over a million messages means tens of seconds and, for
queries, tens of milliseconds. That is comfortably good enough for a desktop tool.

**Alternative if it ever bites.** `ncruces/go-sqlite3`, which is also free of cgo and
closer to C speed. Not `mattn/go-sqlite3`, which would reintroduce the toolchain.

## No device protocol in version one

**Decision.** Read and patch backups that Finder, iTunes or the Apple Devices app have
already made. Hand the restore back to those tools.

**Why.** No Go library implements Apple's backup protocol; `go-ios` covers pairing and
many services but not `mobilebackup2`. Shelling out to `libimobiledevice` works on
macOS but is fragile on Windows, needs Apple Mobile Device Support, and adds a copyleft
redistribution surface. Letting Apple's own software perform the restore is also the
safer experience: it is the path users already trust and can retry.

**Consequence.** The tool needs Full Disk Access on macOS to read the backup folder,
which cannot be requested programmatically, so onboarding must detect the refusal and
guide the user. It also means the Mac App Store is impossible, which is fine because
sandboxing rules it out anyway.

## Merge, never overwrite

**Decision.** Migration adds history into the chats already on the destination phone,
deduplicating by message identifier, and never replaces the destination database.

**Why.** The prototype ran against a phone that was already registered and in use, with
existing groups and recent messages. Overwriting would have destroyed them. Every
commercial tool in this space replaces instead, and "it said success but half my data
is missing" is the most common complaint about them.

## Text first, media later

**Decision.** Version one migrates message text, with media rendered as placeholders.

**Why.** Photos and videos already live in the phone's gallery, so the text is what is
actually at risk. Writing media items without their files is the part of the iPhone
schema least understood and most likely to make the application misbehave, and the
proven approach encodes every message as plain text.

## AGPL for the engine

**Decision.** The engine is AGPL-3.0. The desktop application is a separate commercial
product built on it.

**Why.** People are being asked to trust software with their most private
conversations; an auditable engine is the only honest basis for that trust. The AGPL
keeps competitors from folding the work into closed products without contributing back,
while the application, its interface, packaging and support remain sellable.

## Standard library first

**Decision.** Every third-party dependency needs a decision record justifying it.

**Why.** This code handles encryption keys and parses untrusted files. A small,
auditable dependency graph is a security property, not an aesthetic preference, and it
is also what makes a single static binary practical.

**Consequence.** The crypt15 implementation, including its protocol-buffer header
parser, uses nothing outside the standard library, and is verified byte for byte
against the reference implementation on a real backup.
