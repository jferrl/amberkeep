# How WhatsApp stores messages

Reference documents for the two database formats this project reads. They exist so
that somebody could build their own reader without looking at our code, and so that a
contributor can repair a schema change after reading one once.

| Document | What it covers |
|---|---|
| [android.md](android.md) | the crypt15 backup, `msgstore.db`, and everything in it |
| [ios.md](ios.md) | iPhone backups, `ChatStorage.sqlite`, and everything in it |

Both follow the same rule, which is the reason they are worth reading: **every claim
is marked CONFIRMED, BEST GUESS or UNKNOWN**, and the evidence for each is stated.
WhatsApp publishes nothing about either format, so most write-ups of them are folklore
passed between tools. Where a published table disagrees with what a real archive
shows, these documents follow the archive and record the disagreement.

They also say what cannot be recovered. That matters as much as what can: somebody
deciding whether to trust this program with their entire message history should learn
its limits here rather than by being disappointed later.

No message content, name, phone number or other personal detail appears in either
document, or ever will. Every number in them was measured against real archives that
stay on their owners' machines.

If you have a WhatsApp version we have not seen, the last section of each document
says exactly which structure-only dumps to send, and repeats the rule that message
content never leaves your machine.
