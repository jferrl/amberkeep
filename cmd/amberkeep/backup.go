package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jferrl/amberkeep/internal/backupfs"
)

// Where WhatsApp keeps its message store inside an iPhone backup.
const (
	whatsappDomain = "AppDomainGroup-group.net.whatsapp.WhatsApp.shared"
	chatStorage    = "ChatStorage.sqlite"
)

// runBackups lists the iPhone backups on this computer.
//
// It exists because nobody knows where they are. Finder does not say, the folders
// are named after a device identifier nobody recognises, and the two Windows
// locations depend on which iTunes was installed.
func runBackups(ctx context.Context, args []string) error {
	fs := newFlagSet("backups", "list the iPhone backups on this computer")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = ctx

	found, err := backupfs.Backups()
	if err != nil {
		return err
	}
	if len(found) == 0 {
		fmt.Println("No iPhone backups found on this computer.")
		fmt.Printf("\nTo make one: connect the phone, open Finder, choose the phone in the\n")
		fmt.Printf("sidebar, and back up to this Mac. Turn off \"Encrypt local backup\"\n")
		fmt.Printf("first, because an encrypted backup cannot be read by anything but the\n")
		fmt.Printf("phone it came from.\n")
		fmt.Printf("\nIf a backup lives somewhere else, point at its folder directly:\n")
		fmt.Printf("  amberkeep extract --backup /path/to/the/backup --out .\n")
		return nil
	}

	out := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(out, "DEVICE\tMODEL\tIOS\tBACKED UP\tENCRYPTED\tWHERE\n")
	for _, backup := range found {
		encrypted := "no"
		if backup.Encrypted {
			encrypted = "YES, unreadable"
		}
		when := "unknown"
		if !backup.LastBackup.IsZero() {
			when = backup.LastBackup.Local().Format(time.DateOnly)
		}
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\n",
			firstNonEmpty(backup.DeviceName, backup.UDID), backup.ProductType,
			backup.IOSVersion, when, encrypted, abbreviate(backup.Path))
	}
	if err := out.Flush(); err != nil {
		return err
	}

	fmt.Printf("\nnext: amberkeep extract --backup %s --out .\n", abbreviate(found[0].Path))
	return nil
}

// runExtract pulls WhatsApp's message store out of an iPhone backup.
//
// The store is not a file in the backup with a name: Apple stores every file under
// a hash of its domain and path, so finding it means asking the backup's own index.
// It comes out with the two files beside it that hold whatever had not yet been
// written into it, which are folded in on the way so that what lands is complete.
func runExtract(ctx context.Context, args []string) error {
	fs := newFlagSet("extract", "take WhatsApp's message store out of an iPhone backup")
	var (
		dir      = fs.String("backup", "", "the backup folder, named after the device identifier")
		out      = fs.String("out", ".", "directory to write the message store into")
		what     = fs.String("file", chatStorage, "which file to take out of WhatsApp's container")
		pictures = fs.Bool("pictures", true, "also take out the small copies of photographs")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		fs.Usage()
		return fmt.Errorf("--backup is needed; run \"amberkeep backups\" to find one")
	}

	archive, err := backupfs.Open(ctx, *dir)
	if err != nil {
		return err
	}
	defer func() { _ = archive.Close() }()

	fmt.Fprintf(os.Stderr, "%s, %s, iOS %s, backed up %s\n",
		firstNonEmpty(archive.DeviceName, archive.UDID), archive.ProductType,
		archive.IOSVersion, archive.LastBackup.Local().Format(time.DateOnly))

	path, err := archive.ExtractDatabase(ctx, *out, whatsappDomain, *what)
	if err != nil {
		return err
	}

	size := int64(0)
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}
	fmt.Printf("took %s out to %s (%s)\n", *what, abbreviate(path), humanSize(size))

	if *pictures {
		if err := extractPictures(ctx, archive, *out); err != nil {
			return err
		}
	}

	fmt.Printf("\nnext: amberkeep inspect --db %s\n", abbreviate(path))
	return nil
}

// mediaPrefix is where a backup keeps what the store calls "Media/...".
//
// The store records a picture as `Media/<conversation>/5/e/<name>.thumb` and the
// backup files it one directory further in. That offset is written down nowhere;
// it was established by hashing the store's own paths against a real backup's index,
// where 9,422 of 9,941 then resolved and every one of them was a JPEG.
const mediaPrefix = "Message/"

// thumbnailSuffix marks the small copies, which are the ones worth having.
//
// The full-size files are in the same place and are not taken: the same device
// holds 5.7 GB of them against 13.9 MB of these, they are already in the phone's
// own gallery, and copying gigabytes to say what is already said helps nobody.
const thumbnailSuffix = ".thumb"

// extractPictures copies out the small copies of photographs.
//
// This is the difference between an iPhone archive that shows a decade of pictures
// and one that shows a decade of the words "image omitted". An Android database
// keeps them inside itself; an iPhone store keeps only the paths.
func extractPictures(ctx context.Context, archive *backupfs.Archive, out string) error {
	files, err := archive.Domain(ctx, whatsappDomain)
	if err != nil {
		return err
	}

	var taken, bytes int64
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if file.IsDir || !strings.HasSuffix(file.RelativePath, thumbnailSuffix) {
			continue
		}
		where, ok := strings.CutPrefix(file.RelativePath, mediaPrefix)
		if !ok {
			continue
		}

		// Laid out as the store's own paths expect, so the folder is readable on its
		// own once the backup it came from is gone.
		destination := filepath.Join(out, filepath.FromSlash(where))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return fmt.Errorf("preparing somewhere for the pictures: %w", err)
		}
		if err := archive.Extract(ctx, destination, file); err != nil {
			// One picture that cannot be copied is one picture missing, not a reason
			// to abandon the rest.
			continue
		}
		taken++
		bytes += file.Size
	}

	if taken == 0 {
		fmt.Printf("this backup holds no pictures\n")
		return nil
	}
	fmt.Printf("took out %s as well (%s)\n",
		plural(int(taken), "picture", "pictures"), humanSize(bytes))
	return nil
}

// firstNonEmpty returns the first value with anything in it.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
