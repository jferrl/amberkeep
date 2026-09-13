# WhatsApp on Android

How WhatsApp stores messages on an Android phone, how those messages get into a file
you can read, and how this project reads them. It is written so that somebody could
build their own reader from it without looking at our code, and so that a contributor
can repair a schema change after reading it once. Everything here was checked against
the implementation in `internal/source/android/` and, where a number appears, against
one real archive of 1,124,291 messages. Where the evidence runs out, the document
says so rather than guessing: that is the part most other write-ups of this format
leave out, and it is the reason this one exists.

## Contents

1. [How to read this document](#1-how-to-read-this-document)
2. [Getting the file off the phone](#2-getting-the-file-off-the-phone)
3. [The crypt15 container](#3-the-crypt15-container)
4. [Two schema generations, and why we introspect](#4-two-schema-generations-and-why-we-introspect)
5. [Identity: `jid`, `@lid` and the recovery of a name](#5-identity-jid-lid-and-the-recovery-of-a-name)
6. [The message row](#6-the-message-row)
7. [Message type codes](#7-message-type-codes)
8. [Everything beyond words](#8-everything-beyond-words)
9. [System notices](#9-system-notices)
10. [Performance](#10-performance)
11. [What is not recoverable](#11-what-is-not-recoverable)
12. [Reporting a schema change](#12-reporting-a-schema-change)

---

## 1. How to read this document

Three labels appear throughout. They are not decoration; they are the document's
main claim about itself.

| Label | Means |
|---|---|
| **CONFIRMED** | Demonstrated on real data, usually by a count that matches exactly, or fixed by a cryptographic constant that would break if it were wrong. |
| **BEST GUESS** | A published decoder says so and nothing contradicts it, but no independent evidence in the archive corroborates it. |
| **UNKNOWN** | No source names it. The reader carries the value through by number and says so out loud. |

### The archive the numbers come from

One archive, read strictly read-only, on 2026-09-11 and again on 2026-09-13.

| | |
|---|---|
| Source | WhatsApp Android 2.26.35.75, Samsung SM-A566B, Android 16 |
| `msgstore.db.crypt15` | 247,024,263 bytes |
| `msgstore.db` decrypted | 488,787,968 bytes, page size 4,096, 119,333 pages |
| `PRAGMA journal_mode` | `delete` |
| `PRAGMA user_version` | 1 |
| Tables | 311, of which 3 are virtual (full-text search) |
| Views / triggers | 4 / 0 |
| Indexes | 32, every one of them `sqlite_autoindex_*` (see [§10](#10-performance)) |
| `message` rows | 1,124,291 |
| Messages reachable from a `chat` row | 1,121,482 |
| `chat` rows | 4,286, of which 440 hold no messages |
| Span | 2015-03-31 to 2026-09-10 |

One archive is one archive. A count here proves that something *occurs*, and an exact
correspondence between two counts is strong evidence about *meaning*; neither proves
that a rare code never means something else on somebody else's phone. Counts are given
so that a future reader can tell whether their archive looks like this one.

No message text, no phone number, no name and no other personal content appears in
this document, and none may ever be added to it.

---

## 2. Getting the file off the phone

### The file you want

`msgstore.db.crypt15`, in the phone's own storage under the WhatsApp media directory
(`Android/media/com.whatsapp/WhatsApp/Databases/` on current Android). It is readable
without root, because it lives in shared media storage rather than in the app's
private directory. Dated copies sit beside it.

### The key, not a password

End-to-end encrypted backups can be secured two ways, and only one of them can be
decrypted off the device.

| Protection | Offline? | Why |
|---|---|---|
| **64-digit key** | Yes | The key *is* the root key. WhatsApp shows it to you once, as eight groups of eight hexadecimal digits, and it is the input to the derivation in [§3](#3-the-crypt15-container). |
| **Password** | No | The password is not the key. WhatsApp derives the key server-side and releases it to the app only after the password is verified against Meta's servers. There is no offline path. |
| **Passkey** | No | Same shape as a password, with device biometrics in front. `crypt15.ErrPasskeyProtected`. |

In the app: Settings, Chats, Chat backup, End-to-end encrypted backup, then the
64-digit key option. Write it down before backing up, not after. **CONFIRMED** by the
golden test, which decrypts a real backup byte-for-byte with a real key.

### Why crypt14 needs root

`crypt12` and `crypt14` backups predate end-to-end encrypted backups. They are
encrypted with a device key that WhatsApp stores in
`/data/data/com.whatsapp/files/key` — inside the app's private directory, which is
unreadable without root on any unmodified Android. The backup file itself is easy to
copy and useless on its own. Amberkeep detects this case from the header (a
`crypt14KeyData` field where a crypt15 file has `e2eeKeyData`) and returns
`ErrCrypt14` with the advice to switch the phone to an encrypted backup with a
64-digit key and back up again. **CONFIRMED** by header parsing; the root requirement
is a property of Android's app sandbox, not of WhatsApp.

### Why `msgstore-increment-*.crypt15` files are useless alone

WhatsApp writes incremental backups between full ones. Each holds only the rows that
changed since the previous full backup, and each is encrypted with the same root key
but references a base it does not contain. Without the matching full `msgstore.db.crypt15`
there is nothing to apply them to, and the increments carry no schema of their own.
Copy the full backup. If only increments are present, wait for or force a full backup
from the app before copying anything. **BEST GUESS** on the internal structure of an
increment — this project has never needed to parse one, and the claim here is only
that it cannot stand alone.

### `wa.db` will disappoint you

`wa.db` is WhatsApp's contact database on the phone, and it is backed up beside
`msgstore.db`. In a backup it is empty of contacts.

| Table in a backed-up `wa.db` | Rows |
|---|---|
| `wa_contacts` | 0 |
| `wa_address_book` | 0 |
| `wa_contact_details` | 0 |
| `wa_org_contacts` | 0 |
| `wa_biz_profiles` | 0 |
| `wa_vnames` | 0 |
| `wa_group_descriptions` | 0 |

**CONFIRMED** on the real archive: all 99 tables are present with their full
schema, and the ones that would hold names hold nothing. This is deliberate on
WhatsApp's part — contacts are rebuilt from the phone's own address book on restore —
and it is the single reason an archive read from a backup alone shows phone numbers
where it should show names. The names have to come from somewhere else; see
[§5](#5-identity-jid-lid-and-the-recovery-of-a-name).

Two practical notes for anyone reading these files:

- `wa.db` and `chatsettingsbackup.db` are in **WAL** journal mode. A copy taken
  without its `-wal` sidecar must be opened with `immutable=1`, or SQLite will refuse
  it or silently show stale rows. `msgstore.db` is in `delete` mode and has no such
  problem.
- Open everything read-only. Amberkeep uses `mode=ro` *and*
  `_pragma=query_only(1)`, so neither the reader nor the driver's own bookkeeping can
  write to a file it was handed.

---

## 3. The crypt15 container

A crypt15 file is a protobuf header, a single AES-GCM ciphertext, and an optional
checksum.

```
[varint: header length]
[BackupPrefix protobuf, `header length` bytes]
[AES-256-GCM ciphertext .............................]
[16-byte GCM authentication tag]
[16-byte MD5]                       <- optional, single-file backups only
```

**CONFIRMED** end to end: `internal/crypt15` decrypts a real 247 MB backup to a byte
-identical match against `wa-crypt-tools`, and the header parser has a fuzz test.

### The header

`BackupPrefix` is a plain protobuf message. Only the fields that change behaviour are
read; everything else is skipped by wire type, and an unknown wire type aborts rather
than being ignored — a file we cannot fully parse is a file we should not try to
decrypt.

| Field | Wire type | Meaning |
|---|---|---|
| 1 | varint | key type, older builds |
| 2 | message | `crypt14KeyData` — its presence means crypt12/crypt14 |
| 3 | message | `e2eeKeyData` — crypt15; field 1 inside it is the 16-byte nonce |
| 4 | message | backup metadata: version, expiry |
| 5 | message | passkey metadata — its presence means a passkey backup |
| 6 | varint | key type, current builds |

The header length is bounded at 64 KiB before any conversion, so a hostile varint
cannot be used to address past the end of the file. Real headers are a few dozen bytes.

When the nonce is absent, the most actionable diagnosis is reported first: passkey,
then crypt14, then malformed.

### Key derivation

```
payload key = HKDF-SHA256(
    secret = the 32-byte root key,
    salt   = empty,
    info   = "backup encryption",
    length = 32)
```

**CONFIRMED**, and fixed by a golden test. The info string is load-bearing: change a
character and every derived key changes.

The 64-digit key the app displays is the root key in hexadecimal. Amberkeep's parser
strips spaces, dashes, tabs, newlines and colons, so the eight groups of eight can be
pasted exactly as written, and accepts either case. A raw 32-byte
`encrypted_backup.key` file works too.

### The payload

- **AES-256-GCM** with a **16-byte nonce**, not the 12 bytes AES-GCM defaults to. In
  Go this needs `cipher.NewGCMWithNonceSize`; most libraries need the equivalent.
  **CONFIRMED** — the 12-byte default cannot decrypt a real file.
- No additional authenticated data.
- A failed tag check means "wrong key or damaged file" and nothing more precise: the
  two are indistinguishable, and the cause is deliberately dropped from the error
  because this path is reached with attacker-influenced input.
- The plaintext is usually **zlib**-compressed. Usually, not always, so the first byte
  decides (`0x78` at every compression level WhatsApp uses) rather than an assumption.
  Decompression is capped at 8 GiB and the stream's own checksum is verified on close.

### The MD5 footer

The trailing 16 bytes are an MD5 of everything before them, present on single-file
backups and absent on chunked ones. A GCM tag and an MD5 digest are both 16 opaque
bytes, so the only reliable way to tell them apart is to recompute the digest: if it
matches, strip it; if not, those bytes were ciphertext. **CONFIRMED** on a real file.
It is an integrity marker, not a security control — the GCM tag is what actually
authenticates the payload.

### What comes out

A plain SQLite database. On the real archive, 236 MB of ciphertext becomes 466 MB of
database in about five seconds. It arrives with no indexes at all, which matters
enough to have its own section: [§10](#10-performance).

---

## 4. Two schema generations, and why we introspect

### Modern (2021 onwards)

Normalised. Three tables carry the structure and around twenty more carry content.

```
jid    (_id, user, server, raw_string, ...)        every address, once
chat   (_id, jid_row_id, subject, ...)             every conversation
message(_id, chat_row_id, sender_jid_row_id, ...)  every message
message_media, message_quoted, message_system, ... one per kind of content
```

Everything is joined by integer row identifiers. A person is `jid._id`; a
conversation is `chat._id`; a message is `message._id`. Nothing stores an address as
a string except `jid` itself.

### Legacy (before 2021)

One flat table.

```
messages(_id, key_remote_jid, key_from_me, key_id, data, timestamp, media_wa_type, ...)
```

The conversation is identified by a JID string repeated on every row
(`key_remote_jid`), and there is no `chat` table. Amberkeep detects this layout and
refuses it explicitly with `ErrLegacyUnsupported` rather than half-reading it. That is
a deliberate gap, not an oversight: a wrong archive is worse than an honest refusal.

### Telling them apart

Not by a version number. By what the file contains.

| Test | Verdict |
|---|---|
| `message.chat_row_id` exists **and** `chat` exists **and** `jid` exists | modern |
| `messages.key_remote_jid` exists | legacy |
| neither | not a WhatsApp message database |

There *is* a version-looking value in the database, and it is a good illustration of
why it is not used. `props` holds a row keyed `msgtore_db_schema_version` — WhatsApp's
own typo — and on the real archive its value is `-2071905192`. That is a hash, not an
ordinal: it cannot be compared, ordered or reasoned about. `PRAGMA user_version` is 1
and has presumably been 1 for a decade. **CONFIRMED**, and it settles the question:
introspect.

### How introspection works

At open, the reader lists every non-virtual table from `sqlite_master`, reads each
one's columns with `PRAGMA table_info`, and records every index name. Three rules fall
out of that:

- **Virtual tables are skipped.** A real archive carries several FTS3/FTS4 full-text
  indexes (`message_ftsv2`, `message_newsletter_fts`, `ai_thread_info_fts` — 3 of the
  311 tables). A pure-Go SQLite build without those modules cannot even ask what
  columns they have. They hold no content of their own, only an index over content
  stored elsewhere, so skipping them costs nothing.
- **A table that cannot be described is skipped, not fatal.** One table we cannot use
  is one table we cannot use. It is not a reason to refuse somebody's whole history.
- **Every query is built from what is there.** Helpers: `has(table)`,
  `hasColumn(table, col)`, `pick(table, candidates...)` for a column that has been
  renamed, and `columnOrNull(table, col)`, which emits the SQL literal `NULL` when a
  column is absent so that a projection keeps a stable shape across versions.

The consequence worth stating plainly: this reader can open a database written by a
WhatsApp release nobody has seen, and will lose only the specific details whose
columns changed name.

---

## 5. Identity: `jid`, `@lid` and the recovery of a name

This is the part that decides whether an archive reads like a conversation or like a
list of phone numbers, and it is where most of this project's difficulty lives.

### `jid`

```sql
CREATE TABLE jid (_id INTEGER PRIMARY KEY AUTOINCREMENT, user TEXT NOT NULL,
                  server TEXT NOT NULL, agent INTEGER, type INTEGER,
                  raw_string TEXT, device INTEGER)
```

`user@server`. The server decides what the address refers to.

| `server` | Refers to | Rows in the real archive |
|---|---|---|
| `s.whatsapp.net` | a person, addressed by phone number | 49,894 |
| `lid` | a person, behind a hidden identifier | 49,787 |
| `g.us` | a group | 453 |
| `newsletter` | a channel | 250 |
| `temp` | a placeholder | 56 |
| `broadcast` | a broadcast list, and `status@broadcast` | 18 |
| `bot` | an assistant | 2 |
| `lid_me`, `status_me`, `hosted`, `hosted.lid` | the owner's own identities and hosted variants | 1 each |

100,464 rows in total for an archive with 4,286 conversations: the table accumulates
every address ever seen, including everyone in every group. Amberkeep loads the whole
table into memory at open — it is a few megabytes — because every reference to a
person anywhere else in the database is a row identifier into it.

A `device` or `agent` suffix (`34600111222:12@s.whatsapp.net`) identifies a particular
linked device, not a different person, and is stripped when parsing.

### What an `@lid` is, and why it exists

A **LID** — "linked identity" — is an opaque per-user identifier that stands in for a
phone number. WhatsApp introduced it so that people can interact in groups, channels
and communities without exposing their phone number to everybody present, and has been
migrating existing conversations onto it. The migration is visible in the archive's own
bookkeeping: `props` carries rows named
`lid_migration_phone_number_hiding_migration_task`,
`local_chat_db_lid_migration`, `ChatLidMigrationState_GlobalChatDbMigration`,
`StatusLidMigrationTask_are_statuses_lid_based` and several more.

The practical effect on a reader is blunt. In the real archive there are almost exactly
as many `lid` addresses as phone addresses (49,787 against 49,894), and 1,123 of the
4,286 conversations are keyed by a hidden identifier. Read naively, a quarter of the
archive is signed by strangers.

### The three sources of a name inside the database

**`jid_map`** links a hidden identifier to the phone address behind it.

```sql
CREATE TABLE jid_map (lid_row_id INTEGER PRIMARY KEY NOT NULL,
                      jid_row_id INTEGER NOT NULL, sort_id INTEGER)
```

Both columns are row identifiers into `jid`. 4,766 rows. The reader records each pair
as an alias in both directions, so a name found against either address is found
against both.

**`lid_display_name`** is the push-name store: the name a person chose for themselves
and broadcasts with their messages.

```sql
CREATE TABLE lid_display_name (lid_row_id INTEGER PRIMARY KEY NOT NULL,
                               display_name TEXT NOT NULL, username TEXT)
```

3,431 rows, 3,415 with a non-empty name, 22 with a username. It is keyed by the hidden
identifier, which is the important detail: a name recorded here can be reached from a
phone-keyed conversation by going through `jid_map` backwards
(`jid_map.jid_row_id = chat.jid_row_id`, then `lid_display_name.lid_row_id = jid_map.lid_row_id`).

**`props`** holds the archive owner's own name under the key `user_push_name`. Its
absence is not a failure to open — an archive whose author is labelled generically is
still complete — so a miss here is swallowed deliberately.

### The fourth source: the phone's address book

Not in the database at all. `wa_contacts` is empty in a backup ([§2](#wadb-will-disappoint-you)),
so the only saved names available are the ones on the phone, reachable through
Android's contacts provider with a read-only `adb content query` and exported as
vCard. Amberkeep takes that file with `--contacts` and matches on normalised phone
number, with a `--country` dialling code for local-format numbers.

### Resolution order

1. A saved name from the phone's address book.
2. `lid_display_name.display_name` — the person's own push name, conventionally shown
   with a `~` prefix to mark that it is self-chosen and unverified.
3. `+<phone number>`.
4. The raw address.

A group is named by `chat.subject` instead, and a direct conversation by the best name
known for the other person.

### What it recovers, measured

| Measure | Count |
|---|---|
| Conversations keyed by a hidden identifier, with messages | 1,079 |
| ...with a `jid_map` entry to a phone address | 587 |
| ...with a push name in `lid_display_name` | 826 |
| ...with **neither** | **0** |
| Phone-keyed conversations that gain a push name through the reverse mapping | 354 |
| Distinct senders across group conversations | 2,309 |
| ...resolving to a phone address, directly or through `jid_map` | 2,306 (99.9%) |

**CONFIRMED.** Every hidden conversation in this archive is identifiable by one route
or the other, and three group senders out of 2,309 are not. The two mechanisms cover
different sets — 587 and 826 out of 1,079 — which is why a reader must consult both,
and why `NOTES-real-schema.md` recorded the reverse mapping as a follow-up: it is
worth 354 conversations on its own.

### Group membership

```sql
CREATE TABLE group_participant_user (_id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_jid_row_id INTEGER NOT NULL, user_jid_row_id INTEGER NOT NULL,
  rank INTEGER, pending INTEGER, add_timestamp INTEGER, label TEXT, ...)
```

6,219 rows. `rank > 0` means an administrator — **BEST GUESS**, consistent across
published decoders, with nothing in the archive to corroborate the exact value. The
reader loads the whole table in one pass rather than one query per group. A legacy
`group_participants(gjid, jid, admin)` table also survives in this archive with 1,335
rows; it is not read.

---

## 6. The message row

```sql
CREATE TABLE message (
  _id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_row_id INTEGER NOT NULL,
  from_me INTEGER NOT NULL,
  key_id TEXT NOT NULL,
  sender_jid_row_id INTEGER,
  status INTEGER, broadcast INTEGER, recipient_count INTEGER,
  participant_hash TEXT, origination_flags INTEGER, origin INTEGER,
  timestamp INTEGER, received_timestamp INTEGER, receipt_server_timestamp INTEGER,
  message_type INTEGER, text_data TEXT, starred INTEGER, lookup_tables INTEGER,
  sort_id INTEGER NOT NULL DEFAULT 0, message_add_on_flags INTEGER,
  view_mode INTEGER, translated_text TEXT, view_replies_thread_id INTEGER,
  server_sts INTEGER)
```

### The columns the reader uses

| Column | Meaning | Confidence |
|---|---|---|
| `_id` | The message's identity, and the tiebreaker in the paging cursor. | CONFIRMED |
| `chat_row_id` | `chat._id`. Not a foreign key — see below. | CONFIRMED |
| `from_me` | 1 when the owner wrote it. An outgoing message is never attributed to anyone else. | CONFIRMED |
| `key_id` | WhatsApp's own message identifier, unique within a conversation and the key for deduplication when merging archives. | CONFIRMED |
| `sender_jid_row_id` | `jid._id` of the author. See the note on zero below. | CONFIRMED |
| `timestamp` | **Milliseconds** since the Unix epoch, UTC. Not seconds. | CONFIRMED |
| `message_type` | What the message is. [§7](#7-message-type-codes). | CONFIRMED |
| `text_data` | The words. For an image or video this is the caption. | CONFIRMED |
| `origin` | How the message was created. Distinguishes a voice note from an audio file. | CONFIRMED for value 1 |
| `starred` | 1 when starred. 1,051 rows in the real archive. | CONFIRMED |
| `origination_flags` | A bit field. Bit 0 means forwarded. | CONFIRMED, see below |

Columns deliberately not read: `status` and the two receipt timestamps (delivery state,
not history), `sort_id` (the app's own display ordering), `translated_text` (a local
convenience, empty here), `participant_hash`, `lookup_tables`, `view_mode`,
`server_sts`.

### `timestamp` is milliseconds, and it collides

Zero and absent both mean "no time recorded", which is normal for a conversation that
was never opened. The real archive spans 1427814484000 to 1789079690000, that is
2015-03-31 to 2026-09-10.

**31,645 messages share a timestamp with another message in the same conversation.**
That is why the paging cursor is `(timestamp, _id)` and not `timestamp` alone: ordering
by time alone is not a total order, and a page boundary that landed inside a group of
equal timestamps would drop or repeat messages. **CONFIRMED**, and the reason the
real-data test asserts that no message is ever yielded twice.

### `sender_jid_row_id` is 0, not NULL

696,414 of 1,124,291 rows — 62% — carry `sender_jid_row_id = 0`, and `jid` has no row
with `_id = 0` (the column is `AUTOINCREMENT`, so ids start at 1). Exactly one row in
the whole archive is genuinely `NULL`. Zero is WhatsApp's "not applicable": the message
is the owner's own, or it is in a one-to-one conversation where there is only one
person it could be from.

A reader must therefore treat "0" and "absent" alike, and fall back to the
conversation's own address for a direct chat. Amberkeep does this by looking the row
identifier up in the `jid` map and falling through when it is not found, which is
correct but incidental; a lookup miss and a zero take the same path. Two incoming group
messages in this archive have `sender_jid_row_id = 0` and are consequently
unattributed. **CONFIRMED.**

### `origin`

| Value | Rows | Meaning | Confidence |
|---|---|---|---|
| 0 | 1,085,107 | ordinary | CONFIRMED |
| 1 | 28,183 | recorded in the app — for `message_type = 2`, a voice note | CONFIRMED |
| 12 | 5,348 | UNKNOWN | |
| 3 | 2,874 | UNKNOWN | |
| 2 | 1,214 | UNKNOWN | |
| 29, 31, 30, 5, 4 | 788, 493, 147, 135, 1 | UNKNOWN | |

The one that matters: of 19,163 audio messages, 18,392 have `origin = 1` and 554 have
`origin = 0`. Nothing else in the row distinguishes a voice message from a shared song,
and the difference matters enough to read very differently in an archive. The remaining
217 audio messages have `origin` 3 or 2 and are treated as audio files. **CONFIRMED**
for value 1; everything else UNKNOWN and carried through.

### `origination_flags`

A bit field, mostly zero.

| Value | Rows |
|---|---|
| 0 | 1,098,728 |
| 1 | 9,358 |
| 67108864 | 6,775 |
| 32768 | 6,088 |
| 131072 | 1,496 |
| 512 | 481 |
| 2 | 350 |
| 4398113619968 | 292 |
| 256 | 132 |
| (a long tail) | |

**Bit 0 means forwarded — CONFIRMED**, and this is a finding this archive settles that
earlier notes left open. 9,547 rows have bit 0 set. 8,319 rows exist in
`message_forwarded`. **All 8,319 of them have bit 0 set, and none is missing it.**
Perfect containment in one direction is strong evidence that the flag means what the
side table means; the 1,228 messages with bit 0 and no `message_forwarded` row are
forwards whose forward count was never recorded, which is consistent with the side
table being an optional refinement rather than the fact itself.

Every other bit is **UNKNOWN**. The reader tests bit 0 only.

### `chat_row_id` is not a foreign key

836 distinct `chat_row_id` values in `message` have no matching row in `chat`, holding
**2,809 messages** between them. SQLite does not enforce referential integrity here and
WhatsApp does not either: these are messages whose conversation was deleted, plus one
sentinel row (`_id = 1`, `chat_row_id = -1`, every other column `NULL`) that WhatsApp
appears to seed the table with.

This is exactly the difference between the archive's two headline numbers:

```
1,124,291  rows in message
   -2,809  in conversations that no longer exist
= 1,121,482  messages an archive can actually show
```

**CONFIRMED.** A reader that iterates `chat` and asks for each conversation's messages
— which is what Amberkeep does — never sees the orphans, which is the right outcome:
there is no conversation to put them in. A reader that iterates `message` directly must
decide what to do with them, and should say how many it dropped.

Orphans in the other direction exist too and are listed in [§8](#8-everything-beyond-words):
305 `message_media` rows, 79 `message_thumbnail` rows, 29 `message_text` rows and 27
`message_system` rows point at message identifiers that are gone.

### Paging

Messages are read in pages of 2,000, ordered by `(timestamp, _id)`, with the cursor
carried forward:

```sql
WHERE chat_row_id = :chat
  AND (timestamp > :at OR (timestamp = :at AND _id > :id))
ORDER BY timestamp, _id
LIMIT :limit
```

Reading backwards flips every comparison and the sort, and an empty cursor becomes
`MaxInt64` so that one query shape serves both directions and every page.

**Offsets would be wrong, for two separate reasons.** The cheap one is cost: `OFFSET n`
makes SQLite produce and discard `n` rows, so page 50 of a conversation costs fifty
times page 1, and the archive's largest conversation holds 92,180 messages. The
correctness one is worse: an offset is a position in a result set, not in the data, so
if anything changes underneath — and the same file is read by several commands — rows
shift between pages and a message is shown twice or not at all. A keyset cursor names a
*message*, so a page always resumes exactly where the last one stopped.

---

## 7. Message type codes

`message.message_type` is a small integer. There is no public specification. The
mapping below comes from **IPED's Android extractor**, the most complete public decoder
and the only one verified against real seized devices, cross-checked against
**WhatsApp-Chat-Exporter**, **whapa** and a decompiled build of the app.

Then it was checked against this archive. Several codes are not merely plausible but
**proved**, because a side table's row count matches a type's message count exactly.

### The table

Distribution is over all 1,124,291 rows.

| Code | Meaning | Rows | Confidence |
|---:|---|---:|---|
| 0 | text | 990,879 | CONFIRMED |
| 1 | image | 59,050 | CONFIRMED (all have a `message_media` row) |
| 2 | audio — voice note when `origin = 1`, audio file otherwise | 19,163 | CONFIRMED |
| 3 | video | 3,669 | CONFIRMED |
| 4 | contact card | 386 | CONFIRMED (`message_vcard`) |
| 5 | location | 406 | CONFIRMED (`message_location`) |
| 7 | system notice | 28,909 | CONFIRMED (`message_system`, exactly) |
| 9 | document | 3,388 | CONFIRMED |
| 10 | missed call | 133 | CONFIRMED (`missed_call_logs`, exactly) |
| 11 | call | 66 | BEST GUESS |
| 13 | GIF | 791 | CONFIRMED |
| 14 | several contact cards | 20 | CONFIRMED (`message_vcard`, several rows per message) |
| 15 | deleted by its sender | 3,156 | CONFIRMED (`message_revoked`) |
| 16 | live location | 51 | CONFIRMED (`message_location`) |
| 20 | sticker | 10,416 | CONFIRMED |
| 24 | group invitation card | 13 | CONFIRMED (`message_group_invite`, exactly) |
| 25 | business list message | 4 | BEST GUESS |
| 27 | business buttons message, including one-time codes | 25 | BEST GUESS |
| 28 | announcement from WhatsApp's own account | 26 | BEST GUESS |
| 32 | reply quoting a template message | 1 | BEST GUESS |
| 36 | disappearing-messages timer set, direct chat | 1 | BEST GUESS |
| 42 | view-once image | 1,961 | CONFIRMED (carries media) |
| 43 | view-once video | 142 | CONFIRMED (carries media) |
| 45 | interactive business message | 6 | BEST GUESS |
| 46 | reply quoting one | 2 | BEST GUESS |
| 49 | another form of interactive reply | 4 | BEST GUESS |
| 55 | business carousel | 1 | BEST GUESS |
| 64 | deleted by a group administrator | 27 | CONFIRMED, see below |
| 66 | poll | 804 | CONFIRMED (`message_poll`) |
| 82 | view-once audio | 9 | BEST GUESS |
| 90 | call log entry | 178 | CONFIRMED (`message_call_log`, exactly) |
| 92 | event | 1 | BEST GUESS |
| 99 | album container | 539 | CONFIRMED (`message_album`, exactly) |
| 106 | poll, a second encoding | 1 | CONFIRMED (`message_poll`) |
| 112 | advanced chat privacy turned on or off | 5 | BEST GUESS |
| 116 | a row the app itself never displays | 41 | BEST GUESS |
| 62 | **UNKNOWN** — 7 rows, all with text and a media row | 7 | UNKNOWN |
| 81 | **UNKNOWN** — 2 rows, both with a media row | 2 | UNKNOWN |
| 110 | **UNKNOWN** — 1 row, and it has an edit record | 1 | UNKNOWN |
| 118 | **UNKNOWN** — 6 rows, no text, no media | 6 | UNKNOWN |
| `NULL` | the sentinel row, `_id = 1`, `chat_row_id = -1` | 1 | CONFIRMED |

### The cross-checks that make "CONFIRMED" mean something

Several side tables correspond one-to-one with a type, with no residue at all:

| Side table | Rows | Message types found in it |
|---|---:|---|
| `message_location` | 457 | 5 (406) + 16 (51) = 457 |
| `message_poll` | 805 | 66 (804) + 106 (1) = 805 |
| `message_album` | 539 | 99 (539) |
| `message_call_log` | 178 | 90 (178) |
| `missed_call_logs` | 133 | 10 (133) |
| `message_revoked` | 3,183 | 15 (3,156) + 64 (27) = 3,183 |
| `message_group_invite` | 13 | 24 (13) |
| `message_system` | 28,936 | 7 (28,909) + 27 orphans |

And the sharpest one: of the 3,183 rows in `message_revoked`, exactly 27 carry an
`admin_jid_row_id`, and those 27 are exactly the 27 messages of type 64. **Code 64 is
"deleted by an administrator" — CONFIRMED**, not by a decoder's say-so but by the
database agreeing with itself.

### Codes deliberately left unrecognised

Four codes occur here that no source names: **62, 81, 110 and 118**, 16 messages in
total. They are not guessed at. `kindOf` returns `KindUnknown`, the message is carried
through with its original number in `SourceType`, any text or media it has is still
recovered, and `amberkeep inspect --full` reports the code and the count so the mapping
can be extended from real archives rather than from hope.

This is a deliberate policy and it is worth stating: **a wrong label is worse than an
honest one.** The message survives either way; only the description differs. Guessing
buys a slightly tidier export and risks telling somebody their message was something it
was not.

### One code that means different things on different platforms

Do not carry this table over to an iPhone store.

| Code | Android | iPhone |
|---|---|---|
| 46 | reply to an interactive message | **poll** |
| 66 | **poll** | **album** |

The numbers were assigned independently. A reader that shares a mapping between the two
platforms will silently mislabel a meaningful number of messages.

---

## 8. Everything beyond words

WhatsApp keeps each kind of content in its own table. A reader that stops at the
`message` table turns a shared place, a poll, a call, a contact card and a photograph
into a blank line.

Every recovery below has the same shape: **one query per page of 2,000 messages, never
one per message, and never a join onto the main query.** A message can carry several
reactions or several contact cards, and joining those would multiply the message rows
and silently duplicate history. The order of the loaders matters in two places — quotes
are read before quoted attachments, which fill them in, and the media row is read
before the embedded preview, which may have to create an attachment the media table
never had.

Row counts and orphan counts below are from the real archive.

### 8.1 Media — `message_media`

**99,041 rows, 305 orphaned.** Joined on `message_row_id`, one row per message.

Columns read: `mime_type`, `media_name`, `media_caption`, `media_duration` (seconds),
`file_size`, `width`, `height`.

Present but not read: `file_path`, `file_hash`, `enc_file_hash`, `media_key`,
`direct_path`, `file_length`, `page_count`, `first_scan_sidecar`,
`raw_transcription_text`, `accessibility_label`, `sticker_flags`, and about forty more.

Two findings worth carrying:

- **Where a caption lives has changed.** For images, videos and GIFs in this archive,
  `media_caption` is empty in every single row, and the caption is in
  `message.text_data` — 7,977 of 59,050 images have one. Only documents use
  `media_caption` (423 of 3,388), and even there 380 of those messages *also* have
  `text_data`, of which only 90 are identical. The reader reads `text_data` first and
  falls back to `media_caption`, which is the right order for a modern database; the
  code comment beside it describes the older arrangement. **CONFIRMED.**
- **`file_size` is not the only size column.** 78,604 rows have `file_size > 0`, and a
  further **20,429 have `file_length > 0` with `file_size` zero or absent**. The reader
  reads only `file_size`, so a fifth of the attachments in this archive report a size of
  zero when the database knows better. **CONFIRMED**, and a straightforward fix:
  `pick("message_media", "file_size", "file_length")`.

`file_path` is a path on the phone (92,759 of 92,941 begin `Media/`), not a file. See
[§11](#11-what-is-not-recoverable).

### 8.2 Thumbnails — `message_thumbnail`

```sql
CREATE TABLE message_thumbnail (message_row_id INTEGER PRIMARY KEY, thumbnail BLOB)
```

**3,832 rows, 3,787 with bytes, 16.4 MB in total, 79 orphaned.** Average 4.3 KB.

The reader creates an attachment for a thumbnail even when the message has no
`message_media` row, because that is precisely the case where the preview is the only
thing left.

**What this table actually holds, in this archive, is not what the code comment beside
it claims.** Broken down by the type of message it belongs to:

| Message type | Thumbnails |
|---|---:|
| 0 (text) | 3,247 |
| 5 (location) | 381 |
| 16 (live location) | 49 |
| 15 (deleted) | 12 |
| 62, 64, 24, 81 | 19 between them |
| **image, video, GIF, sticker** | **0** |

Only 64 messages in the entire archive have both a `message_media` row and a
`message_thumbnail` row. These are link-preview images and map snapshots, not
photographs. The claim in `content.go` that this recovery saves "a decade of
photographs" is **not supported by this archive** — a reader should expect link
previews and map tiles here. **CONFIRMED** by the breakdown above.

Two more thumbnail stores exist, and the reader uses one of them:

| Table | Rows with bytes | Bytes | Read? |
|---|---:|---:|---|
| `message_thumbnail` | 3,787 | 16.4 MB | yes |
| `message_quoted_media.thumbnail` | 7,322 | 16.3 MB | yes |
| `message_quoted_text.thumbnail` | 719 | | no |
| `message_quoted_location.thumbnail` | 36 | | no |
| `media_hash_thumbnail` | 3,851 | 28.2 MB | **no** |

`media_hash_thumbnail(media_hash TEXT PRIMARY KEY, thumbnail BLOB)` is keyed by content
hash rather than by message, and joins to `message_media.file_hash`. On this archive
that join reaches **8,802 messages** — 8,792 stickers and 10 deleted messages — carrying
28 MB of images the reader currently ignores. It is the largest single unexploited
recovery found while writing this document. **CONFIRMED** by the join; whether it holds
photographs on other archives is UNKNOWN.

### 8.3 Quotes — `message_quoted`

**89,844 rows, no orphans.** One row per replying message, holding a *copy* of what was
quoted rather than a pointer to it — which is why a reply survives even when the
message it answered has been deleted.

Columns read: `from_me`, `sender_jid_row_id`, `message_type`, `text_data`. Also present:
`chat_row_id`, `key_id`, `timestamp`, `quoted_type`.

The quoted message's type is run through the same `kindOf` mapping, with `origin` 0
because the quoted copy does not carry one.

### 8.4 Quoted media — `message_quoted_media`

**8,940 rows, no orphans, 7,322 carrying thumbnail bytes.** Read after the quotes it
fills in, and skipped for any message that has no quote.

Columns read: `mime_type`, `media_name`, `media_caption`, `media_duration`, `thumbnail`.

This is where a conversation about a photograph keeps its other half. Without it, one
side of every such exchange is blank.

### 8.5 Reactions and other add-ons — `message_add_on`

**28,320 rows.** The general mechanism for things attached to a message after it was
sent. It is the one side table that is *not* keyed by `message_row_id`:

```sql
CREATE TABLE message_add_on (_id INTEGER PRIMARY KEY AUTOINCREMENT, chat_row_id INTEGER,
  from_me INTEGER, key_id TEXT NOT NULL, sender_jid_row_id INTEGER,
  parent_message_row_id INTEGER, timestamp INTEGER, status INTEGER,
  message_add_on_type INTEGER, ...)
```

The link is **`parent_message_row_id`**. This matters for indexing; see
[§10](#10-performance).

`message_add_on_type` selects a detail table:

| `message_add_on_type` | Rows | Detail table | Rows | Read? |
|---:|---:|---|---:|---|
| 67 | 16,261 | `message_add_on_poll_vote` | 16,261 | no |
| 56 | 10,462 | `message_add_on_reaction` | 10,462 | yes |
| 74 | 1,584 | *(edits — see `message_edit_info`)* | | indirectly |
| 93 | 8 | `message_add_on_event_response` | 8 | no |
| 79 | 4 | `message_add_on_pin_in_chat` | 4 | no |
| 68 | 1 | `message_add_on_keep_in_chat` | 1 | no |

The correspondence between type 56 and `message_add_on_reaction`'s row count is exact,
and between type 67 and `message_add_on_poll_vote` likewise. **CONFIRMED.** The
remaining type numbers are **BEST GUESS** by elimination; `message_add_on_receipt_device`
(27,865 rows) is delivery bookkeeping, not content.

`message_add_on_poll_vote` and `message_add_on_poll_vote_selected_option` (17,366 rows)
hold **who voted for what** in a poll. The reader does not read them; it recovers only
the per-option totals. That is a deliberate gap, not an oversight, but it is a gap.

Reactions are read by joining `message_add_on` to `message_add_on_reaction` on
`message_add_on._id`, filtering out null and empty emoji (an empty reaction is a
reaction that was removed). Each carries `from_me`, `sender_jid_row_id` and
`sender_timestamp`.

### 8.6 Mentions — `message_mentions`

**2,575 rows over 2,058 messages, no orphans.** Columns: `message_row_id`,
`jid_row_id`. There is a `display_name` column and it is empty in every row, so the name
has to be resolved through the directory like any other address.

### 8.7 Polls — `message_poll` and `message_poll_option`

**805 polls, 4,251 options, no orphans.** The question is the message's own `text_data`;
the poll row holds only the metadata.

`message_poll`: `selectable_options_count` (how many answers a voter could pick),
`end_time` (non-zero means closed — zero in all 805 rows here, so **BEST GUESS**).

`message_poll_option`: `option_name`, `vote_total` (non-zero on 2,914 of 4,251),
`option_sha256`, `option_hash`, `contributor_jid_row_id`, `added_timestamp_ms`. Read in
`_id` order so the answers keep the order they were offered in.

A database can hold options for a poll whose own row is missing. The reader keeps them
and synthesises a header rather than discarding the answers for want of one.

### 8.8 Location — `message_location`

**457 rows, no orphans**, exactly the 406 messages of type 5 plus the 51 of type 16.

Columns read: `latitude`, `longitude`, `place_name` (20 rows), `place_address`, `url`,
`live_location_share_duration` (> 0 on 51 rows, and those are exactly the type-16 live
locations). **CONFIRMED.**

### 8.9 Contact cards — `message_vcard` and `message_vcard_jid`

**433 cards across 406 messages** — the 386 of type 4 and the 20 of type 14, which
confirms type 14 as "several cards in one message". No orphans.

The vCard is kept verbatim; only the `FN:` line is pulled out for a readable label, and
only for display. `message_vcard_jid` (412 rows over 336 messages) records that a card
was matched to a WhatsApp address, which links a shared card to a real conversation.

**A known imprecision:** `message_vcard_jid` does not say *which* card an address
belongs to when several were sent together, so the reader attributes them all to the
first card. For the 20 multi-card messages here that is a guess. It is recorded in the
code and repeated here so nobody mistakes it for a fact.

### 8.10 Link previews — `message_text`

**4,447 rows, 29 orphaned.** Despite the name, this table holds what WhatsApp showed
beneath a shared link: `url` (3,355 rows), `page_title` (4,386), `description` (3,622).
The messages are types 0 and 15.

This is often the only surviving record of what was actually shared, because the page
itself may have changed or gone. A preview with all three fields empty is discarded
rather than attached as an empty object.

### 8.11 Calls — `call_log`, `message_call_log`, `missed_call_logs`

Two routes, read in order.

`message_call_log(message_row_id, call_log_row_id)` — **178 rows** — joins a message to
`call_log` — **1,060 rows**, of which the rest belong to call history with no message.
Columns read from `call_log`: `video_call`, `duration` (seconds), `call_result`,
`group_jid_row_id`.

`missed_call_logs` — **133 rows**, exactly the 133 messages of type 10 — is the fallback
and is only applied to a message that got nothing from the first route.

`call_result` is interpreted, but **duration is trusted first**: a call that lasted was
answered under any version's numbering, and the result codes shift between releases.

| `call_result` | Rows | Read as | Confidence |
|---:|---:|---|---|
| 5 | 569 | connected | BEST GUESS |
| 2 | 403 | missed | BEST GUESS |
| 4 | 45 | declined | BEST GUESS |
| 3 | 28 | declined | BEST GUESS |
| 0 | 12 | unknown | |
| 7, 6 | 2, 1 | unknown | |

713 of 1,060 calls have `video_call = 1`.

### 8.12 Albums — `message_album`

**539 rows, no orphans**, exactly the 539 messages of type 99. Columns: `image_count`,
`video_count`. The album row is a *container*: the pictures themselves are separate
messages in the same conversation, and the reader records only how many items were sent
together.

### 8.13 Edits — `message_edit_info`

**1,588 rows, no orphans.** Columns: `original_key_id`, `edited_timestamp`,
`sender_timestamp`. The reader picks whichever timestamp column exists.

**WhatsApp keeps only the final wording.** An archive can say that a message was edited
and when, and cannot say what it used to say. That is a property of the format, not a
limitation of this reader. When the timestamp is missing, the row's mere presence is the
fact that matters, so the message's own send time is used rather than losing the edit
entirely.

Mostly text (1,557 of 1,588), plus 27 images, 3 videos and the single type-110 message.

### 8.14 Ephemeral messages — `message_ephemeral`

**142 rows, no orphans.** Column: `duration`, in seconds. Two values occur: 7,776,000
(90 days, 136 rows) and 604,800 (7 days, 6 rows).

Worth correcting a natural assumption: this table is **not** the record of the timer
being changed. Its rows hang off ordinary messages — 126 text, 9 deleted, 3 video, and
one each of sticker, contact, audio and image — and none of them is a system notice. It
marks *a message that is itself set to disappear*. The timer *change* is a separate
thing: message type 36 in a direct chat, or system notice action 56. **CONFIRMED.**

### 8.15 Forwarding — `message_forwarded`

**8,319 rows, no orphans.** Column: `forward_score`, how many hops the message had made
before it arrived.

| `forward_score` | Rows |
|---:|---:|
| 1 | 5,944 |
| 2 | 1,266 |
| 3 | 322 |
| 4 | 179 |
| 127 | 607 |
| 0 | 1 |

127 is a sentinel for "many times forwarded" — the state WhatsApp displays as a
double-arrow — rather than a literal count. **BEST GUESS**, though the gap between 4 and
127 with nothing in between makes it hard to read any other way.

The relationship with `origination_flags` bit 0 is in [§6](#origination_flags): every row
here has the bit set.

### 8.16 Revocations — `message_revoked`

**3,183 rows, no orphans.** Columns: `revoked_key_id`, `admin_jid_row_id`,
`revoke_timestamp`.

The reader sets the message's kind to deleted on the strength of this row, whatever
`message_type` said. 27 rows carry an administrator, and those 27 are exactly the
type-64 messages.

Note what survives: the *fact* of a deletion, its time, and who did it. Not the text.

### 8.17 Group invitations — `message_group_invite`

**13 rows, no orphans**, exactly the 13 messages of type 24. Columns: `group_name`,
`group_jid_row_id`, `expiration`.

---

## 9. System notices

Roughly 2.6% of this archive is WhatsApp talking rather than a person: "you were added
to this group", "your security code changed", "messages are end-to-end encrypted".

**28,936 rows in `message_system`, of which 27 are orphaned; the remaining 28,909 are
exactly the messages of type 7.**

Almost none of them carry text of their own: **28,361 of 28,909 have an empty
`text_data`.** What happened is a number, and the particulars are spread across a dozen
small tables. Any reader that shows only `text_data` shows 28,361 blank lines.

### The structure

```sql
CREATE TABLE message_system (message_row_id INTEGER PRIMARY KEY, action_type INTEGER)
```

That is all. `action_type` selects which detail table, if any, holds the specifics. The
reader queries `message_system` first, keeps the list of message identifiers that turned
out to be notices, and then runs each detail query only when that list is non-empty — a
page with no notices costs one query, not ten.

### Detail tables

Each detail table accompanies a specific set of action codes, and the correspondence
is exact enough to identify several codes on its own. Measured by joining each table
back to `message_system.action_type`:

| Table | Rows | Messages | Action codes found on them | What the reader takes |
|---|---:|---:|---|---|
| `message_system_group` | 3,736 | 3,736 | every group-chat notice (39 codes) | `is_me_joined` |
| `message_system_initial_privacy_provider` | 6,372 | 6,372 | **67 only** | *(not read)* |
| `message_system_chat_participant` | 2,904 | 2,384 | 12, 20, 14, 79, 2, 15, 13, 52, 123-127, 144 | `user_jid_row_id` |
| `message_system_device_change` | 181 | 181 | **57 only** | added and removed device counts |
| `message_system_value_change` | 170 | 170 | 1, 83, 46, 50, 84, 85 | `old_data` |
| `message_system_photo_change` | 135 | 135 | **6 only** | *(not read)* |
| `message_system_number_change` | 76 | 76 | **10 (56) and 28 (20)** | old and new address |
| `message_system_business_state` | 56 | 56 | **69 only** | `business_name` |
| `message_system_linked_group_call` | 28 | 28 | **70 only** | *(not read)* |
| `message_system_username_change` | 26 | 26 | **165 only** | old and new username |
| `message_system_block_contact` | 14 | 14 | **58 only** | `is_blocked` |
| `message_system_with_group_nodes` | 280 | 195 | 108-116, 123-128, 144 | `group_subject` |
| `message_system_group_with_parent` | 5 | 5 | **87 only** | *(not read)* |
| 21 further `message_system_*` tables | 0 | | | present in the schema, empty here |

Eight codes are pinned by a table that accompanies them and nothing else: **6, 57, 58,
67, 69, 70, 87 and 165**. Two more are pinned as a pair: `message_system_number_change`
holds exactly the 56 notices of action 10 plus the 20 of action 28, which settles code
28 as a second encoding of the same event. **CONFIRMED.**

18,065 notices — action 18, the security-code change — have **no detail table at all**,
along with actions 118 (131 rows), 129 (78), 68, 59, 134, 132 and 80.

`is_me_joined` is the one that reads backwards if ignored: without it, a notice about
the owner being added to a group renders as the owner adding themselves.

The **actor** of a notice is the message's own sender — the person who caused it —
which is exactly what every phrasing needs.

`message_system_number_change` is worth its own sentence: without it, a conversation
appears to change person partway through, because the address on later messages is
simply different.

### The action codes

Same provenance as the type codes: IPED first, cross-checked against
WhatsApp-Chat-Exporter, whapa and a decompiled build, then checked against this archive,
where each code's presence alongside the detail table the sources predict is the
corroboration available.

| Code | Meaning | Rows here | Confidence |
|---:|---|---:|---|
| 1 | group subject changed | 110 | value change CONFIRMED (all 110 carry `message_system_value_change`); that the value is the subject is BEST GUESS |
| 4 | added to group, older encoding | 51 | BEST GUESS |
| 5 | left the group | 671 | BEST GUESS |
| 6 | group photo changed | 136 | CONFIRMED — `message_system_photo_change` holds 135 of them and no other code |
| 7 | you were removed | 0 | BEST GUESS |
| 10 | phone number changed | 56 | CONFIRMED — with code 28, exactly fills `message_system_number_change` |
| 11 | group created | 170 | BEST GUESS |
| 12 | added to group | 1,228 | involves participants CONFIRMED (all 1,228 carry `message_system_chat_participant`); "added" rather than "removed" is BEST GUESS |
| 13 | left the group, second encoding | 6 | BEST GUESS |
| 14 | removed from the group | 170 | involves participants CONFIRMED; the direction is BEST GUESS |
| 15 | you are now an administrator | 20 | BEST GUESS |
| 16 | you are no longer an administrator | 0 | BEST GUESS |
| 18 | security code changed | **18,065** | CONFIRMED by ubiquity; no detail table |
| 20 | joined via an invite link | 768 | involves participants CONFIRMED; the invite link is BEST GUESS |
| 27 | group description changed | 53 | BEST GUESS |
| 28 | phone number changed, second encoding | 20 | CONFIRMED — see code 10 |
| 56 | disappearing-messages setting changed | 33 | BEST GUESS |
| 57 | linked devices changed | 181 | CONFIRMED (`message_system_device_change`, exactly) |
| 58 | contact blocked or unblocked | 14 | CONFIRMED (`message_system_block_contact`, exactly) |
| 67 | the end-to-end encryption banner | 6,372 | CONFIRMED (`message_system_initial_privacy_provider`, exactly) |
| 69 | business notice | 56 | CONFIRMED (`message_system_business_state`, exactly) |
| 70 | group call started | 28 | CONFIRMED (`message_system_linked_group_call`, exactly) |
| 79 | joined this group from the community | 131 | BEST GUESS |
| 110 | a community change | 22 | BEST GUESS |
| 118 | a message was pinned | 131 | BEST GUESS |
| 129 | the sender is in your contacts | 78 | BEST GUESS |
| 136 | a contact joined WhatsApp | 0 | BEST GUESS |

Codes 6, 57, 58, 67, 69 and 70 are proved by an exact, exclusive row-count match with
their detail table, and 10 and 28 by jointly filling theirs. That is as good as evidence
gets without a specification. The rest rest on the published decoders, with the detail
table they carry as partial corroboration.

### The codes nobody can name

**58 distinct action codes occur in this archive. The reader phrases 24 of them.**

| | Codes | Rows | Share of notices |
|---|---|---:|---:|
| Phrased | 24 of the table above | 28,570 | 98.7% |
| Named by one weak source, deliberately rejected | 2, 83, 109, 111, 165 | 236 | 0.8% |
| No source at all | 29 further codes | 130 | 0.4% |

The five deliberately rejected, with the reason recorded rather than rediscovered:

| Code | Rows | Why it is not phrased |
|---:|---:|---|
| 2 | 49 | One source calls it group creation, which contradicts code 11. |
| 83 | 38 | One weak source calls it a request to join. |
| 109 | 63 | Sits in the community-management range, unnamed. |
| 111 | 60 | Same. |
| 165 | 26 | Was "newer than every published table" — but see below. |

The 29 with no candidate meaning at all: 21, 29, 31, 46, 50, 52, 59, 68, 80, 84, 85, 87,
91, 99, 108, 115, 116, 123, 124, 125, 126, 127, 128, 131, 132, 134, 138, 144, 167.
Between 1 and 22 rows each. 282 of the 366 unphrased notices are in group conversations,
33 in direct ones and 2 in a channel.

### Four of them can be placed from the evidence in this archive

Not phrased yet, but no longer blind. The join above puts each unnamed code with the
detail table that accompanies it, which is a statement about what *kind* of event it is
even when the exact wording is unknown.

| Code | Rows | Evidence | Reading | Confidence |
|---:|---:|---|---|---|
| **165** | 26 | `message_system_username_change` holds exactly these 26 notices and no others | **a username changed** | CONFIRMED |
| **87** | 5 | `message_system_group_with_parent` holds exactly these 5 and no others | a group was linked to a parent community | CONFIRMED as a community-link event |
| **2** | 49 | carries `message_system_chat_participant` rows, like codes 12, 14 and 20 | somebody was added or removed, not a group creation | supports rejecting the "group created" reading |
| **83, 46, 50, 84, 85** | 24, 19, 8, 7, 2 | carry `message_system_value_change`, like code 1 | some named value changed from an old to a new one | CONFIRMED as a value change |
| **108-116, 123-128, 144** | 1-63 each | carry `message_system_with_group_nodes` with a `group_subject` | community and sub-group management | CONFIRMED as community events |

Code 165 is the sharpest of these, and it is a small embarrassment worth recording: the
reader already *reads* `message_system_username_change` and recovers the old and new
usernames, then renders the result as "Unrecognised notice, code 165" because the
phrasing table does not connect the two. The fact was in the archive the whole time.

**The source comment beside this code says "five codes"; measured across the whole
table, thirty-four fall through.** The five are the ones for which a candidate meaning
existed and was rejected. The other twenty-nine simply have no candidate. The honest
figure is thirty-four, and it is recorded here because the point of this document is to
be checkable.

Every one of them still reaches the reader. `describeUnidentified` states the facts —
"Unrecognised notice from <actor>, code 131" plus whatever targets, subject, business
name or old and new values were recovered — phrased as a *description* rather than as
something somebody said, so nobody mistakes it for a claim about what happened.
`IsIdentifiedNotice` lets a report say honestly how much of an archive was understood.

### Why the facts and the words are kept apart

`notices.go` recovers facts into a `model.Notice`. `phrasing.go` turns a notice into an
English sentence. They are separate files because the sentence has to be translated one
day and the facts do not, and because an interface should be free to phrase a notice
itself rather than parse a string this package wrote.

---

## 10. Performance

### A decrypted backup has no usable indexes

This is the single most consequential fact about reading these files, and it is easy to
miss because SQLite does not complain.

The real archive has **32 index entries in `sqlite_master`, and every one of them is
`sqlite_autoindex_*`** — the implicit index SQLite creates for a `UNIQUE` or composite
`PRIMARY KEY` constraint. Not one index was written by WhatsApp. On the twenty-odd
tables a reader actually touches, exactly one of those automatic indexes is useful
(`message_system_with_group_nodes`, whose primary key happens to start with
`message_row_id`).

The app's own bookkeeping says so too: `props` carries
`MessagesDBHelper_CreateAsyncIndexes = 0` alongside a family of
`db-maint/msgstore.db/CREATE_INDEXES_ASYNC/...` and `CREATE_INDEXES_DEFAULT/...` rows.
WhatsApp treats index creation as a maintenance task performed after a restore, which
is exactly why a backup arrives without them: indexes are derived data, and shipping
them would inflate the backup for no gain. **CONFIRMED.**

### What SQLite says when asked

```
sqlite> EXPLAIN QUERY PLAN
        SELECT ... FROM message
        WHERE chat_row_id = ? AND (timestamp > ? OR (timestamp = ? AND _id > ?))
        ORDER BY timestamp, _id LIMIT 2000;

SCAN message
USE TEMP B-TREE FOR ORDER BY
```

A full scan of 1,124,291 rows, then a temporary B-tree to sort the result, **for every
page of every conversation.**

The conversation list is no better:

```
sqlite> EXPLAIN QUERY PLAN
        SELECT chat._id, ... FROM chat LEFT JOIN
          (SELECT chat_row_id, COUNT(*), MAX(timestamp) FROM message GROUP BY chat_row_id)
          AS stats ON stats.chat_row_id = chat._id ORDER BY stats.last_at DESC;

MATERIALIZE stats
SCAN message
USE TEMP B-TREE FOR GROUP BY
SCAN chat
BLOOM FILTER ON stats (chat_row_id=?)
SEARCH stats USING AUTOMATIC COVERING INDEX (chat_row_id=?) LEFT-JOIN
USE TEMP B-TREE FOR ORDER BY
```

### The arithmetic

Measured read-only on the real archive, best of three runs, warm cache:

| Query | Time |
|---|---:|
| One page of 2,000 messages, first page of the busiest conversation | 34.3 ms |
| The same, from the middle of a 92,180-message conversation | 33.5 ms |
| The whole conversation list | 165.4 ms |

The cost is flat with depth, which is the keyset cursor working as designed — and it is
flat *at the cost of a full table scan*, which is the missing index.

Streaming the archive means 4,289 pages across 3,846 conversations. At 34.3 ms of
scanning each, that is **147 seconds spent scanning a table SQLite has no index for**,
before a single detail query runs. The repository's measured five minutes for a full
export is that number plus everything else, on a pure-Go SQLite that runs roughly 1.5 to
3 times slower than the C library these timings came from.

### What `prepare` builds

`internal/source/android/prepare.go` creates 30 indexes, all prefixed `amberkeep_` so
they can be recognised and removed again without touching anything of WhatsApp's.

| | |
|---|---|
| Indexes built | 30 |
| Time | 1.4 s |
| File growth | 26 MB on a 466 MB file, about six per cent |
| Effect on a full export | **five minutes to fifty-four seconds**, byte-identical output |

The tables are a **written list**, not "every table with a `message_row_id` column":
the real archive has **178** such tables and this reader touches about twenty of them.
Indexing the rest would cost time and around fifty megabytes to speed up queries nobody
runs. Which indexes actually get built is decided from the schema the program just read,
so a table this version has never seen costs nothing and one that has been removed is
skipped.

The important one is the first:

```sql
CREATE INDEX amberkeep_message ON message (chat_row_id, timestamp, _id)
```

Its columns are the filter, the sort key and the tiebreaker, in that order, so a page
becomes a range scan of exactly the rows it returns and the temporary B-tree disappears.

### Most of the other twenty-nine do nothing

This is a finding, not a description of intent, and it comes from reading the query
plans against the real schema.

**23 of the 28 side tables in the written list already have `message_row_id` as their
primary key.** In 22 of them it is declared `INTEGER PRIMARY KEY`, which in SQLite makes
the column an alias for the rowid; in the twenty-third,
`message_system_with_group_nodes`, it is the first column of a composite primary key,
which gets an automatic index. Either way the lookup is already a direct B-tree seek, and
an index on it is a second copy of a key SQLite already has.

```
sqlite> EXPLAIN QUERY PLAN SELECT ... FROM message_media WHERE message_row_id IN (...);
SEARCH message_media USING INTEGER PRIMARY KEY (rowid=?)
```

Same for `message_thumbnail`, `message_quoted`, `message_quoted_media`, `message_system`,
`message_text`, `message_revoked`, `message_forwarded`, `message_edit_info`,
`message_location`, `message_poll`, `message_album`, `message_call_log`,
`message_ephemeral`, `message_group_invite` and every `message_system_*` table read.
`message_add_on_reaction(message_add_on_row_id INTEGER PRIMARY KEY)` likewise.
`message_system_with_group_nodes` is covered by its automatic composite index.

Only **five** tables the reader touches genuinely lack an index on the column it filters
by, and `EXPLAIN QUERY PLAN` says `SCAN` for each: `message_mentions` (2,575 rows),
`message_system_chat_participant` (2,904), `message_poll_option` (4,251),
`message_vcard` (433) and `message_vcard_jid` (412). All five are small enough that a
scan costs 0.1-0.2 ms per page.

| Enrichment query, per page of 2,000 ids | Time |
|---|---:|
| `message_media` (rowid seek) | 0.2 ms |
| `message_thumbnail` (rowid seek) | 0.2 ms |
| `message_quoted` (rowid seek) | 0.2 ms |
| `message_mentions` (full scan) | 0.2 ms |
| `message_poll_option` (full scan) | 0.2 ms |
| `message_vcard` (full scan) | 0.1 ms |
| reactions join (scans `message_add_on`, 28,320 rows) | 0.9 ms |

So: **`amberkeep_message` is worth the entire 5x improvement, and the other 29 indexes
are worth a few milliseconds per page between them.** The honest version of the claim in
the README is "the index on `message` is missing, and it is the one that matters".

### The one index that is genuinely missing

`message_add_on` has **no `message_row_id` column**. Its link to a message is
`parent_message_row_id`. The written list names `message_add_on`, the index planner
checks for `message_row_id`, finds nothing, and silently skips it — so the one side table
that really is scanned on every page never gets the index it needs. The planner confirms
it: `SCAN message_add_on`, 28,320 rows, for every page of every conversation.

The existing test only checks that a planned index names a table that exists, so a table
whose index is never planned at all passes. The fix is one line in `indexesFor`
(`add("message_add_on", "parent_message_row_id")`) and a test that asserts the plan, not
the table list.

### Cache and memory-mapped reads

Separately from indexes, `readingPragmas` in `android.go` sets
`cache_size(-262144)` — 256 MB — and `mmap_size(2147483648)`. The default page cache is
two megabytes, which for a 466 MB database means nearly every page is fetched from the
operating system again the next time it is wanted; that call, not the disk, accounted for
about ninety per cent of the time spent reading an archive. The change took twenty-five
per cent off reading every message. Memory mapping raises the process's virtual size, not
the memory it holds.

### Where writing is allowed to happen

The published guarantee is that originals are never modified, and it is meant. An index
holds no information that is not already in the table beside it, and deleting one again
loses nothing — but it is still a change to a file, so:

- **`decrypt`** adds the indexes to the file it itself just wrote, a second after writing
  it. The encrypted backup is untouched and nothing is being preserved that did not exist
  a moment ago.
- **`prepare`** adds them to a database somebody names. Running the command is the
  consent, and it prints what it is about to change before it does.
- **Every other command** opens read-only, with `query_only(1)` as well, and says when the
  archive it was given could be read six times faster.

Journalling is deliberately left **on** while the indexes are built. Turning it off makes
this about twice as quick, and an interruption would then leave a database holding
somebody's whole history in an undefined state. A second is not worth that.

The test that makes this acceptable hashes every value of every row of every table before
and after, so nothing can change but indexes. It caught its own first version: the rows
are identical but come back in a different order once an index exists, because the planner
then reads through it — which is why it now reads in `rowid` order.

---

## 11. What is not recoverable

Stated plainly, because a tool that quietly omits things is worse than one that says what
it cannot do.

### Media files

`message_media.file_path` holds a path on the phone (`Media/...` for 92,759 of the 92,941
rows that have one), not the bytes. The files live in
`Android/media/com.whatsapp/WhatsApp/Media/` and are **not in the backup**. They must be
copied separately, and on an old archive most of them are long gone.

What survives inside the database is the description — name, type, size, duration,
dimensions — plus the thumbnails in [§8.2](#82-thumbnails--message_thumbnail), which in
this archive are link previews and map snapshots rather than photographs.

**CONFIRMED.**

### Voice-note transcriptions

`message_media` has a `raw_transcription_text` column. In this archive, across 19,163
audio messages and 99,041 media rows, **it is populated zero times**.

WhatsApp's voice transcription runs on the device and the result is not persisted to
`msgstore.db` — at least not in this column, in this version, on this phone. No other
column or table in the 311 present holds transcript text. A reader should not promise
transcriptions. **CONFIRMED for this archive**; whether some other build populates it is
**UNKNOWN**.

### The previous text of an edited message

`message_edit_info` records that an edit happened and when. WhatsApp keeps only the final
wording. **CONFIRMED** — there is no column and no table holding a prior version.

### The text of a deleted message

`message_revoked` records the deletion, its time and, for administrator deletions, who did
it. The words are gone from the database. **CONFIRMED.**

### Who voted for what in a poll

Recoverable, but not by this reader: `message_add_on_poll_vote` (16,261 rows) and
`message_add_on_poll_vote_selected_option` (17,366 rows) hold it. Amberkeep recovers only
per-option totals. A gap, not an impossibility.

### Contacts

`wa.db` is empty of contacts in a backup ([§2](#wadb-will-disappoint-you)). Names come
from the phone's address book or from the push names inside `msgstore.db`, and nowhere
else.

### The pre-2021 flat schema

Detected and refused with `ErrLegacyUnsupported`. A deliberate gap.

### Status updates

The status feed is a pseudo-conversation (`status@broadcast`) and is excluded from an
archive by default. In this archive it holds no messages at all.

### Message content in an increment

`msgstore-increment-*.crypt15` files cannot be applied without the full backup they are
increments against. **BEST GUESS** on their internals.

### Anything in the tables nobody reads

311 tables exist; the reader touches 39 of them. The rest are payments, business profiles,
stickers, AI threads, delivery receipts, ranking signals and migration bookkeeping. Most
are empty. None is known to hold conversation content that is not also in the tables
above — but "none is known to" is not "none does", and that is **UNKNOWN**.

---

## 12. Reporting a schema change

WhatsApp changes these databases every few months. A report from a version nobody here
has seen is the most valuable contribution this project can receive, and it must contain
no message content whatsoever.

### The rule

**Message content never leaves your machine.** Not one message, not one phone number,
not one name, not a "harmless" example, not even in a screenshot. If you are unsure
whether something counts, it counts. The dumps below are structure and counts only, and
you can read every line of them before sending.

### What to send

Everything from a read-only connection. `immutable=1` is safe even for a WAL-mode file
without its sidecar.

**1. Version and shape.**

```sh
sqlite3 "file:msgstore.db?mode=ro&immutable=1" <<'SQL'
.mode list
SELECT 'user_version', * FROM pragma_user_version();
SELECT 'page_size', * FROM pragma_page_size();
SELECT 'schema_version_prop', value FROM props WHERE key = 'msgtore_db_schema_version';
SELECT 'tables', count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%';
SELECT 'indexes', count(*) FROM sqlite_master WHERE type='index';
SQL
```

Plus your WhatsApp version, phone model and Android version, which you can read in the
app under Settings, Help.

**2. The structure, with no data.** `.schema` prints `CREATE TABLE` statements only; it
does not print rows.

```sh
sqlite3 "file:msgstore.db?mode=ro&immutable=1" .schema > schema.sql
```

Read `schema.sql` before sending it. It should contain nothing but `CREATE TABLE`,
`CREATE INDEX`, `CREATE VIEW` and `CREATE VIRTUAL TABLE` lines.

**3. Row counts per table.** Counts, not rows.

```sh
sqlite3 "file:msgstore.db?mode=ro&immutable=1" <<'SQL' > counts.txt
.mode list
.headers off
SELECT name, (SELECT count(*) FROM pragma_table_info(name)) AS columns
FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name;
SQL
```

**4. The code distributions.** These are the numbers this document is built on, and they
are the ones that change.

```sh
sqlite3 "file:msgstore.db?mode=ro&immutable=1" <<'SQL' > codes.txt
.mode list
SELECT 'message_type', message_type, count(*) FROM message GROUP BY message_type ORDER BY 2;
SELECT 'action_type', action_type, count(*) FROM message_system GROUP BY action_type ORDER BY 2;
SELECT 'origin', origin, count(*) FROM message GROUP BY origin ORDER BY 2;
SELECT 'add_on_type', message_add_on_type, count(*) FROM message_add_on GROUP BY 2 ORDER BY 2;
SELECT 'jid_server', server, count(*) FROM jid GROUP BY server ORDER BY 2;
SQL
```

**5. What Amberkeep made of it.** `inspect` writes counts and codes, never content:

```sh
amberkeep inspect --db msgstore.db --full
```

Its "Not recognised" section lists the type codes your archive has and this build does
not know, with counts. That list is the report.

### What we do with it

A new type code becomes a row in `kinds.go` with its provenance, or stays unrecognised if
no source names it. A new action code becomes a phrasing in `phrasing.go`, or joins the
list in this document of codes nobody can name. A renamed column becomes another
candidate in a `pick(...)` call, which costs nothing and keeps both versions working. A
new table becomes a loader and an entry in the index list.

Nothing in that process requires a single message, which is the point.
