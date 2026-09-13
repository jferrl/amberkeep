package main

import (
	"errors"

	"github.com/jferrl/amberkeep/internal/crypt15"
	"github.com/jferrl/amberkeep/internal/export"
	"github.com/jferrl/amberkeep/internal/search"
	"github.com/jferrl/amberkeep/internal/source/android"
)

// Every failure a person can do something about carries an identifier from the
// package that raised it, and this is where each one becomes advice.
//
// The point is that nobody should have to search the internet to get past a
// problem this tool already understands. An error that says "the backup could not
// be decrypted with this key" is accurate; an error that also says what to check,
// in what order, is useful.

// advice is what to tell somebody, keyed by the identifier the error carries.
var advice = map[string]string{
	crypt15.GuidanceKeyFormat: `The key is the 64-digit one WhatsApp shows you, in eight groups of eight
characters. Spaces, dashes and line breaks in the file are fine.

If you only have a password, it cannot be used here: WhatsApp releases the
key to the app after checking the password and never lets it leave the
phone. Switch the backup to a 64-digit key and back up again. No chats are
lost by doing this.

  WhatsApp > Settings > Chats > Chat backup > End-to-end encrypted backup`,

	crypt15.GuidanceWrongKey: `Two things produce this, and they cannot be told apart from outside:

  * The key belongs to a different backup. Check you copied the key that
    WhatsApp showed for this phone and this account.
  * The file is incomplete. Copy it off the phone again and compare the
    size with the original before trying once more.`,

	crypt15.GuidancePasskeyNotSupported: `This backup is locked with a passkey, which never leaves the phone, so no
program on a computer can open it.

Switch to a 64-digit key and make a fresh backup. Your chats are untouched
by the change.

  WhatsApp > Settings > Chats > Chat backup > End-to-end encrypted backup`,

	crypt15.GuidanceCrypt14NeedsRoot: `This is an older backup format, locked with a key WhatsApp keeps inside its
own private storage on the phone. Reaching it needs root access.

The straightforward path is to make a new backup instead: turn on
end-to-end encrypted backup with a 64-digit key, tap Back Up, and use the
msgstore.db.crypt15 file it produces.`,

	crypt15.GuidanceMalformedFile: `This does not look like a WhatsApp backup.

The file you want is called msgstore.db.crypt15 and lives in
Android/media/com.whatsapp/WhatsApp/Databases on the phone. The files
beginning "msgstore-increment" are partial and cannot be read on their own.`,

	crypt15.GuidanceUnexpectedPlaintext: `The key was right and the file is intact, but its contents are in a shape
this version does not recognise. That usually means the backup came from a
newer WhatsApp release than this build knows about.

It is worth reporting, with the WhatsApp version that made the backup.`,

	android.GuidanceNotAMessageDatabase: `This is a database, but not the one that holds messages.

After decrypting you should have a file called msgstore.db. Two other files
sit beside it in a backup and are easy to confuse with it: wa.db holds
contacts, and chatsettingsbackup.db holds preferences.`,

	android.GuidanceLegacyUnsupported: `This backup uses the database layout WhatsApp used before 2021, which this
version cannot read yet.

If the phone still works, making a fresh backup will produce the current
layout. Otherwise this is worth reporting: the older layout is documented
and supporting it is a matter of work rather than discovery.`,

	search.GuidanceEmptyQuery: `There are no words in that search.

Type what you remember of the message. A part of a word works too, with a
star at the end: vermu* finds vermut and vermuts.`,

	search.GuidanceNoFullText: `This build cannot search, which should not be possible in a release and is
worth reporting.

Everything else still works: the archive can be exported and read as web
pages, which carry their own search.`,

	search.GuidanceIndexUnreadable: `The search index could not be used. It is a derived file and nothing in the
archive depends on it, so the fix is to build it again:

  amberkeep search --db msgstore.db --rebuild "your search"

If that keeps happening, check there is room on the disk: an index is
roughly a third the size of the archive.`,

	android.GuidanceUnreadable: `The file could not be opened at all. Check that the path is right, that the
file finished copying, and that you have permission to read it.`,
}

// adviseOn returns what to tell somebody about a failure, or nothing when there
// is no advice worth giving beyond the message itself.
func adviseOn(err error) string {
	var decryption *crypt15.Error
	if errors.As(err, &decryption) {
		return advice[decryption.Guidance]
	}

	var reading *android.Error
	if errors.As(err, &reading) {
		return advice[reading.Guidance]
	}

	var searching *search.Error
	if errors.As(err, &searching) {
		return advice[searching.Guidance]
	}

	if errors.Is(err, export.ErrExists) {
		return `An archive is already there, and it will not be overwritten by accident.

Choose an empty directory with --out, or pass --force to replace what is
there. Nothing else in the directory is touched either way.`
	}
	return ""
}
