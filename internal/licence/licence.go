// Package licence says what somebody has paid for, on a machine that will never ask
// anybody.
//
// This program makes no network calls. Not for updates, not for telemetry, and a
// licence is not going to be the exception that phones home — a tool asking to be
// trusted with somebody's entire correspondence cannot also be a tool that reports
// back. So a licence is a signed statement rather than a lookup: the seller writes
// "this covers the archive and the migration, issued on this day", signs it with a
// key nobody else has, and the program checks that signature against a public key
// built into it. Offline, at purchase and forever after.
//
// What that buys and what it cannot:
//
//   - It cannot be forged without the private key, which is not in this repository
//     and never will be.
//   - It can be copied. Two people with the same key both pass, and the program will
//     never know. What stops that is the same thing that stops it everywhere else:
//     most people would rather pay 29 euros than be the sort of person who does not.
//   - It can be removed entirely by anybody who compiles the engine themselves, which
//     is AGPL-3.0 and meant to be compiled by anybody. That is the licence working as
//     intended rather than a hole in this one.
//
// Nothing here is switched on yet. Sells reports whether there is anything to buy,
// it currently answers no, and while it does every part of the program is free —
// because a program that asks for a licence nobody can buy is a program nobody can
// use.
package licence

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// sells is whether there is a shop.
//
// One place, deliberately: when there is somewhere to buy a licence this becomes
// true, and everything that asks what it may do starts asking. Until then every gate
// in the program is open, which is the honest state of a product with no checkout.
//
// A variable rather than a constant so that the tests can open the shop and close it
// again. The gated paths are the ones that must work on the day it opens for real,
// and testing them then would be finding out too late.
var sells = false

// Sells reports whether there is a shop.
func Sells() bool { return sells }

// Grant is something a licence covers.
type Grant string

const (
	// Archive is writing the history out: web pages, plain text, structured data.
	Archive Grant = "archive"
	// Migration is moving an Android history onto an iPhone.
	Migration Grant = "migration"
)

// Reading, searching and the verification report are not grants and never will be.
// Somebody whose phone has died can get their own history onto a screen without
// paying anybody, and that is the point of the whole thing rather than a trial.

// Errors a caller can act on. A licence that will not parse and one that is signed by
// somebody else are different problems: the first is usually a copy-and-paste that
// lost a character, and the second is not.
var (
	// ErrMalformed is a key that is not one, or was truncated on its way here.
	ErrMalformed = errors.New("that does not look like a licence key")
	// ErrForged is a key whose signature is not this project's.
	ErrForged = errors.New("that licence was not issued by Amberkeep")
)

// prefix is what every key starts with, so that a person can see what they are
// holding and a program can refuse a string that was never meant to be one.
const prefix = "AMBERKEEP-1."

// Licence is what a key says.
type Licence struct {
	// Grants is what it covers.
	Grants []Grant `json:"grants"`
	// Issued is the day it was sold.
	Issued string `json:"issued"`
	// Updates is the last day of the twelve months of updates. A build released
	// after it is not covered — and the build the buyer already had goes on working
	// forever, which is the difference between a licence and a hostage.
	Updates string `json:"updates"`
	// Order is the seller's reference, for somebody who has lost their key and needs
	// it reissued. A reference, never a name and never an address.
	Order string `json:"order,omitempty"`
}

// Covers reports whether this licence covers something.
func (l Licence) Covers(grant Grant) bool {
	for _, held := range l.Grants {
		if held == grant {
			return true
		}
	}
	return false
}

// Current reports whether a build made on this day is within the updates the licence
// was sold with.
//
// An unknown build date — which is every build somebody compiled themselves — counts
// as current. The alternative is a program that locks its own developer out, and the
// question this answers is a commercial one rather than a safety one.
func (l Licence) Current(built time.Time) bool {
	if built.IsZero() || l.Updates == "" {
		return true
	}
	until, err := time.Parse(time.DateOnly, l.Updates)
	if err != nil {
		return true
	}
	return !built.After(until)
}

