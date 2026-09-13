package backupfs

import (
	"context"
	"crypto/sha1" //nolint:gosec // this is Apple's own backup addressing scheme, not a cryptographic control
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

// flagDirectory is the Files.flags value Apple uses for a directory entry, per the
// Manifest.db format: 1 means a file, 2 means a directory.
const flagDirectory = 2

// fileIDOf computes Apple's content address for one backed-up file: the lowercase
// hex SHA-1 of "<domain>-<relativePath>". It is also the file's name on disk, split
// into a two-character shard directory: <backup>/<fileID[:2]>/<fileID>.
//
// Computing this locally, rather than only ever reading it back out of Manifest.db,
// is what lets Find do a primary-key lookup instead of a scan: the caller's domain
// and relative path are enough to know which row to ask for.
func fileIDOf(domain, relativePath string) string {
	sum := sha1.Sum([]byte(domain + "-" + relativePath)) //nolint:gosec // see above: not a security control
	return hex.EncodeToString(sum[:])
}

// verifyFilesTable confirms Manifest.db actually has the Files table this package
// depends on, with the columns Apple documents for it, before any query assumes they
// exist. Manifest.db is untrusted input like any other file in the backup.
func verifyFilesTable(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(Files)`)
	if err != nil {
		return ErrCorruptManifest.withCause(fmt.Errorf("reading the Files table: %w", err))
	}
	defer rows.Close()

	got := make(map[string]struct{})
	for rows.Next() {
		var (
			cid        int
			name       string
			declType   sql.NullString
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &declType, &notNull, &defaultVal, &primaryKey); err != nil {
			return ErrCorruptManifest.withCause(fmt.Errorf("reading a Files column: %w", err))
		}
		got[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return ErrCorruptManifest.withCause(fmt.Errorf("reading the Files table: %w", err))
	}

	if len(got) == 0 {
		return ErrCorruptManifest.withCause(errors.New("no Files table in Manifest.db"))
	}
	for _, want := range [...]string{"fileID", "domain", "relativePath", "flags", "file"} {
		if _, ok := got[want]; !ok {
			return ErrCorruptManifest.withCause(fmt.Errorf("no %s column in the Files table", want))
		}
	}
	return nil
}

// lookupFile finds one row of the Files table by its primary key. Manifest.db can
// hold entries for hundreds of thousands of files across every app on the phone; this
// is cheap regardless, because it is a primary-key lookup rather than a scan.
func lookupFile(ctx context.Context, db *sql.DB, domain, relativePath string) (File, bool, error) {
	id := fileIDOf(domain, relativePath)

	var (
		flags int64
		blob  []byte
	)
	err := db.QueryRowContext(ctx,
		`SELECT flags, file FROM Files WHERE fileID = ?`, id).Scan(&flags, &blob)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return File{}, false, nil
	case err != nil:
		return File{}, false, ErrUnreadable.withCause(fmt.Errorf("reading %s %s: %w", domain, relativePath, err))
	}

	f, err := fileFromRow(id, domain, relativePath, flags, blob)
	if err != nil {
		return File{}, false, err
	}
	return f, true, nil
}

// listFilesInDomain lists every file recorded under one domain. Only rows matching
// that domain are fetched from Manifest.db, never the whole Files table.
func listFilesInDomain(ctx context.Context, db *sql.DB, domain string) ([]File, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT fileID, relativePath, flags, file FROM Files WHERE domain = ? ORDER BY relativePath`, domain)
	if err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("listing %s: %w", domain, err))
	}
	defer rows.Close()

	var out []File
	for rows.Next() {
		var (
			id           string
			relativePath string
			flags        int64
			blob         []byte
		)
		if err := rows.Scan(&id, &relativePath, &flags, &blob); err != nil {
			return nil, ErrUnreadable.withCause(fmt.Errorf("reading a file in %s: %w", domain, err))
		}
		f, err := fileFromRow(id, domain, relativePath, flags, blob)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("listing %s: %w", domain, err))
	}
	return out, nil
}

// fileFromRow builds a File from one Files row. A directory entry carries no bytes of
// its own: its "file" BLOB is metadata about the directory, not an MBFile describing
// content, so it is never parsed as one.
func fileFromRow(id, domain, relativePath string, flags int64, blob []byte) (File, error) {
	f := File{
		id:           id,
		Domain:       domain,
		RelativePath: relativePath,
		IsDir:        flags == flagDirectory,
	}
	if f.IsDir {
		return f, nil
	}
	meta, err := parseMBFile(blob)
	if err != nil {
		return File{}, ErrCorruptManifest.withCause(fmt.Errorf("%s %s: %w", domain, relativePath, err))
	}
	f.Size = meta.Size
	return f, nil
}
