// Command amberkeep reads, checks and exports a WhatsApp archive from a backup
// you already have, on your own machine.
//
// It makes no network calls, never writes to a file you point it at, and keeps
// your decryption key out of its own output.
//
//	amberkeep decrypt --key key.txt --in msgstore.db.crypt15 --out msgstore.db
//	amberkeep inspect --db msgstore.db
//	amberkeep export  --db msgstore.db --contacts contacts.vcf --out archive/
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
)

// version is stamped at build time. A person reporting a problem needs to be
// able to say which build they were running.
var version = "dev"

// command is one thing the tool can do.
type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string) error
}

func commands() []command {
	return []command{
		{"backups", "list the iPhone backups on this computer", runBackups},
		{"extract", "take WhatsApp's message store out of an iPhone backup", runExtract},
		{"decrypt", "turn an encrypted backup into a readable database", runDecrypt},
		{"inspect", "report what an archive contains, without changing anything", runInspect},
		{"prepare", "make a decrypted database quick to read", runPrepare},
		{"export", "write the archive out as web pages, text and structured data", runExport},
		{"search", "find messages anywhere in the archive", runSearch},
		{"serve", "read the archive in a browser, on this machine only", runServe},
	}
}

func main() { os.Exit(execute()) }

// execute runs the tool and reports the status to exit with.
//
// It is separate from main because os.Exit skips deferred calls, and the signal
// handler has to be released however the run ends.
func execute() int {
	// A long export should stop cleanly when somebody presses control-C, rather
	// than leaving half a file behind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := run(ctx, os.Args[1:])
	switch {
	case err == nil:
		return 0
	case errors.Is(err, context.Canceled):
		fmt.Fprintln(os.Stderr, "\nstopped.")
		return 130
	default:
		fmt.Fprintf(os.Stderr, "\n%s\n", err)
		if advice := adviseOn(err); advice != "" {
			fmt.Fprintf(os.Stderr, "\n%s\n", advice)
		}
		return 1
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage(os.Stdout)
		return nil
	}

	switch args[0] {
	case "-h", "--help", "help":
		usage(os.Stdout)
		return nil
	case "-v", "--version", "version":
		fmt.Println("amberkeep " + version)
		return nil
	}

	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(ctx, args[1:])
		}
	}

	usage(os.Stderr)
	return fmt.Errorf("there is no command called %q", args[0])
}

func usage(w *os.File) {
	fmt.Fprintf(w, `amberkeep %s — read and export your own WhatsApp archive.

Usage:
  amberkeep <command> [options]

Commands:
`, version)
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-9s %s\n", c.name, c.summary)
	}
	fmt.Fprint(w, `
Run "amberkeep <command> --help" for what each one takes.

Getting started, from an iPhone:

  1. Back the phone up to this computer with Finder, with "Encrypt local
     backup" switched off.
  2. amberkeep backups
  3. amberkeep extract --backup <the folder it printed> --out .
     This brings the message store out and the pictures with it.
  4. amberkeep serve   --db ChatStorage.sqlite

Getting started, from an Android phone:

  1. In WhatsApp, turn on end-to-end encrypted backup and choose the
     64-digit key rather than a password. Save the key somewhere safe.
  2. Copy the backup off the phone. It is called msgstore.db.crypt15 and
     lives in Android/media/com.whatsapp/WhatsApp/Databases.
  3. amberkeep decrypt --key key.txt --in msgstore.db.crypt15 --out msgstore.db
  4. amberkeep export  --db msgstore.db --out archive/

Then open archive/index.html. Everything it shows is inside the files:
it works with the network switched off, and always will.

Or, without writing anything to disk:

  amberkeep serve  --db msgstore.db
  amberkeep search --db msgstore.db "whatever you remember"

Nothing here contacts WhatsApp, and nothing leaves this machine.

Not affiliated with or endorsed by WhatsApp LLC or Meta Platforms, Inc.
`)
}

// newFlagSet returns a flag set that prints its own guidance rather than Go's
// default, which mentions the program's internals.
func newFlagSet(name, description string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "amberkeep %s — %s\n\nOptions:\n", name, description)
		fs.PrintDefaults()
	}
	return fs
}

// abbreviate shortens a path for display, so a summary stays readable when the
// user is working somewhere deep in their home directory.
func abbreviate(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if after, found := strings.CutPrefix(path, home); found {
		return "~" + after
	}
	return path
}