// verifier is the public half of the key this project signs with.
//
// Empty until there is a shop, which is also why nothing verifies yet: a build
// carrying a real public key and no way to buy a licence would be a build that could
// refuse work nobody could pay for.
var verifier ed25519.PublicKey

// Read turns a key somebody pasted into what it says, or says why it will not.
func Read(key string) (Licence, error) {
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, prefix) {
		return Licence{}, ErrMalformed
	}

	parts := strings.Split(strings.TrimPrefix(key, prefix), ".")
	if len(parts) != 2 {
		return Licence{}, ErrMalformed
	}

	said, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Licence{}, ErrMalformed
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Licence{}, ErrMalformed
	}

	var licence Licence
	if err := json.Unmarshal(said, &licence); err != nil {
		return Licence{}, ErrMalformed
	}
	if len(verifier) == ed25519.PublicKeySize && !ed25519.Verify(verifier, said, signature) {
		return Licence{}, ErrForged
	}
	return licence, nil
}

// Write returns the key for a licence, signed with the private half.
//
// This is the seller's side, and it is here rather than hidden because hiding it
// would protect nothing: what cannot be forged is the signature, and the secret is
// the key rather than the method. The private key is not in this repository.
func Write(l Licence, signer ed25519.PrivateKey) (string, error) {
	if len(signer) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("that is not a signing key")
	}
	said, err := json.Marshal(l)
	if err != nil {
		return "", fmt.Errorf("writing the licence: %w", err)
	}
	return prefix +
		base64.RawURLEncoding.EncodeToString(said) + "." +
		base64.RawURLEncoding.EncodeToString(ed25519.Sign(signer, said)), nil
}

// GuidanceNeeded names what to say when something costs money and this computer has
// not paid for it. The words live in internal/guide, in both languages, like every
// other failure somebody can act on.
const GuidanceNeeded = "licence.needed"

// Error is that something costs money and this computer has not paid for it.
//
// Typed rather than a sentinel because what was asked for is worth saying: somebody
// who bought the archive licence and reached the migration should be told which of
// the two they are missing rather than that "a licence" is needed. Every package here
// that can fail in a way somebody could act on carries its errors this way.
type Error struct {
	// Grant is what was asked for and not held.
	Grant Grant
	// Guidance names the advice that goes with it.
	Guidance string
}

func (e *Error) Error() string {
	switch e.Grant {
	case Archive:
		return "writing the archive out needs a licence"
	case Migration:
		return "moving a history onto an iPhone needs a licence"
	default:
		return "that needs a licence"
	}
}

// Is lets a caller ask whether any failure was about a licence.
func (e *Error) Is(target error) bool {
	var other *Error
	return errors.As(target, &other) && (other.Grant == "" || other.Grant == e.Grant)
}

// ErrNeeded matches any failure that was about a licence, whichever it was.
var ErrNeeded = &Error{}

// built is the day this build was made, as the release stamps it.
//
// Set with -X at build time and empty everywhere else, which means a build somebody
// compiled themselves is never judged against the twelve months of updates a licence
// was sold with. That is a commercial question rather than a safety one, and the
// answer to an unknown build date is yes.
var built string

// Built is that day, or the zero time when the build does not say.
func Built() time.Time {
	when, err := time.Parse(time.DateOnly, strings.TrimSpace(built))
	if err != nil {
		return time.Time{}
	}
	return when
}

// Allows says whether this computer may do something.
//
// It answers yes to everything while there is nothing to buy, which is what Sells
// reports. When there is, this is the one question every gate in the program asks,
// and the failure it returns carries the identifier of what to say about it.
func Allows(grant Grant) error {
	if !Sells() {
		return nil
	}

	held, err := Held()
	if err != nil {
		return &Error{Grant: grant, Guidance: GuidanceNeeded}
	}
	if !held.Covers(grant) || !held.Current(Built()) {
		return &Error{Grant: grant, Guidance: GuidanceNeeded}
	}
	return nil
}
