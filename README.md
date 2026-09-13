# Amberkeep

Read, search and export your own WhatsApp history from backups you already have, on
your own computer. Later, move that history between an Android phone and an iPhone.

Amber preserves an insect perfectly for fifty million years. That is the idea.

> Amberkeep is not affiliated with, endorsed by, or connected to WhatsApp LLC or Meta
> Platforms, Inc. It reads backup files you already possess, using keys you already
> have, and never contacts WhatsApp's services.

## Status

Early. The decryption engine works and is verified against the reference
implementation on real backups. Everything else is being built.

| Component | State |
|---|---|
| `crypt15` decryption | working, golden-tested against `wa-crypt-tools` |
| Android message reader | next |
| iPhone reader and backup access | planned |
| Export (text, JSON, HTML) | planned |
| Desktop application | planned |
| Android to iPhone migration | planned |

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
