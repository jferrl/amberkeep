package main

import (
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

	// The whole file is held in memory. A backup is a few hundred megabytes, and
	// the alternative is streaming state that could corrupt the output if it were
	// ever wrong, which is a poor trade for somebody's only copy.
	encrypted, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("reading the backup: %w", err)
	}
	fmt.Fprintf(os.Stderr, "read %s (%s)\n", abbreviate(*in), humanSize(int64(len(encrypted))))

	if err := ctx.Err(); err != nil {
		return err
	}

	plaintext, err := crypt15.Decrypt(key, encrypted)
	if err != nil {
		return err
	}

	// Written for this user only: a decrypted archive is every message they have
	// ever sent or received.
	// #nosec G703 -- the destination is the path the user asked to write to.
	if err := os.WriteFile(target, plaintext, 0o600); err != nil {
		return fmt.Errorf("writing the database: %w", err)
	}

	fmt.Printf("decrypted to %s (%s)\n", abbreviate(target), humanSize(int64(len(plaintext))))
	fmt.Printf("\nnext: amberkeep inspect --db %s\n", abbreviate(target))
	return nil
}

// readKey loads the key from a file, or accepts it directly.
//
// A file is the better way round: a key typed on the command line is recorded in
// the shell's history and shows up in the list of running processes, where other
// people on the machine can read it.
func readKey(source string) (crypt15.Key, error) {
	// #nosec G304 -- reading the file the user named is what --key means.
	if contents, err := os.ReadFile(source); err == nil {
		key, err := crypt15.ParseKey(string(contents))
		if err != nil {
			return crypt15.Key{}, fmt.Errorf("reading the key from %s: %w", abbreviate(source), err)
		}
		return key, nil
	}

	// Not a file, so treat it as the key itself. The error deliberately does not
	// repeat what was passed, in case it ends up in a log.
	key, err := crypt15.ParseKey(source)
	if err != nil {
		return crypt15.Key{}, fmt.Errorf("--key is neither a readable file nor a valid key: %w", err)
	}
	fmt.Fprintln(os.Stderr,
		"warning: passing the key on the command line leaves it in your shell history;\n"+
			"         keeping it in a file and passing that is safer")
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
