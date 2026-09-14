package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/fixture"

	_ "modernc.org/sqlite"

	"github.com/jferrl/amberkeep/internal/app"
	"github.com/jferrl/amberkeep/internal/backupfs"
)

// TestExtractTakesTheStoreOutOfABackup runs the path somebody with an iPhone takes:
// a backup folder in, a readable message store out, then every other command.
func TestExtractTakesTheStoreOutOfABackup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	backup := fixture.BackupWithAStore(t, dir)
	out := filepath.Join(dir, "out")

	if err := run(context.Background(), []string{"extract", "--backup", backup, "--out", out}); err != nil {
		t.Fatalf("the extract failed: %v", err)
	}

	store := filepath.Join(out, "ChatStorage.sqlite")
	if _, err := os.Stat(store); err != nil {
		t.Fatalf("the store was not taken out: %v", err)
	}

	t.Run("what came out can be read", func(t *testing.T) {
		if err := run(context.Background(), []string{"inspect", "--db", store}); err != nil {
			t.Errorf("the extracted store could not be read: %v", err)
		}
	})

	t.Run("the pictures come out beside it", func(t *testing.T) {
		// An iPhone store holds only the paths. Without this an archive read from one
		// shows no photographs at all.
		picture := filepath.Join(out, "Media", "a", "b", "one.thumb")
		contents, err := os.ReadFile(picture)
		if err != nil {
			t.Fatalf("the picture was not taken out: %v", err)
		}
		if !bytes.Equal(contents, fixture.OnePixelJPEG) {
			t.Error("what came out is not the picture that was put in")
		}
	})

	t.Run("and reach the messages they belong to", func(t *testing.T) {
		// inspect --full is what counts them, and it reads every message.
		if err := run(context.Background(), []string{"inspect", "--db", store, "--full"}); err != nil {
			t.Errorf("reading the extracted archive failed: %v", err)
		}
	})

	t.Run("the backup itself was not changed", func(t *testing.T) {
		for _, sidecar := range []string{"Manifest.db-wal", "Manifest.db-shm"} {
			if _, err := os.Stat(filepath.Join(backup, sidecar)); err == nil {
				t.Errorf("reading the backup left %s inside it", sidecar)
			}
		}
	})
}

func TestExtractRefusesWhatIsNotABackup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want error
	}{
		{
			name: "no backup named",
			args: []string{"extract"},
		},
		{
			name: "a folder that is not one",
			args: []string{"extract", "--backup", t.TempDir()},
			want: backupfs.ErrNotABackup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := run(context.Background(), tt.args)
			if err == nil {
				t.Fatal("the extract was accepted")
			}
			if tt.want != nil && !errorMatches(err, tt.want) {
				t.Errorf("the error is %v, want %v", err, tt.want)
			}
		})
	}
}

// TestBackupsSurvivesHavingNowhereToLook covers the listing on a machine with no
// backups, which is most machines, and must read as an explanation rather than a
// failure.
func TestBackupsSurvivesHavingNowhereToLook(t *testing.T) {
	// Not parallel: it changes where the home directory is for the duration.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())

	if err := run(context.Background(), []string{"backups"}); err != nil {
		t.Errorf("listing backups on a machine with none failed: %v", err)
	}
}

// TestPrepareMakesAnArchiveQuick checks the one command that writes to a file
// somebody named, including that it says so and that running it twice is safe.
func TestPrepareMakesAnArchiveQuick(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := filepath.Join(dir, "msgstore.db")
	fixture.TinyArchive(t, db)

	before := contentsOf(t, db)
	if err := run(context.Background(), []string{"prepare", "--db", db}); err != nil {
		t.Fatalf("the prepare failed: %v", err)
	}
	if contentsOf(t, db) != before {
		t.Error("preparing changed what the archive holds")
	}

	t.Run("running it again is safe", func(t *testing.T) {
		if err := run(context.Background(), []string{"prepare", "--db", db}); err != nil {
			t.Errorf("the second prepare failed: %v", err)
		}
	})

	t.Run("it says what it is about to change", func(t *testing.T) {
		said := explainPreparation("msgstore.db")
		for _, want := range []string{"msgstore.db", "index", "not touched"} {
			if !strings.Contains(said, want) {
				t.Errorf("the explanation does not mention %q:\n%s", want, said)
			}
		}
	})

	t.Run("with nothing to work on", func(t *testing.T) {
		if err := run(context.Background(), []string{"prepare"}); err == nil {
			t.Error("prepare with no database was accepted")
		}
	})
}

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "the first one", values: []string{"a", "b"}, want: "a"},
		{name: "skipping an empty one", values: []string{"", "b"}, want: "b"},
		{name: "nothing at all", values: []string{"", ""}, want: ""},
		{name: "no values", values: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := app.FirstNonEmpty(tt.values...); got != tt.want {
				t.Errorf("app.FirstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

// contentsOf hashes everything in a database, so a change of any kind shows up.
func contentsOf(t *testing.T, path string) string {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var sum string
	row := db.QueryRow(`SELECT group_concat(t) FROM (
		SELECT quote(_id) || quote(text_data) AS t FROM message ORDER BY _id)`)
	if err := row.Scan(&sum); err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	return sum
}

// errorMatches reports whether a failure is the one expected, however wrapped.
func errorMatches(err, want error) bool {
	return err != nil && want != nil && strings.Contains(err.Error(), want.Error())
}
