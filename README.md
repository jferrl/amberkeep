# Amberkeep

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
| Export: web pages, text, JSON | working |
| Full-text search | working, accent-insensitive |
| Local viewer (`serve`) | working |
| Command-line tool | working |
| iPhone reader and backup access | next |
| Desktop application | planned |
| Android to iPhone migration | planned |

Measured on one real archive of 4,286 conversations and 1,121,482 messages, on a laptop:

| Step | Time |
|---|---|
| decrypt 236 MB to 466 MB | 5 s |
| export every conversation as web pages | 5 min |
| build the search index | 5 min |
| a search across the whole archive | under 10 ms |
| open a 92,180-message conversation in the viewer | 0.12 s |

## Using it

```sh
amberkeep decrypt --key key.txt --in msgstore.db.crypt15 --out msgstore.db
amberkeep inspect --db msgstore.db --full
amberkeep serve   --db msgstore.db --contacts contacts.vcf --country 34
amberkeep export  --db msgstore.db --contacts contacts.vcf --country 34 --out archive/
amberkeep search  --db msgstore.db "whatever you remember"
```

`serve` opens the archive in a browser. It listens on the loopback address only, and
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

Go 1.24 or newer. No cgo, no external toolchain.

```sh
go build ./...
go test ./... -race
golangci-lint run ./...
```

The golden test runs against a real backup when you point it at one. Real backups and
real keys must never be committed:

```sh
AMBERKEEP_GOLDEN_CRYPT15=/path/to/msgstore.db.crypt15 \
AMBERKEEP_GOLDEN_KEY=/path/to/key.txt \
AMBERKEEP_GOLDEN_EXPECTED=/path/to/msgstore.db \
  go test ./internal/crypt15/ -run TestDecryptGolden
```

## Contributing

Read [docs/PRINCIPLES.md](docs/PRINCIPLES.md) first; it is binding, not advisory.
Commits follow the Conventional Commits format and need a DCO sign-off (`git commit -s`).

Schema reports are the most useful contribution. WhatsApp changes its databases every
few months, and a **structure-only** dump from a version we have not seen keeps the
readers working for everyone. Never send message content.

## Licence

AGPL-3.0. See [LICENSE](LICENSE).

The engine is free software so that anyone can verify what it does with their private
messages. The desktop application built on top of it is a separate commercial product.

## Prior art

Standing on the shoulders of [wa-crypt-tools](https://github.com/ElDavoo/wa-crypt-tools),
[WhatsApp-Chat-Exporter](https://github.com/KnugiHK/Whatsapp-Chat-Exporter) and
[watoi](https://github.com/residentsummer/watoi), which documented these formats first.
