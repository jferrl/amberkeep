# 2. Apple backup access

- Status: accepted
- Date: 2026-09-13

## Context

Amberkeep needs to read WhatsApp's iPhone store, `ChatStorage.sqlite`, out of a
backup Finder, iTunes or the Apple Devices app already made. Apple's backup format is
a folder of content-addressed blobs plus a SQLite manifest, described in
`internal/backupfs`'s package comment. Two sub-decisions came out of building that
package.

## Property-list parsing

**Decision.** Add `howett.net/plist` to read `Info.plist`, `Manifest.plist`, and the
NSKeyedArchiver-encoded `MBFile` metadata blob stored in each `Manifest.db` row.

**Why.** Apple's property-list format has three wire encodings (XML, binary, and the
old OpenStep text format) and, for the `MBFile` blobs specifically, is wrapped a
second time in NSKeyedArchiver's flat, UID-referenced object graph. `encoding/xml`
only reads the least common of the three encodings that Backup, iTunes and the Apple
Devices app actually write, which in practice is almost always binary. Reimplementing
a binary-plist and keyed-archiver reader is a well-defined but sizeable amount of
work — this is exactly the shape of problem the standard-library-first rule expects
an exception for, not a reason to abandon it. `howett.net/plist` is small, has no
transitive dependencies, needs no cgo, and decodes both binary and XML plists into
plain Go values (`map[string]any`, `[]any`, `plist.UID`) that a caller can walk
directly, which is what `internal/backupfs/mbfile.go` does.

**Consequence.** One more dependency, audited the same way `modernc.org/sqlite`
already is: pure Go, no cgo, and its decoder already treats its input as untrusted
(a panic inside `Decode` becomes an error via its own `recover`). `internal/backupfs`
adds one more layer of defensiveness on top for the parts *it* does after decoding —
resolving a keyed-archiver reference and reading the `Size` field — with a fuzz test
covering that layer specifically.

## Detect encrypted backups; do not decrypt them

**Decision.** `internal/backupfs` reads `Manifest.plist`'s `IsEncrypted` flag and
refuses to open an encrypted backup (`ErrEncrypted`), rather than attempting to
decrypt one.

**Why.** This is a deliberate scope decision, not an oversight. An encrypted backup's
per-file keys are wrapped by a key derived from a password the user chose in Finder,
iTunes or the Apple Devices app, using a key-derivation and keybag format Apple has
never published. Reimplementing it correctly, and carrying the user's backup password
through this package to do so, is a substantial and security-sensitive undertaking —
for a problem the tool that made the backup already solves on request: turning
encryption off asks for the current password once and writes a fresh, unencrypted
backup. Amberkeep already asks for a decryption key on the Android side because there
is no alternative there; here there is one, and it is a few clicks.

**Consequence.** `Backups` still lists an encrypted backup, with `Encrypted` set, so
a caller can tell the user which of their backups need to be redone. `Open` refuses
one outright, and `ErrEncrypted`'s guidance text says exactly what to do about it.
If this decision is ever revisited, it is its own project: understanding Apple's
keybag format is unrelated to anything else this package does.
