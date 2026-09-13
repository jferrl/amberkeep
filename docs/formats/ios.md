# WhatsApp on an iPhone

How WhatsApp stores messages on an iPhone, how those messages get out of a backup,
and how this project reads them. It is written so that somebody could build their own
reader from it without looking at our code, and so that a contributor can repair a
schema change after reading it once. Everything here was checked against the
implementation in `internal/source/ios/` and `internal/backupfs/`, and every number
was measured against real stores from two phones. Where the evidence runs out the
document says so rather than guessing.

Two things in here are not written down anywhere else that we could find. Replies are
not where every public tool looks for them, and the column an iPhone conversation
should be ordered by is not the obvious one. Both are in sections 6 and 9.

## Contents

1. [How to read this document](#1-how-to-read-this-document)
2. [Getting the file off the phone](#2-getting-the-file-off-the-phone)
3. [Inside a backup](#3-inside-a-backup)
4. [The store is a Core Data database](#4-the-store-is-a-core-data-database)
5. [Conversations](#5-conversations)
6. [Messages](#6-messages)
7. [Message type codes](#7-message-type-codes)
8. [Everything beyond words](#8-everything-beyond-words)
9. [Replies, which are not where you would look](#9-replies-which-are-not-where-you-would-look)
10. [Identity and names](#10-identity-and-names)
11. [Reading it quickly](#11-reading-it-quickly)
12. [What is not recoverable](#12-what-is-not-recoverable)
13. [Android and iPhone compared](#13-android-and-iphone-compared)
14. [Reporting a schema change](#14-reporting-a-schema-change)

## 1. How to read this document

Every claim carries one of three labels.

**CONFIRMED** means the data proves it, and the proof is stated. Usually that is a
correlation measured across a whole table rather than a sample: a type code is
settled by the media type recorded beside it in every row that has one, not by a
table somebody published.

**BEST GUESS** means the data is consistent with it, no other explanation fits, and
it has not been proved. What would settle it is stated.

**UNKNOWN** means we could not tell. This is a perfectly good answer. A wrong mapping
puts sentences into somebody's archive that nobody ever said, which is worse than a
gap that announces itself.

### The stores the numbers come from

Three real stores, from two phones, none of which appears in this repository and none
of whose contents appears in this document.

| Store | Conversations | Messages | Notes |
|---|---|---|---|
| Phone A | 554 | 161,026 | iPhone 16,2, iOS 26.6.2, WhatsApp 2.26.33 |
| Phone B | 212 | 1,377 | iPhone 18,3, iOS 26.2 |
| Phone B, after a migration | 877 | 1,095,229 | the same phone after an Android history was merged in |

The third is useful precisely because it was written by this project rather than by
WhatsApp, so a claim that holds on the first two and not the third is a claim about
the app rather than about the format.

## 2. Getting the file off the phone

### The file you want

`ChatStorage.sqlite`, in WhatsApp's shared app group container. It does not sit on a
filesystem you can browse. It has to come out of a backup.

Two files sit beside it and both matter:

| File | What it is |
|---|---|
| `ChatStorage.sqlite` | the database |
| `ChatStorage.sqlite-wal` | everything written since the last checkpoint |
| `ChatStorage.sqlite-shm` | shared-memory index for the above |

**CONFIRMED.** A backup copies whatever files exist at the moment it runs, without
asking WhatsApp to close its database first. A store extracted without its `-wal` is
therefore missing whatever had not yet been folded in. On one real backup the main
file was 158,924,800 bytes and the extracted, checkpointed result was 118,296,576:
different, because checkpointing rewrites the file, not because anything was lost.

### The backup must not be encrypted

**CONFIRMED.** An encrypted backup stores every file's bytes under a per-file key
wrapped in a keybag that only the phone can open. Apple does not publish the format.
Nothing on a computer can read one.

The fix is in Finder, iTunes or the Apple Devices app: uncheck "Encrypt local
backup", which asks for the password that was set, then take a fresh backup. Chats
are unaffected. Saved passwords and Health data are left out of an unencrypted
backup, so it is worth turning back on afterwards.

One warning that belongs here because it costs people their history: **Finder keeps
one backup per device and overwrites it.** Before making an unencrypted backup,
right-click the existing one in Manage Backups and choose Archive, and it will
survive.

This project detects an encrypted backup from `Manifest.plist` and refuses it with
that explanation. It does not attempt decryption, deliberately.

### Where backups live

| Platform | Location |
|---|---|
| macOS | `~/Library/Application Support/MobileSync/Backup/` |
| Windows, Apple Devices app or Microsoft Store iTunes | `%USERPROFILE%\Apple\MobileSync\Backup\` |
| Windows, classic iTunes | `%APPDATA%\Apple Computer\MobileSync\Backup\` |

Each backup is a folder named after the device's UDID, a forty-character hexadecimal
string.

On macOS that directory is behind Full Disk Access. A program without it gets a
permission error, not an empty list, and the two are worth distinguishing in whatever
you build: telling somebody there are no backups when there are is a bad failure.

## 3. Inside a backup

A backup folder contains four files that identify it, and then the data.

| File | What it holds |
|---|---|
| `Manifest.db` | a SQLite index of every file in the backup |
| `Manifest.plist` | whether the backup is encrypted, and the keybag if it is |
| `Info.plist` | device name, model, iOS version, last backup date |
| `Status.plist` | whether the backup finished |

**CONFIRMED.** All four present is what makes a folder a backup. Treat a folder
missing any of them as something else.

### How a file is found

`Manifest.db` has one table that matters:

```sql
CREATE TABLE Files (
    fileID TEXT PRIMARY KEY,
    domain TEXT,
    relativePath TEXT,
    flags INTEGER,
    file BLOB
);
```

`flags` is 1 for a file and 2 for a directory.

**CONFIRMED.** `fileID` is the lowercase hexadecimal SHA-1 of `domain + "-" +
relativePath`. The bytes live at `<backup>/<first two characters of fileID>/<fileID>`.
Verified against two independently published forensics vectors as well as against
real backups.

WhatsApp's store is therefore:

```
domain       AppDomainGroup-group.net.whatsapp.WhatsApp.shared
relativePath ChatStorage.sqlite
```

On one real backup that domain held 38,486 files; on another, 743.

### The `file` column

An NSKeyedArchiver binary property list describing one file: size, mode, owner,
timestamps, and for an encrypted backup a wrapped key. It is the flat,
UID-referenced shape Apple's archiver always produces, with `$archiver`, `$version`,
`$top` and `$objects`. The only field a reader needs is `Size`.

### A trap worth naming

**CONFIRMED, and it cost us a bug.** Apple writes `Manifest.db` in write-ahead-log
mode. SQLite creates a `-wal` and a `-shm` beside any database it opens in that mode,
**read-only or not**. Simply reading the index therefore puts two new files inside
somebody's backup.

Opening it with `immutable=1` skips the write-ahead machinery and creates nothing.
That is only correct when there is no log to replay, because `immutable` ignores one,
so check first and fall back to an ordinary open when a non-empty `-wal` is present.
A backup Apple has finished writing has none.

No synthetic fixture finds this, because a fixture is in the default journal mode.
Only a real backup does.

## 4. The store is a Core Data database

`ChatStorage.sqlite` is a SQLite database written by Apple's Core Data, which imposes
a shape of its own on top of WhatsApp's.

- Every entity becomes a table named `Z` plus the entity name in capitals.
- Every attribute becomes a column named `Z` plus the attribute name in capitals.
- Every row has `Z_PK` (its primary key), `Z_ENT` (which entity it is) and `Z_OPT`
  (an optimistic-locking counter).
- `Z_PRIMARYKEY` holds the next primary key for each entity.
- `Z_METADATA` holds the store's UUID and model hash.

The tables this project reads:

| Table | Rows on phone A | What it holds |
|---|---|---|
| `ZWAMESSAGE` | 161,026 | one row per message |
| `ZWACHATSESSION` | 554 | one row per conversation |
| `ZWAMEDIAITEM` | 111,618 | what a message carried besides words |
| `ZWAGROUPMEMBER` | 2,284 | who is in which group, and their name |
| `ZWAGROUPINFO` | — | a group's creation date and creator |
| `ZWAPROFILEPUSHNAME` | 931 | the name somebody chose for themselves |
| `ZWAMESSAGEDATAITEM` | 1,908 | link previews |
| `ZWAMESSAGEINFO` | 154,270 | delivery receipts |

**Do not assume `Z_ENT` values.** Core Data renumbers entities whenever the model
changes, so an entity number from one phone means nothing on another. Read
`Z_PRIMARYKEY` if you need them; a reader does not.

### Timestamps

**CONFIRMED.** Every `TIMESTAMP` column counts seconds, possibly fractional, from
2001-01-01 00:00:00 UTC, which is Core Data's epoch rather than Unix's. On phone A
the message dates run from 386,538,218 to 810,896,036, which is April 2013 to
September 2026.

A zero is not a date at the start of 2001. It is a column nobody filled in, and
rendering it as a real date puts messages in 2001 that were sent last week.

## 5. Conversations

`ZWACHATSESSION`, one row per conversation. The columns that matter:

| Column | Meaning |
|---|---|
| `ZCONTACTJID` | the address: a person, a group, a channel, a status feed |
| `ZPARTNERNAME` | what to call it |
| `ZSESSIONTYPE` | the store's own classification |
| `ZLASTMESSAGEDATE` | when it was last used |
| `ZARCHIVED` | whether it is archived |
| `ZMESSAGECOUNTER` | how many messages it holds, allegedly |
| `ZGROUPINFO` | a row in `ZWAGROUPINFO`, for a group |

### Classify by the address, not by `ZSESSIONTYPE`

**CONFIRMED** on phone A, where the correlation is exact:

| `ZSESSIONTYPE` | Address ends in | Count |
|---|---|---|
| 0 | `@s.whatsapp.net` or `@lid` | 304 |
| 1 | `@g.us` | 30 |
| 2 | `@broadcast` | 1 |
| 3 | `@status` or `@lid.status` | 219 |

Types 4 and 5 appear on phone B, one row each, and are **UNKNOWN**.

Since the address determines the kind on every row we have seen, and an address is
self-describing where a code is not, this project classifies by the address. That
also means an iPhone reader and an Android reader agree by construction.

### The status feeds are a surprise

**CONFIRMED.** Android keeps a single status feed at `status@broadcast`. An iPhone
keeps **one pseudo-conversation per person whose status has been seen**, addressed
`<number>@status` or `<id>@lid.status`. Phone A had 219 of them, which is 40% of its
conversation list.

They are not conversations and an archive should leave them out, exactly as it leaves
out Android's single feed. If you classify by `ZSESSIONTYPE` alone, or by looking for
`status@broadcast`, you will export several hundred empty pseudo-chats.

### `ZMESSAGECOUNTER` is wrong

**CONFIRMED.** On phone A it disagreed with the true count for **551 of 554**
conversations, always by one. Count the messages yourself; one grouped scan does the
whole store.

## 6. Messages

`ZWAMESSAGE`. The columns a reader uses:

| Column | Meaning |
|---|---|
| `Z_PK` | the message's row |
| `ZCHATSESSION` | which conversation, by row |
| `ZISFROMME` | 1 if the phone's owner sent it |
| `ZMESSAGEDATE` | when, in Core Data seconds |
| `ZSORT` | the store's own display order — see section 11 |
| `ZMESSAGETYPE` | what kind of message — see section 7 |
| `ZTEXT` | the words, or a caption |
| `ZSTANZAID` | WhatsApp's own identifier, stable across devices |
| `ZFROMJID` | who sent it, sometimes |
| `ZTOJID` | who it went to, sometimes |
| `ZGROUPMEMBER` | who sent it in a group, by row into `ZWAGROUPMEMBER` |
| `ZPUSHNAME` | the name the sender had set at the time |
| `ZSTARRED` | whether it is starred |
| `ZMEDIAITEM` | a row in `ZWAMEDIAITEM` |
| `ZGROUPEVENTTYPE` | meaningful only for types 6 and 10 — see below |
| `ZPARENTMESSAGE` | always empty — see section 9 |

### Who sent it

**CONFIRMED.** In a group, the sender is named by `ZGROUPMEMBER`, a row identifier
into `ZWAGROUPMEMBER`, and by nothing else useful: `ZFROMJID` holds the group's own
address. On phone A, 50,571 of 50,686 incoming group messages have a member row, so
99.8% resolve and the rest are unrecoverable.

In a one-to-one conversation the sender columns are often empty, because there is
only one person it could be. Fall back to the conversation's own address.

### `ZGROUPEVENTTYPE` is not what its name suggests

**CONFIRMED.** The column is populated on nearly every message, not only on group
events. For plain text messages on phone A:

| `ZISFROMME` | `ZGROUPEVENTTYPE` | Count |
|---|---|---|
| 0 | 2 | 82,473 |
| 0 | 0 | 7 |
| 1 | 0 | 56,763 |
| 1 | 2 | 2,929 |

That is a direction marker, not an event type. **Read it only when `ZMESSAGETYPE` is
6 or 10.** For those, it is the code for what happened.

### Notice codes are UNKNOWN

On phone A, type 6 carries `ZGROUPEVENTTYPE` values 2, 7, 50, 12, 4, 1, 3, 21, 20, 42
and others; type 10 carries 3, 2, 58, 56, 36, 38, 1, 91, 47, 6. No reliable public
source names them and we have not established them from the data.

This project therefore keeps the code and uses whatever `ZTEXT` the store itself
wrote, rather than inventing a sentence. The Android reader has a verified table for
its own, quite different, action codes; nothing transfers.

## 7. Message type codes

`ZMESSAGETYPE`. The mapping below was established by joining each code to the media
type recorded beside it in `ZWAMEDIAITEM.ZVCARDSTRING`, across all 161,026 messages
on phone A. That column is a mime type for media, a vCard for a contact card,
semicolon-separated coordinates for a location, and an address for a deletion.

| Code | Meaning | Count | Evidence | Confidence |
|---|---|---|---|---|
| 0 | text | 142,172 | no media type on any row | CONFIRMED |
| 1 | image | 6,658 | `image/jpeg` on all 6,658 | CONFIRMED |
| 2 | video | 919 | `video/mp4` on 918, one absent | CONFIRMED |
| 3 | voice note | 1,794 | `audio/ogg; codecs=opus` on 1,791 | CONFIRMED |
| 4 | contact card | 86 | a vCard on every row | CONFIRMED |
| 5 | location | 159 | coordinates, or none | CONFIRMED |
| 6 | group notice | 336 | no media, `ZGROUPEVENTTYPE` set | CONFIRMED |
| 7 | text with a link preview | 1,582 | all have text, 1,110 a URL, 1,581 a preview title | CONFIRMED |
| 8 | document | 223 | PDF, Word, Excel, PowerPoint | CONFIRMED |
| 10 | account or security notice | 1,345 | no media, `ZGROUPEVENTTYPE` set | CONFIRMED |
| 11 | video | 225 | `video/mp4` on all 225 | CONFIRMED as video |
| 14 | deleted for everyone | 184 | an address where a mime type goes | CONFIRMED |
| 15 | sticker | 4,309 | `image/webp` on all 4,309 | CONFIRMED |
| 38 | image | 485 | `image/jpeg` on all 485 | CONFIRMED as an image |
| 39 | video | 55 | `video/mp4` on all 55 | CONFIRMED as a video |
| 20 | image | 4 | `image/jpeg` | CONFIRMED as an image |
| 24 | document | 4 | `application/pdf` | CONFIRMED as a document |
| 53 | voice note | 2 | `audio/ogg; codecs=opus` | CONFIRMED as audio |
| 54 | video | 7 | `video/mp4` | CONFIRMED as a video |
| 43 | video | 1 | `video/mp4` | CONFIRMED as a video |
| 12, 19, 25, 27, 28, 30, 32, 34, 41, 46, 55, 59, 60, 63, 66, 73, 75, 76, 91 | — | 474 in total | media rows with no mime type | UNKNOWN |

Note the difference between the last two kinds of confirmation. For 1, 2, 3, 8 and 15
the evidence settles what the code means. For 11, 38, 39, 20, 24, 53, 54 and 43 it
settles only what sort of file the message carried: several of these are plainly
variants of "a video" whose distinction from code 2 we have not established, and a
reader that shows them as videos is right about the thing that matters and silent
about the thing it does not know.

The unknown codes are 474 messages of 161,013, which is 0.29%.

### Deciding by the media type when the code is unknown

**This is what keeps a new WhatsApp release from turning photographs into blanks.**
When the numeric code is one this build does not know, the media type recorded beside
the message decides instead. The number may be new; `image/jpeg` has not changed. A
message of an unknown code with no media beside it is reported with its number.

### Codes collide between platforms

**CONFIRMED.** Android and iPhone numbering are unrelated. Codes 46 and 66 exist on
both and mean different things. Never carry a table from one to the other.

## 8. Everything beyond words

`ZWAMEDIAITEM`, at most one row per message, joined by `ZWAMESSAGE.ZMEDIAITEM`. It is
misleadingly named: it holds far more than media.

| Column | What it holds |
|---|---|
| `ZVCARDSTRING` | a mime type, a vCard, coordinates, or an address, by message type |
| `ZVCARDNAME` | whose card was shared |
| `ZTITLE` | a document's name, a place's name, a link preview's title |
| `ZFILESIZE` | the file's size |
| `ZMOVIEDURATION` | how long a video or voice note runs |
| `ZLATITUDE`, `ZLONGITUDE` | where a shared place is |
| `ZMEDIALOCALPATH` | where the file sat on the phone |
| `ZXMPPTHUMBPATH` | where the thumbnail sat on the phone |
| `ZMETADATA` | a protobuf — see section 9 |

One column reuse is worth stating plainly: for a deleted message, `ZVCARDSTRING`
holds the address of whoever removed it. That is how an archive can say a message was
deleted by an administrator rather than by its sender.

### Link previews are in their own table

`ZWAMESSAGEDATAITEM`, joined by `ZMESSAGE`. 1,908 rows on phone A, 1,579 with a
title. `ZTITLE` is the page's title, `ZSUMMARY` its description, `ZMATCHEDTEXT` the
address. A message can carry more than one, so join it separately rather than widening
the message query.

### Thumbnails are files, not data

**CONFIRMED, and it is the biggest difference from Android.** An iPhone store keeps
no picture inside itself. `ZXMPPTHUMBPATH` is a path, populated on 9,941 rows, and
the image is a file in the backup. `ZMETADATA` averages 203 bytes, far too small to
hold a picture.

An Android archive embeds 12,510 recoverable previews of pictures whose original
files are long gone. An iPhone store embeds none. Recovering iPhone thumbnails means
extracting the files those paths name from the backup, which this project does not
do yet.

## 9. Replies, which are not where you would look

**CONFIRMED, and this is the finding this document exists for.**

The obvious place for a reply to point at what it answers is `ZWAMESSAGE.ZPARENTMESSAGE`,
which is what the public tools read. On phone A that column is `NULL` in **all
161,026 rows**. On the merged store it is `NULL` in all 1,095,229. There are no
replies there at all.

They are in `ZWAMEDIAITEM.ZMETADATA`, a protobuf attached to the replying message.
93,396 text messages on phone A have one.

Walking the wire format across the whole store and tabulating which fields appear
gives:

| Field | Shape | Meaning |
|---|---|---|
| 5 | 16 to 40 hexadecimal characters | the answered message's `ZSTANZAID` |
| 6 | an address | who sent the answered message, in a group |
| 19 | a nested message whose field 1 is text | a copy of the answered message |

The proof that field 5 is what it looks like: of 20,000 text messages with metadata,
5,691 carried a value of that shape, and **5,682 of them, 99%, name a message that is
really in the database**. A field that resolves against 161,026 known identifiers at
that rate is not a coincidence.

Field 6 is present on 2,888 of 4,000 sampled replies. It is absent in one-to-one
conversations, where there is only one person the answered message could be from.

Field 19 carries the answered message's own words, which means **a reply survives
even when the message it answers does not**.

Fields 4, 13, 32, 35, 46, 50, 68 and 105 also appear. They are **UNKNOWN** and this
project ignores them.

On phone A this recovers **14,880 replies** that would otherwise have been plain
messages.

### Read it defensively

This is undocumented, it is Apple's own wrapper rather than WhatsApp's wire
`ContextInfo` (whose field numbers are 1, 2 and 3 for the same three things), and it
will change. Accept a value only if it has the shape it should have: an identifier
that is not hexadecimal is not an identifier, and an address with no `@` is not an
address. A field that means something else in a future release is then ignored rather
than believed.

`ZWAMESSAGEINFO.ZRECEIPTINFO`, 154,270 rows, is a different protobuf holding delivery
and read receipts. It is not where replies are.

## 10. Identity and names

**An iPhone names almost everybody already**, which is the largest practical
difference from Android.

| Source | Rows on phone A | What it is |
|---|---|---|
| `ZWACHATSESSION.ZPARTNERNAME` | 554 of 554 | the name from the phone's address book |
| `ZWAGROUPMEMBER.ZCONTACTNAME` | 2,284 of 2,284 | the same, for a group member |
| `ZWAGROUPMEMBER.ZFIRSTNAME` | 198 | a first name, where nothing better was saved |
| `ZWAPROFILEPUSHNAME.ZPUSHNAME` | 931 | the name somebody chose for themselves |

WhatsApp on iOS copies the phone's contacts into the store, so **all 304 direct
conversations on phone A carry a real name**, and not one of them is merely the
number written out. The same person's Android archive needed a separate address-book
export and still reached only about half.

Resolution order, most trusted first: the saved name, then a group member's first
name, then the name somebody chose for themselves, then the address.

Hidden `@lid` identifiers appear here as they do on Android, and the address book
resolves them without any mapping table being needed: on phone A, 43 of 304 direct
conversations are addressed by `@lid` and all 43 carry a name.

## 11. Reading it quickly

### Do not order a conversation by date

**CONFIRMED, and it is worth 77 times.**

A conversation is read by filtering on `ZCHATSESSION` and ordering by something.
Ordering by `ZMESSAGEDATE` is the obvious choice and the wrong one: there is no index
over a conversation and a date, so SQLite says

```
SEARCH m USING INDEX ZWAMESSAGE_ZCHATSESSION_INDEX (ZCHATSESSION=?)
USE TEMP B-TREE FOR ORDER BY
```

and sorts the entire conversation into a temporary table for every page, throwing
away all but the fifty rows wanted.

`ZSORT` is the store's own display order and `Z_WAMessage_compoundIndex` covers
`(ZCHATSESSION, ZSORT)`, so the same query becomes

```
SEARCH m USING INDEX Z_WAMessage_compoundIndex (ZCHATSESSION=?)
```

with no sort at all.

It is safe to use, measured on a conversation of 89,894 messages:

- `ZSORT` is never null, in any row of any store we have.
- It is unique within a conversation: 89,894 distinct values for 89,894 rows.
- It never disagrees with the date: **zero inversions** across the whole conversation,
  and none anywhere in a store of 1,095,229 messages.

Measured effect:

| Operation | By date | By `ZSORT` |
|---|---|---|
| Twenty pages of 500, backwards | 0.40 s | under 0.01 s |
| Reading a whole conversation of 89,894 backwards | 14.9 s | 0.19 s |
| Reading 1,095,206 messages straight through | 7.5 s | 2.5 s |

Keep the date as a fallback so that a store without `ZSORT` is slow rather than
unreadable.

### Page on a key, never on an offset

`LIMIT ... OFFSET n` makes SQLite walk and discard `n` rows, so the last page of a
long conversation costs the most. Carry the position instead, and the cost of a page
is the page.

### An iPhone store needs no index rebuilding

Unlike an Android backup, which arrives with no indexes at all, a store extracted
from an iPhone backup keeps every index WhatsApp created. Twenty of them on
`ZWAMESSAGE` alone. There is nothing to put back.

### What it costs, measured

| Operation | Phone A, 161,026 messages |
|---|---|
| Open and read the conversation list | under 0.1 s |
| Read every message | 0.9 s |
| Export every conversation as web pages | 3 s |
| Page backwards through 32,935 messages | 0.19 s |

## 12. What is not recoverable

**Reactions.** CONFIRMED. There is no reaction table in the store and no column for
one. Nothing named for them exists. An Android archive records who reacted to what
with which emoji; an iPhone store does not, and this is a limitation to publish
rather than a gap to paper over.

**Embedded picture previews.** The store holds paths, not images. See section 8.

**Media files.** Photographs, videos, voice notes and documents are files in the
backup, not rows in the database. Their names, sizes and durations survive here; the
files themselves must be extracted separately.

**Voice-note transcriptions.** Not stored locally, on either platform.

**Earlier versions of an edited message.** Only the final text is kept.

**The meaning of a notice code.** See section 6.

**Anything from an encrypted backup.** See section 2.

## 13. Android and iPhone compared

For anybody reading both documents.

| | Android | iPhone |
|---|---|---|
| File | `msgstore.db`, from a crypt15 backup | `ChatStorage.sqlite`, from an iPhone backup |
| Obtained by | decrypting with a 64-digit key | extracting from a backup, unencrypted |
| Schema | WhatsApp's own tables | Core Data, `Z`-prefixed |
| Epoch | milliseconds since 1970 | seconds since 2001 |
| Indexes in the file | **none at all**, must be rebuilt | all of them, twenty on messages alone |
| Order a conversation by | `timestamp` | `ZSORT`, never the date |
| Names | needs an address-book export; about half resolve | already inside; essentially all resolve |
| Replies | `message_quoted`, a proper table | a protobuf beside the message |
| Reactions | recoverable | not stored |
| Embedded previews | 12,510 pictures on one archive | none; paths only |
| Notice codes | a verified table for most | UNKNOWN |
| Status feed | one, at `status@broadcast` | one per person, hundreds of them |

## 14. Reporting a schema change

WhatsApp changes these databases every few months. The most useful contribution
anybody can make is a structure-only dump from a version we have not seen.

**Never send message content.** Everything below is schema and counts only, and you
should read the output before sending it.

```sh
# The shape of every table.
sqlite3 ChatStorage.sqlite ".schema" > ios-schema.sql

# How many rows each table holds.
sqlite3 ChatStorage.sqlite \
  "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name" \
  | while read t; do
      printf '%s %s\n' "$t" "$(sqlite3 ChatStorage.sqlite "SELECT count(*) FROM \"$t\"")"
    done > ios-counts.txt

# Which message types appear, and how often.
sqlite3 ChatStorage.sqlite \
  "SELECT ZMESSAGETYPE, count(*) FROM ZWAMESSAGE GROUP BY 1 ORDER BY 2 DESC" > ios-types.txt

# Which notice codes appear, for types 6 and 10 only.
sqlite3 ChatStorage.sqlite \
  "SELECT ZMESSAGETYPE, ZGROUPEVENTTYPE, count(*) FROM ZWAMESSAGE
   WHERE ZMESSAGETYPE IN (6,10) GROUP BY 1,2 ORDER BY 3 DESC" > ios-notices.txt

# Which media types accompany which message types. This is what settles a new code.
sqlite3 ChatStorage.sqlite \
  "SELECT m.ZMESSAGETYPE, mi.ZVCARDSTRING, count(*)
   FROM ZWAMESSAGE m JOIN ZWAMEDIAITEM mi ON mi.Z_PK = m.ZMEDIAITEM
   WHERE mi.ZVCARDSTRING LIKE '%/%' AND mi.ZVCARDSTRING NOT LIKE '%BEGIN:VCARD%'
   GROUP BY 1,2 ORDER BY 1" > ios-media-types.txt
```

The last one is the valuable file. It is how every CONFIRMED row in section 7 was
established, and it contains no message content: only type numbers and mime types.

Also useful, and safe: the WhatsApp version from the App Store, the iOS version, and
the model. Not the phone number, not the device serial, not a contact name.
