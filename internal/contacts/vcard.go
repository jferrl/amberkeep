package contacts

import (
	"bufio"
	"fmt"
	"io"
	"mime/quotedprintable"
	"strings"
)

// vCard is the format every phone exports its address book in, and it has three
// incompatible versions in active use.
//
// The awkward parts, all of which appear in real exports from real phones:
//
//   - A long line is folded, and continues on the next line after a space or tab.
//   - Version 2.1 encodes accented names as quoted-printable, with its own kind of
//     line continuation that looks nothing like folding.
//   - A property can be prefixed with a group, as in "item1.TEL".
//   - One person can have any number of phone numbers.
//   - Version 4.0 writes numbers as URIs rather than as numbers.
//
// Getting these wrong does not fail loudly. It quietly drops the accented names,
// which in a Spanish or Portuguese address book is most of them.

// maxVCardBytes bounds a single card. Real cards are a few hundred bytes; the
// limit stops a malformed file from being read into memory without end.
const maxVCardBytes = 1 << 20

// card is one person as the file describes them, before normalisation.
type card struct {
	formatted string   // FN, the name as it should be displayed
	family    string   // from N, used only when FN is absent
	given     string   //
	phones    []string //
}

// name is the best display name this card offers.
func (c card) name() string {
	if c.formatted != "" {
		return c.formatted
	}
	switch {
	case c.given != "" && c.family != "":
		return c.given + " " + c.family
	case c.given != "":
		return c.given
	default:
		return c.family
	}
}

// readVCards parses a vCard file into cards, tolerating anything it does not
// understand rather than failing: an address book that half-parses is far more
// useful than one that refuses to load.
func readVCards(r io.Reader) ([]card, error) {
	lines, err := unfold(r)
	if err != nil {
		return nil, err
	}

	var (
		cards   []card
		current *card
	)
	for _, line := range lines {
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "BEGIN:VCARD"):
			current = &card{}
			continue
		case strings.HasPrefix(upper, "END:VCARD"):
			if current != nil && (current.name() != "" || len(current.phones) > 0) {
				cards = append(cards, *current)
			}
			current = nil
			continue
		}
		if current == nil {
			continue
		}

		property, params, value, ok := splitProperty(line)
		if !ok {
			continue
		}
		value = decodeValue(value, params)

		switch property {
		case "FN":
			current.formatted = unescape(value)
		case "N":
			// The structured name lists family first, then given, then middle names,
			// then any prefix and suffix.
			parts := splitEscaped(value, ';')
			if len(parts) > 0 {
				current.family = unescape(parts[0])
			}
			if len(parts) > 1 {
				current.given = unescape(parts[1])
			}
		case "TEL":
			if value != "" {
				current.phones = append(current.phones, value)
			}
		}
	}
	return cards, nil
}

// unfold reads the file and joins folded lines back together.
//
// It handles both kinds of continuation: the standard one, where a line beginning
// with a space or tab continues the line before it, and quoted-printable's soft
// break, where a line ending in an equals sign continues on the next.
func unfold(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxVCardBytes)

	var (
		lines   []string
		current strings.Builder
	)
	flush := func() {
		if current.Len() > 0 {
			lines = append(lines, current.String())
			current.Reset()
		}
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		switch {
		case line == "":
			flush()
			continue
		case line[0] == ' ' || line[0] == '\t':
			// A folded continuation: the leading whitespace is not part of the value.
			current.WriteString(line[1:])
			continue
		}

		// A quoted-printable soft break also continues, but the marker sits at the
		// end of the previous line rather than the start of this one.
		if strings.HasSuffix(current.String(), "=") {
			s := current.String()
			current.Reset()
			current.WriteString(strings.TrimSuffix(s, "="))
			current.WriteString(line)
			continue
		}

		flush()
		current.WriteString(line)
	}
	flush()

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading the address book: %w", err)
	}
	return lines, nil
}

// splitProperty breaks "item1.TEL;TYPE=CELL:+34600111222" into its parts.
func splitProperty(line string) (property string, params []string, value string, ok bool) {
	head, value, found := strings.Cut(line, ":")
	if !found {
		return "", nil, "", false
	}

	fields := strings.Split(head, ";")
	property = fields[0]
	// A group prefix says which properties belong together and is not part of the
	// property's name.
	if _, after, isGrouped := strings.Cut(property, "."); isGrouped {
		property = after
	}
	return strings.ToUpper(strings.TrimSpace(property)), fields[1:], value, true
}

// decodeValue undoes any encoding the parameters declare.
func decodeValue(value string, params []string) string {
	for _, p := range params {
		if !strings.EqualFold(strings.TrimSpace(p), "ENCODING=QUOTED-PRINTABLE") {
			continue
		}
		decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(value)))
		if err != nil {
			// A card that will not decode keeps its raw value, which is still better
			// than losing the person entirely.
			return value
		}
		return string(decoded)
	}
	return value
}

// splitEscaped splits on a separator, respecting backslash escapes.
func splitEscaped(s string, sep byte) []string {
	var (
		parts   []string
		current strings.Builder
		escaped bool
	)
	for i := range len(s) {
		c := s[i]
		switch {
		case escaped:
			current.WriteByte('\\')
			current.WriteByte(c)
			escaped = false
		case c == '\\':
			escaped = true
		case c == sep:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(c)
		}
	}
	if escaped {
		current.WriteByte('\\')
	}
	return append(parts, current.String())
}

// unescape undoes the escaping the format applies to commas, semicolons and
// newlines inside a value.
func unescape(s string) string {
	replacer := strings.NewReplacer(
		`\\`, `\`,
		`\,`, `,`,
		`\;`, `;`,
		`\n`, "\n",
		`\N`, "\n",
	)
	return strings.TrimSpace(replacer.Replace(s))
}
