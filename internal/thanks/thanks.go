// Package thanks is where somebody can say so, if they want to.
//
// This program is free and stays free: reading a history back off a dead phone,
// searching it, writing it out, moving it onto another phone. There is no licence,
// no key, no tier and nothing switched off — see docs/adr/0010-a-tip-jar.md for why
// that is a decision rather than a stage on the way to charging.
//
// What there is instead is one line, shown twice in the life of the program: when an
// archive has been written out, and when a migration has produced the backup. Both
// are moments when something that mattered has just worked. Never on a failure,
// never at startup, never twice in a row, and never in the way of anything.
package thanks

// Address is where to say it.
//
// Empty means there is nowhere, and everything that would show it shows nothing —
// a program that points somebody at a page which does not exist has spent the only
// goodwill this line was ever going to earn. It is written in full, scheme and all,
// because a terminal makes an address clickable and a page needs one to link to;
// what is shown to a person is the address without the scheme, which is how anybody
// would write it down.
const Address = "https://ko-fi.com/jferrl"

// Asked reports whether there is anywhere to point somebody.
func Asked() bool { return Address != "" }
