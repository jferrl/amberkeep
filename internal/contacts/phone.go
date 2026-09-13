// Package contacts turns the places people keep names into an address book the
// rest of the archive can use.
//
// This matters more than it sounds. A message database identifies everybody by
// phone number or by an opaque identifier, and in a real archive only a few
// thousand of a hundred thousand known people have a name of any kind. Without an
// address book, a decade of conversation reads as a list of numbers.
//
// Nothing here touches the network. Names come from files the user already has:
// a vCard export, WhatsApp's own contacts database, or a listing of the phone's
// address book.
package contacts

import "strings"

// NormalizePhone reduces a written phone number to the digits WhatsApp uses to
// identify somebody: the full international number with no plus and no spacing.
//
// It reports false when the input is not a usable number at all.
//
// defaultCountry is applied to numbers written the way people write them at home,
// without any international prefix. It is a dialling code without a plus, such as
// "34" for Spain. When it is empty, a national number is returned as it stands and
// simply will not match anybody.
func NormalizePhone(raw, defaultCountry string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	// A vCard 4.0 number is a URI, sometimes with an extension attached.
	if rest, ok := cutPrefixFold(raw, "tel:"); ok {
		raw = rest
	}
	if i := strings.IndexAny(raw, ";x"); i > 0 {
		// An extension identifies a desk phone behind a switchboard and is never
		// part of a WhatsApp address.
		if _, isExt := cutPrefixFold(raw[i:], ";ext="); isExt || raw[i] == 'x' {
			raw = raw[:i]
		}
	}

	international := strings.HasPrefix(raw, "+")
	digits := onlyDigits(raw)
	switch {
	case digits == "":
		return "", false
	case international:
		return digits, true
	case strings.HasPrefix(digits, "00"):
		// The older way of writing an international number.
		return strings.TrimPrefix(digits, "00"), true
	}

	if defaultCountry == "" {
		return digits, len(digits) >= minimumDigits
	}

	// A number written for people in the same country. Some countries prefix those
	// with a trunk digit that is dropped when the country code is added.
	national := strings.TrimPrefix(digits, "0")
	if strings.HasPrefix(digits, defaultCountry) && len(digits) >= len(defaultCountry)+minimumDigits {
		// It already carries its country code, written without a plus.
		return digits, true
	}
	if len(national) < minimumDigits {
		return "", false
	}
	return defaultCountry + national, true
}

// minimumDigits is the shortest a real subscriber number gets. Anything shorter is
// a short code, an emergency number or a typo, none of which identify a person.
const minimumDigits = 6

// onlyDigits strips everything people put in phone numbers to make them readable.
func onlyDigits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cutPrefixFold is strings.CutPrefix, ignoring case.
func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return s, false
	}
	return s[len(prefix):], true
}
