package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		target = defaultOutput(*in)
	}
	if !*force {
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("%s already exists; pass --force to replace it", abbreviate(target))
		}
	}

	// The encrypted file is held whole, because the cipher authenticates all of it
	// before releasing a single byte and a backup is only a few hundred megabytes.
	// #nosec G304 -- reading the file the user named is what --in means.
	encrypted, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("reading the backup: %w", err)
	}
	fmt.Fprintf(os.Stderr, "read %s (%s)\n", abbreviate(*in), humanSize(int64(len(encrypted))))

	if err := ctx.Err(); err != nil {
		return err
	}

	written, err := writeDecrypted(key, encrypted, target)
	if err != nil {
		return err
	}

	fmt.Printf("decrypted to %s (%s)\n", abbreviate(target), humanSize(written))

	// A backup arrives with none of its indexes, so reading it is several times
	// slower than it needs to be. This file did not exist a moment ago and this
	// command wrote it, so putting them back needs nobody's permission.
	prepareOutput(ctx, target)

	fmt.Printf("\nnext: amberkeep inspect --db %s\n", abbreviate(target))
	return nil
}

// writeDecrypted turns the backup into a database on disk.
//
// The database is written through rather than assembled in memory first. Holding
// it peaked at a gigabyte and a half for a 236 MB backup, because growing the
// plaintext buffer leaves the old copy and the new one live at the same moment
// while the ciphertext is still referenced.
//
// A failure takes the file with it. A half-written database is worse than none:
// it looks like something somebody could open.
func writeDecrypted(key crypt15.Key, encrypted []byte, target string) (int64, error) {
	// Created for this user only: a decrypted archive is every message they have
	// ever sent or received.
	// #nosec G304 -- the destination is the path the user asked to write to.
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, fmt.Errorf("creating the database: %w", err)
	}

	abandon := func(cause error) (int64, error) {
		_ = file.Close()
		_ = os.Remove(target)
		return 0, cause
	}

	out := bufio.NewWriterSize(file, 1<<20)
	written, err := crypt15.DecryptTo(key, encrypted, out)
	if err != nil {
		return abandon(err)
	}
	if err := out.Flush(); err != nil {
		return abandon(fmt.Errorf("writing the database: %w", err))
	}
	if err := file.Close(); err != nil {
		return abandon(fmt.Errorf("writing the database: %w", err))
	}
	return written, nil
}

// loadKey reads the key from a file, or accepts it directly, and says which it was.
//
// Both spellings are accepted because both are what people have: WhatsApp shows the
// key on screen to be written down, and anybody sensible then saves it in a file.
// Which one arrived matters only to the caller, who may have something to say about
// it, so it is reported rather than remarked on here.
func loadKey(source string) (key crypt15.Key, fromFile bool, err error) {
	// #nosec G304 -- reading the file the user named is what --key means.
	if contents, err := os.ReadFile(source); err == nil {
		key, err := crypt15.ParseKey(string(contents))
		if err != nil {
			return crypt15.Key{}, true, fmt.Errorf("reading the key from %s: %w", abbreviate(source), err)
		}
		return key, true, nil
	}

	// Not a file, so treat it as the key itself. The error deliberately does not
	// repeat what was passed, in case it ends up in a log.
	key, err = crypt15.ParseKey(source)
	if err != nil {
		return crypt15.Key{}, false, fmt.Errorf("that is neither a readable file nor a valid key: %w", err)
	}
	return key, false, nil
}

// readKey loads the key for the command line, where passing it directly has a cost
// worth mentioning: a key typed as an argument is recorded in the shell's history
// and shows up in the list of running processes, where other people on the machine
// can read it.
func readKey(source string) (crypt15.Key, error) {
	key, fromFile, err := loadKey(source)
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

// defaultOutput picks a name beside the backup, so the simple case needs no
// second path.
func defaultOutput(in string) string {
	base := filepath.Base(in)
	for _, suffix := range []string{".crypt15", ".crypt14", ".crypt12"} {
		if trimmed, found := strings.CutSuffix(base, suffix); found {
			base = trimmed
			break
		}
	}
	if base == filepath.Base(in) {
		base += ".decrypted"
	}
	return filepath.Join(filepath.Dir(in), base)
}

// humanSize renders a byte count the way people talk about files.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d bytes", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
