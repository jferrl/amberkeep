package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jferrl/amberkeep/internal/app"
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

	fmt.Printf("%s\n", explainPreparation(app.Abbreviate(*db)))

	prepared, err := android.Prepare(ctx, *db)
	if err != nil {
		return err
	}

	if prepared.NothingToDo() {
		fmt.Printf("already prepared; nothing to do.\n")
		return nil
	}
	fmt.Printf("added %s in %s, %s larger.\n",
		app.Plural(len(prepared.Created), "index", "indexes"),
		prepared.Took.Round(time.Millisecond), app.HumanSize(prepared.Grew))
	return nil
}

// explainPreparation says what is about to change, because this is the one command
// that changes a file somebody named.
func explainPreparation(path string) string {
	return "Adding the indexes WhatsApp's backup leaves out to " + path + ".\n" +
		"Nothing is added to or removed from the conversations: an index only says\n" +
		"where rows already in the file are. The encrypted backup is not touched."
}
