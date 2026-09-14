package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jferrl/amberkeep/internal/app"
	"github.com/jferrl/amberkeep/internal/crypt15"
)

// runDecrypt turns an encrypted backup into a database the other commands can read.
func runDecrypt(ctx context.Context, args []string) error {
	fs := newFlagSet("decrypt", "turn an encrypted backup into a readable database")
	var (
		keySource = fs.String("key", "", "the 64-digit key, or a file containing it")
		in        = fs.String("in", "", "the encrypted backup, usually msgstore.db.crypt15")
		out       = fs.String("out", "", "where to write the readable database (default: alongside the backup)")
		force     = fs.Bool("force", false, "replace the output file if it already exists")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *keySource == "" || *in == "" {
		fs.Usage()
		return fmt.Errorf("both --key and --in are needed")
	}

	key, err := readKey(*keySource)
	if err != nil {
		return err
	}

	target := *out
	if target == "" {
		target = app.DefaultOutput(*in)
	}
	if !*force {
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("%s already exists; pass --force to replace it", app.Abbreviate(target))
		}
	}

	// The encrypted file is held whole, because the cipher authenticates all of it
	// before releasing a single byte and a backup is only a few hundred megabytes.
	// #nosec G304 -- reading the file the user named is what --in means.
	encrypted, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("reading the backup: %w", err)
	}
	fmt.Fprintf(os.Stderr, "read %s (%s)\n", app.Abbreviate(*in), app.HumanSize(int64(len(encrypted))))

	if err := ctx.Err(); err != nil {
		return err
	}

	written, err := app.WriteDecrypted(key, encrypted, target)
	if err != nil {
		return err
	}

	fmt.Printf("decrypted to %s (%s)\n", app.Abbreviate(target), app.HumanSize(written))

	// A backup arrives with none of its indexes, so reading it is several times
	// slower than it needs to be. This file did not exist a moment ago and this
	// command wrote it, so putting them back needs nobody's permission.
	app.PrepareOutput(ctx, target)

	fmt.Printf("\nnext: amberkeep inspect --db %s\n", app.Abbreviate(target))
	return nil
}

// readKey loads the key for the command line, where passing it directly has a cost
// worth mentioning: a key typed as an argument is recorded in the shell's history
// and shows up in the list of running processes, where other people on the machine
// can read it.
func readKey(source string) (crypt15.Key, error) {
	key, fromFile, err := app.LoadKey(source)
	if err != nil {
		return crypt15.Key{}, err
	}
	if !fromFile {
		fmt.Fprintln(os.Stderr,
			"warning: passing the key on the command line leaves it in your shell history;\n"+
				"         keeping it in a file and passing that is safer")
	}
	return key, nil
}
