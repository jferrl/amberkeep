// Package model holds the domain types every reader produces and every exporter
// consumes. Nothing here knows about SQLite, about WhatsApp's column names, or
// about which platform a conversation came from.
package model

import "strings"

// Server is the part of a WhatsApp address after the "@". It decides what kind of
// thing the address refers to.
type Server string

// The servers that matter to us. Others exist and are treated as unknown rather
// than rejected, so a new one cannot stop an archive from opening.
const (
	ServerUser       Server = "s.whatsapp.net" // an individual, addressed by phone number
	ServerGroup      Server = "g.us"           // a group
	ServerHidden     Server = "lid"            // an individual behind a hidden identifier
	ServerBroadcast  Server = "broadcast"      // a broadcast list, or status@broadcast
	ServerNewsletter Server = "newsletter"     // a channel
	// ServerStatus and ServerHiddenStatus are how an iPhone addresses one person's
	// status feed. Android keeps a single feed at status@broadcast; iOS keeps one
	// pseudo-conversation per person whose status was seen, which is why an iPhone
	// store can hold hundreds of them and why they are excluded by the same rule.
	ServerStatus       Server = "status"
	ServerHiddenStatus Server = "lid.status"
)

// JID is a WhatsApp address, such as "34600111222@s.whatsapp.net" for a person or
// "123456789-1600000000@g.us" for a group.
//
// WhatsApp increasingly hides phone numbers behind "@lid" identifiers. A JID does
// not resolve those itself; a reader does that while building the archive, so that
// by the time a JID reaches an exporter it is already as identifiable as it can be.
type JID struct {
	// User is the part before the "@": a phone number, a group identifier, or an
	// opaque hidden identifier.
	User string
	// Server says what User refers to.
	Server Server
	// Raw is the address exactly as the source database stored it, kept so an
	// archive can be traced back to its source.
	Raw string
}

// ParseJID splits an address. It never fails: an address it does not understand
// keeps its raw form and reports an unknown server, because refusing to open an
// archive over one odd row would be worse than carrying it through.
func ParseJID(raw string) JID {
	user, server, found := strings.Cut(raw, "@")
	if !found {
		return JID{User: raw, Raw: raw}
	}
	// Device and agent suffixes ("34600111222:12@s.whatsapp.net") identify a
	// particular linked device rather than a different person.
	if base, _, ok := strings.Cut(user, ":"); ok {
		user = base
	}
	return JID{User: user, Server: Server(server), Raw: raw}
}

// IsZero reports whether the address is absent.
func (j JID) IsZero() bool { return j.Raw == "" && j.User == "" }

// IsGroup reports whether the address refers to a group conversation.
func (j JID) IsGroup() bool { return j.Server == ServerGroup }

// IsHidden reports whether the address is a hidden identifier rather than a phone
// number. Those can often be resolved to a person, but not always.
func (j JID) IsHidden() bool { return j.Server == ServerHidden }

// IsStatus reports whether the address is the status feed, which is a pseudo-chat
// most archives exclude by default.
func (j JID) IsStatus() bool {
	if j.Server == ServerStatus || j.Server == ServerHiddenStatus {
		return true
	}
	return j.Server == ServerBroadcast && j.User == "status"
}

// Phone returns the phone number when the address carries one, and reports false
// otherwise. Hidden and group addresses have no phone number of their own.
func (j JID) Phone() (string, bool) {
	if j.Server != ServerUser || j.User == "" {
		return "", false
	}
	return j.User, true
}

// String returns the address as the source stored it.
func (j JID) String() string {
	if j.Raw != "" {
		return j.Raw
	}
	if j.Server == "" {
		return j.User
	}
	return j.User + "@" + string(j.Server)
}
