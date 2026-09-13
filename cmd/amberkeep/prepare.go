package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jferrl/amberkeep/internal/source/android"
)

// runPrepare makes a decrypted database quick to read.
//
// It is separate from every other command because it is the only one that writes to
// a file the user supplied. What it writes is derived data and nothing else: an
// index holds no information that is not already in the table beside it, and
// deleting one again loses nothing. Running this command is the consent.
func runPrepare(ctx context.Context, args []string) error {
	fs := newFlagSet("prepare", "make a decrypted database quick to read")
	db := fs.String("db", "", "the decrypted message database, usually msgstore.db")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *db == "" {
		fs.Usage()
		return fmt.Errorf("--db is needed")
	}

	fmt.Printf("%s\n", explainPreparation(abbreviate(*db)))

	prepared, err := android.Prepare(ctx, *db)
	if err != nil {
		return err
	}

	if prepared.NothingToDo() {
		fmt.Printf("already prepared; nothing to do.\n")
		return nil
	}
	fmt.Printf("added %s in %s, %s larger.\n",
		plural(len(prepared.Created), "index", "indexes"),
		prepared.Took.Round(time.Millisecond), humanSize(prepared.Grew))
	return nil
}

// explainPreparation says what is about to change, because this is the one command
// that changes a file somebody named.
func explainPreparation(path string) string {
	return "Adding the indexes WhatsApp's backup leaves out to " + path + ".\n" +
		"Nothing is added to or removed from the conversations: an index only says\n" +
		"where rows already in the file are. The encrypted backup is not touched."
}

// prepareOutput adds the indexes to a database this program itself just wrote.
//
// No permission is asked because nothing was asked to be preserved: the file did
// not exist a second ago, this command made it, and the backup it came from is
// untouched. A failure is worth a line and nothing more, since the database is
// perfectly readable without them, only slower.
func prepareOutput(ctx context.Context, path string) {
	prepared, err := android.Prepare(ctx, path)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"note: could not add the indexes that make reading quick (%v); everything still works\n", err)
		return
	}
	if prepared.NothingToDo() {
		return
	}
	fmt.Fprintf(os.Stderr, "added %s so reading it is quick (%s, %s larger)\n",
		plural(len(prepared.Created), "index", "indexes"),
		prepared.Took.Round(time.Millisecond), humanSize(prepared.Grew))
}

// mentionPreparation tells somebody reading a slow archive that it need not be.
//
// The check is an optional one: a reader that cannot answer is a reader for which
// the question does not arise.
func mentionPreparation(reader any, path string) {
	indexed, asks := reader.(interface{ Indexed() bool })
	if !asks || indexed.Indexed() {
		return
	}
	fmt.Fprintf(os.Stderr,
		"note: this database has none of the indexes that make it quick to read, which\n"+
			"      is how WhatsApp's backups arrive. Reading it will take several times\n"+
			"      longer than it needs to. To fix that once, in about a second:\n\n"+
			"        amberkeep prepare --db %s\n\n", abbreviate(path))
}
