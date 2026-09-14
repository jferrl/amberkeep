# Signing and distribution

Nothing this project publishes is signed. That is a decision, not an oversight, and
this file is what it would take to change it — written while nobody is under time
pressure, because the day a certificate is bought is a poor day to work out what to
do with it.

The release workflow already has the signing steps in it. They are dormant: with no
secrets configured they do nothing and the release goes out unsigned exactly as it
does today. Adding the secrets is the whole change.

## What is wrong today

Both systems refuse an unsigned application the first time.

On macOS the bundle is ad-hoc signed — no certificate, no identity — which is the
minimum that makes it internally consistent. Without it macOS will not even assess
the bundle:

    code has no resources but signature indicates they must be present

With it, the verdict is a plain `rejected`, which is the correct answer for an
application whose publisher cannot be identified. A copy carrying the quarantine flag
a browser attaches stays refused until somebody clears it by hand:

```sh
xattr -dr com.apple.quarantine /Applications/Amberkeep.app
```

On Windows, SmartScreen shows "Windows protected your PC" and needs **More info** →
**Run anyway**.

Somebody who meets either of these concludes the download is broken. It is not: it is
unsigned, which is a different thing, and saying so is on us. The README does.

## macOS — the one worth doing

**Apple Developer Program, 99 USD a year.** An individual enrolment is enough; the
certificate is issued in your own name. Notarisation costs nothing on top.

1. Create a **Developer ID Application** certificate — the one for distribution
   outside the App Store, not the Mac App Store one.
2. Export it as a `.p12` with a password.
3. Create an **App Store Connect API key** for notarisation. Preferred over an Apple
   ID and an app-specific password: it is revocable on its own and is not tied to
   somebody's personal account.

Then five repository secrets:

| Secret | What it is |
|---|---|
| `MACOS_CERTIFICATE_P12` | the `.p12`, base64 encoded |
| `MACOS_CERTIFICATE_PASSWORD` | the password it was exported with |
| `MACOS_SIGNING_IDENTITY` | `Developer ID Application: Name (TEAMID)` |
| `ASC_KEY_ID` | the API key's identifier |
| `ASC_ISSUER_ID` | the issuer the key belongs to |
| `ASC_KEY_P8` | the key file's contents |

```sh
base64 -i certificate.p12 | pbcopy
security find-identity -v -p codesigning   # to read the identity string exactly
```

The workflow signs with `--options runtime`, because notarisation refuses a bundle
without the hardened runtime, and staples the result so a machine that is offline the
first time it opens the disk image still sees a valid ticket.

This removes the warning completely and permanently.

## Windows — messier, and worth less

Two routes, neither free:

- **Azure Trusted Signing**, about 10 USD a month, no hardware token, built for CI.
  Eligibility has historically wanted verifiable business history and Microsoft has
  been widening it; check at signup rather than assuming either way.
- **An OV certificate** from Sectigo, DigiCert or SSL.com, roughly 200–400 USD a
  year. Since 2023 the private key has to live on a FIPS token or an HSM, so signing
  from CI needs the issuer's cloud service (DigiCert KeyLocker, SSL.com eSigner)
  rather than a token in a drawer.

**EV certificates are not worth the premium.** They stopped buying instant SmartScreen
reputation in 2024.

And the thing to be clear-eyed about: even with a valid certificate, SmartScreen
reputation is earned over download volume. Early downloads may still warn. macOS is
the opposite — 99 USD makes the warning go away outright.

So: macOS first, Windows when there is a reason.

## What signing does not fix

The compatibility of what the program produces. No backup Amberkeep has made has ever
been restored to a phone, and a signature would not change that by a byte. See the
README.
