# Reporting a security problem

**Do not open a public issue.** Use GitHub's private reporting:

**https://github.com/jferrl/amberkeep/security/advisories/new**

It reaches the maintainer and nobody else. You will get an acknowledgement within a
week; if you do not, assume it went astray and say so in a public issue *without any
detail* — "I sent an advisory on the 3rd" is enough to prompt a look.

Please do not include anybody's real backup, database or key in a report, including
your own. If a reproduction genuinely needs one, say so and a private channel will be
arranged.

## What counts

This program's whole claim is that somebody's entire message history stays on their own
machine. Anything that breaks that claim is a security problem, even when it is not a
memory-safety bug:

- A decryption key reaching a log, a file, a process argument, an error message, or
  any HTTP response.
- Anything at all leaving the machine. There is no telemetry, no update check and no
  CDN; a single outbound request is a vulnerability, not a feature.
- The local viewer being reachable by another program on the same computer, or by
  another machine. It binds to loopback and every request carries a secret made fresh
  at each launch.
- Message content escaping into markup rather than being rendered as text.
- A path from a database or a backup reading or writing a file outside the directory
  it is confined to. Both are untrusted input: they are files somebody was handed.
- Anything that modifies an original — a source database, a `.crypt15` file, a backup
  folder — including leaving a `-wal` or `-shm` beside one.

## What does not

- That the tool can decrypt a backup given its key. That is the entire purpose, and
  the key is the user's own.
- That an archive on disk is readable by the person who owns it.
- Findings from a scanner with no path to exploitation. Please include one.

## Supported

The most recent release, and `main`. There is no long-term-support branch and there is
not going to be one while this is one person's work.
