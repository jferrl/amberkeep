# 10. Free, with a way to say thanks

Date: 2026-09-15

## Status

Accepted. Supersedes [9. A licence is a signed statement, not a question asked of a
server](0009-offline-licences.md), which is kept because the reasoning in it is still
true — it is simply answering a question this project has stopped asking.

## Context

The plan was to sell this: a free tier for reading, and about 29 euros for writing an
archive out and moving a history onto a phone. ADR 9 designed the licence for it —
offline, signed, no network — and it was built, tested and switched off waiting for a
shop.

Then the price came down to ten euros, and at ten euros the arithmetic stops working.
A sale nets about 7.35 after VAT and the merchant's fee, and the first thing a licence
brings with it is support: keys lost, keys mistyped, keys on the wrong computer, a
reinstall, a refund. Three of those emails cost more than the sale they protect.

And they protect very little. The engine is AGPL-3.0 and published: anybody who wants
the paid parts without paying can build them in half a minute, with no patching and no
cleverness. The gate was always social rather than technical.

Underneath that was a question nobody had answered out loud: what is this program
*for*? It exists because somebody's phone died and the official tools failed them three
times. The person it is written for is having one of the worse weeks of their year.
Asking them for a licence key at the moment they get their own conversations back is
not a business model, it is a toll booth.

## Decision

**It is free. All of it. There is no licence, no key, no tier and nothing switched
off.**

There is one line, shown twice in the life of the program: when an archive has been
written out, and when a migration has produced the backup. Both are moments when
something that mattered has just worked. It says that if this helped, there is
somewhere to buy the author a coffee, that it is voluntary, and that nothing here
depends on it.

- **Never on a failure**, never at startup, never twice in a row, never in the way.
- **Absent entirely until there is an address.** `thanks.Address` is empty and both
  places that would show it show nothing.
- **Suggested ten euros**, on a page that offers five, ten and twenty-five. Most
  people take the suggestion; the largest is there because a few will want it.
- **The address travels with the result** that earned it rather than being fetched,
  because the two screens it belongs on are the two that already say it worked.

## Consequences

**This will not be a living.** Voluntary payment runs at something like half a percent
to two percent of users. At a thousand downloads a month that is somewhere between
fifty and two hundred euros: coffee money and the occasional good week. Turning it into
income would need a price and a gate, and that would be a different decision, taken
deliberately, with this one in front of it.

**A great deal of machinery goes away.** No merchant of record, no VAT registration, no
key format, no signing key to guard for a decade, no reissuing, no refund policy to
honour programmatically, no support category that exists only because of the shop. A
donation given for nothing in return is also outside VAT in most of Europe, where a
licence sold in exchange for something is not — which removes the last piece of
paperwork.

**Nobody is ever locked out of their own history.** That was always the point, and now
it is unconditional rather than a tier boundary that happened to fall in a kind place.

**Signing is still worth paying for, and for a different reason.** An unsigned download
is refused by both operating systems and cannot be told apart from one a stranger has
modified. That protects the person who downloads it rather than the revenue, which is
the better thing to spend money on and the one place where spending it buys something
real.
