# 4. The import wizard

- Status: accepted
- Date: 2026-09-14

## Context

Everything built so far assumes the hard part is over. `amberkeep serve --db
msgstore.db` needs a decrypted database; getting one means knowing that WhatsApp's
backup is encrypted, that the 64-digit key is not the same thing as a backup
password, where Android files the backup, that an iPhone backup keeps every file
under a hash of its own path, and that a message store copied out on its own shows
no photographs. Somebody who knows all of that does not need this program.

The person this is for is the one whose phone died, or whose Move to iOS failed for
the third time. They have a folder Finder made and will not open, or a file called
`msgstore.db.crypt15` they were told to copy off a phone. The distance between that
and a readable archive is the product.

## Decision

`amberkeep serve` starts with no archive. The server holds a session with four
stages — empty, working, ready, failed — and the page walks somebody from whatever
they have to an archive that is open. Six endpoints:

    GET  /api/state                                  what is happening, and what is open
    GET  /api/backups                                the iPhone backups on this computer
    POST /api/open    {path, contacts?}              read something already readable
    POST /api/extract {backup, into?, contacts?}     take a store out of an iPhone backup
    POST /api/decrypt {file, key, into?, contacts?}  decrypt an Android backup
    POST /api/close   {}                             go back to the beginning

Each of the three that does work answers 202 immediately and runs in the
background; `/api/state` is the only thing the page polls. While nothing is open,
every endpoint that reads an archive answers 409 rather than an error, because a
page that has not caught up is not a failure.

### The wizard guides; it does not drive the phone

It reads backups that Finder, iTunes or the Apple Devices app already made, and
tells somebody how to make one. It does not speak the backup protocol, does not
drive a phone over adb, and does not restore anything. This follows what ADR 1
settled for the engine and is the same decision one level up: the value is in
knowing what to tell somebody, not in taking the wheel. It also means the whole
wizard is reversible — nothing it does can leave a phone in a worse state than it
found it.

### The server does not know how any of it works

`api.Importer` is the seam. The server knows what to ask and when to say so; the
command knows how a crypt15 file is decrypted, where Apple hides a backup, and
which of two databases it has been handed. That knowledge already existed in
`cmd/amberkeep`, and duplicating it behind the API would produce two
implementations that could disagree about what a backup is. So `cmd/amberkeep`
satisfies the interface and every method is a few lines calling the command's own
helper.

A consequence worth stating: a server started without an importer — a test, or an
embedding that only ever serves one archive — answers 501 to the wizard and serves
the archive it was given. The wizard is an addition, not a requirement.

### The long operations report as they run

Decrypting is seconds; indexing a million messages is half a minute; extracting a
backup over a slow disk is longer than either. A person watching a bar that does not
move assumes the program has hung, so `Importer` takes a `Progress` callback and the
sentences it writes are shown verbatim. They are written to be read by the person
waiting, not by a developer reading a log.

### What it writes, and where

Everything lands in a workspace, shown on screen before anything is written to it
and defaulting to `~/Amberkeep`: a folder somebody who has forgotten all of this can
still find by looking. Originals are read and never touched. Decryption refuses to
write over a file that is already there, because what is already there may be the
archive from last time.

Indexes are the one exception to "originals are never modified", and only on a file
this program itself just wrote a moment earlier. An index holds no information that
is not already in the table beside it.

### The key is used and dropped

The 64-digit key arrives in one request, is passed to the decryption, and is not
written down, logged, echoed back in the state the page polls, or put into any
error. Errors are the thing most likely to be pasted into a bug report. There is a
test in `internal/api` that fails if it appears in any response, and another in
`cmd/amberkeep` that checks the same thing over the real HTTP API.

### Failing is not the end of the road

A failure sets the stage to failed with two things: one sentence saying what went
wrong, and the several lines of guidance that already exist for that error in
`cmd/amberkeep/guidance.go`. The next request works from the same screen. Nobody is
left at a dead end with a sentence they cannot act on.

## Consequences

- `api.Archive` gained `Close`, because a session that opens a second archive has to
  let go of the first: somebody trying two backups in turn must not leave a database
  open on each.
- The search index moved from the server to whatever archive is open, so an archive
  brought in through the wizard is searchable without anybody being told to run a
  command. An index that cannot be built — a folder that cannot be written to — is
  reported and the archive opens anyway, because an archive that cannot be searched
  is far better than one that will not open.
- One import at a time. A second request while one is running is refused with 409
  rather than queued: two decryptions writing to the same file would ruin both.
- Work outlives the request that began it. Somebody who closes the tab halfway
  through a decryption comes back to a finished archive rather than half a file.
