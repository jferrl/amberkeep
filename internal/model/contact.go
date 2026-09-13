package model

// Contact is a person an archive knows about, assembled from every source that had
// something to say: the phone's address book, the names people set in WhatsApp, and
// the addresses in the message database itself.
type Contact struct {
	JID JID

	// Name is what the archive's owner had this person saved as. It comes from the
	// address book and is the most trustworthy label available.
	Name string

	// PushName is the name the person chose for themselves in WhatsApp. It is a real
	// name often enough to be useful and a nickname often enough to be marked as one.
	PushName string

	// Phone is the number in international form without a leading plus, when known.
	Phone string

	// Business marks a verified business account, whose name WhatsApp vouches for.
	Business bool
}

// DisplayName is what to call this person, in descending order of trust: the name
// from the address book, then the name they chose for themselves marked with a
// leading tilde as WhatsApp does, then their phone number, then their raw address.
//
// It returns an empty string in exactly one case: a contact with no name and no
// address. In practice that is an archive whose owner never recorded a name for
// themselves, and the word for it ("You", "Tú") has to be translated, so choosing
// it belongs to the presentation layer rather than here.
func (c Contact) DisplayName() string {
	switch {
	case c.Name != "":
		return c.Name
	case c.PushName != "":
		return "~" + c.PushName
	case c.Phone != "":
		return "+" + c.Phone
	default:
		return c.JID.String()
	}
}

// IsIdentified reports whether the archive found a human name for this person, as
// opposed to falling back to a number or an opaque address. Counting these is how a
// reader tells the user how complete their contact information is.
func (c Contact) IsIdentified() bool {
	return c.Name != "" || c.PushName != ""
}

// Directory resolves addresses to people. Readers fill one while building an
// archive and exporters consult it; neither needs to know which source a name
// came from.
//
// The zero value is not usable; call NewDirectory.
type Directory struct {
	byJID   map[string]Contact
	byPhone map[string]Contact
	// aliases maps a hidden identifier to the phone address it stands for, so a
	// message signed with a hidden identifier still finds its person.
	aliases map[string]JID
	// reverse maps that phone address back to the hidden identifier, because a name
	// is often only recorded against one of the two and must reach both.
	reverse map[string]JID
	owner   Contact
}

// NewDirectory returns an empty directory.
func NewDirectory() *Directory {
	return &Directory{
		byJID:   make(map[string]Contact),
		byPhone: make(map[string]Contact),
		aliases: make(map[string]JID),
		reverse: make(map[string]JID),
	}
}

// Add records what one source knows about a person, merging it with anything
// already known. Better information wins: a name from the address book is never
// replaced by a self-chosen name, and a name is never replaced by nothing.
func (d *Directory) Add(c Contact) {
	key := c.JID.String()
	existing, seen := d.byJID[key]
	if seen {
		if c.Name == "" {
			c.Name = existing.Name
		}
		if c.PushName == "" {
			c.PushName = existing.PushName
		}
		if c.Phone == "" {
			c.Phone = existing.Phone
		}
		c.Business = c.Business || existing.Business
	}
	if c.Phone == "" {
		if phone, ok := c.JID.Phone(); ok {
			c.Phone = phone
		}
	}

	d.byJID[key] = c
	if c.Phone != "" {
		// A phone entry lets a hidden identifier find this person once the alias is
		// known, and lets an address book match by number alone.
		if prior, ok := d.byPhone[c.Phone]; !ok || (!prior.IsIdentified() && c.IsIdentified()) {
			d.byPhone[c.Phone] = c
		}
	}
}

// Alias records that a hidden identifier stands for a phone address. The link is
// recorded both ways, because WhatsApp records a person's name against whichever of
// the two it happened to see, and a reader should not have to care which.
func (d *Directory) Alias(hidden, phone JID) {
	if hidden.IsZero() || phone.IsZero() {
		return
	}
	d.aliases[hidden.String()] = phone
	d.reverse[phone.String()] = hidden
}

// SetOwner records who this archive belongs to, so their own messages can be
// labelled without pretending they are a contact.
func (d *Directory) SetOwner(c Contact) { d.owner = c }

// Owner returns who this archive belongs to.
func (d *Directory) Owner() Contact { return d.owner }

// Lookup returns what is known about an address.
//
// A person can appear under several addresses at once: a phone number, a hidden
// identifier standing for it, and an entry copied from the phone's address book.
// A name recorded against any one of them belongs to all of them, so the search
// follows the link in both directions and then falls back to matching on the phone
// number alone.
//
// It always returns a usable Contact. An address nobody has ever named yields one
// that names itself, so no caller has to invent a fallback.
func (d *Directory) Lookup(j JID) Contact {
	if j.IsZero() {
		return d.owner
	}

	// Start from whatever is recorded against this exact address, keeping the
	// address the caller asked about rather than the one a name was found under.
	best := d.byJID[j.String()]
	best.JID = j
	if best.Phone == "" {
		if phone, ok := j.Phone(); ok {
			best.Phone = phone
		}
	}

	// Everything known about this person is merged rather than stopping at the
	// first record that names them at all. Stopping early would let a name
	// somebody chose for themselves beat a name the archive's owner saved, purely
	// because the two were recorded against different addresses.
	for _, linked := range []JID{d.aliases[j.String()], d.reverse[j.String()]} {
		if linked.IsZero() {
			continue
		}
		if c, ok := d.byJID[linked.String()]; ok {
			best = best.named(c)
		}
	}

	// Two records of the same number that were never explicitly linked.
	if best.Phone != "" {
		if c, ok := d.byPhone[best.Phone]; ok {
			best = best.named(c)
		}
	}
	return best
}

// named returns c with the names taken from source, keeping c's own address and
// number so a caller still sees the address it asked about.
func (c Contact) named(source Contact) Contact {
	if c.Name == "" {
		c.Name = source.Name
	}
	if c.PushName == "" {
		c.PushName = source.PushName
	}
	if c.Phone == "" {
		c.Phone = source.Phone
	}
	c.Business = c.Business || source.Business
	return c
}

// NameOf is the shorthand exporters use: the best label for an address.
func (d *Directory) NameOf(j JID) string { return d.Lookup(j).DisplayName() }

// Len reports how many people the directory knows about.
func (d *Directory) Len() int { return len(d.byJID) }

// Identified reports how many of them have a human name rather than a number.
// The difference between this and Len is what a reader shows the user when it
// explains how complete their contact information is.
func (d *Directory) Identified() int {
	var n int
	for _, c := range d.byJID {
		if c.IsIdentified() {
			n++
		}
	}
	return n
}
