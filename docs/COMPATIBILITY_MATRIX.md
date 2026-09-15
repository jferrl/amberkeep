# What Amberkeep has been run against

WhatsApp renames and adds database columns every few months, on both phones, without
telling anybody. Nothing in this program assumes a column exists — the readers ask the
database what it has and adapt — which is what lets one build open a database written
by a version nobody has looked at yet. It is also why this page exists: a database
carrying something new opens perfectly and says nothing about the part that was not
read, so what has actually been seen has to be written down.

Every row below is a real phone somebody used, not a claim about a version range.

## Reading an archive

| Platform | WhatsApp | OS | Status | What that rests on |
|---|---|---|---|---|
| Android | 2.26.35.75 | Android 16 | **verified** | 4,286 conversations and 1,121,482 messages read, exported and searched; 17 of those 1.1 M messages carry a type code this build has no meaning for |
| iPhone | 2.26.33.73 | iOS 26 | **verified** | 554 conversations and 161,026 messages read, including replies and the pictures an iPhone store keeps only the paths to |
| iPhone | 2.26.33.73 | iOS 26 | **verified** | a second device: 212 conversations, 1,377 messages |
| Android | before 2021 | any | **best-effort** | the reader recognises the older `messages` layout and reads it; no real database of that age has been through it |
| Android | crypt14 or a passkey | any | **unsupported** | the key is inside the app's own storage on the phone, or never leaves it. Amberkeep says so and explains what to switch on instead |

"Verified" means a real archive from that version went through this program end to
end and the result was checked. "Best-effort" means the code is there and the path is
untravelled. "Unsupported" means it cannot work, for a reason, and the program says
which.

## Moving a history onto an iPhone

| From | To | Status | What that rests on |
|---|---|---|---|
| Android 2.26.35.75 | iPhone 2.26.33.73 on iOS 26 | **unproven** | 1,093,822 messages planned and written into a copy of a real 4.6 GB backup, all 3,427 consistency checks passing, 2 files of 27,352 changed, the original byte-identical — and no phone has ever accepted the result |

Nothing has been restored to a phone. Until somebody does that on a spare device, the
migration is unproven however many checks pass, and the program says so on the screen
where it matters.

## The shapes this build carries

```
amberkeep canary --shapes
```

Each is one database's table and column names, recorded from a real phone. They are
what "new" is measured against: a table in none of them is a table this program has
never seen.

## Adding yours

If you have a WhatsApp database this build has not been run against — a newer version,
an older phone, another country's build — this is the most useful thing anybody can
send:

```sh
amberkeep canary --db msgstore.db          # what this build makes of it
amberkeep canary --db msgstore.db --emit \
  --whatsapp 2.26.36.1 --os "Android 16" --device SM-A566B > android-2.26.36.1.json
```

The file it writes is table names, column names and the version you typed. No message,
no name, no number, no filename: the report is built from the database's catalogue and
from counting rows, and there is a test that fails if a value ever reaches it. It is
short enough to read before you send it, and reading it before you send it is a
perfectly reasonable thing to do.

Open an issue with it at <https://github.com/jferrl/amberkeep/issues>, or a pull
request adding it to `internal/canary/corpus/`.

## What the canary reports

- **Tables and columns no known shape has.** The early warning. A table that appeared
  is usually a feature that appeared.
- **Tables and columns every known shape has and yours does not.** Usually an older
  version, occasionally a feature that was removed.
- **Message type codes nothing here has a meaning for, with how many rows carry each.**
  A code on one message is a curiosity; a code on forty thousand is a release note.
  On an iPhone, a code with no meaning is not necessarily a message that was not read:
  the media type recorded beside it settles most of them.
