package android

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jferrl/amberkeep/internal/media"
	"github.com/jferrl/amberkeep/internal/model"
)

// TestAPictureIsOfferedOnlyWhenItIsThere covers the thing that makes this worth
// having and the thing that makes it safe to rely on.
//
// A database records where a photograph was on the phone whether or not anybody ever
// copied it; on a real device 92,941 of 99,041 attachments do. An archive that
// offered every one of those as a picture would offer 80,000 broken images to
// somebody who copied the database and nothing else.
func TestAPictureIsOfferedOnlyWhenItIsThere(t *testing.T) {
	t.Parallel()

	db := buildFixture(t)

	t.Run("without the folder, nothing is offered", func(t *testing.T) {
		t.Parallel()

		reader := opened(t, db)
		if file := pictureIn(t, reader); file != "" {
			t.Errorf("an archive with no files offered %q", file)
		}
	})

	t.Run("with a folder that does not hold it, still nothing", func(t *testing.T) {
		t.Parallel()

		folder := phone(t, "Media/WhatsApp Video/VID-0002.mp4")
		reader := opened(t, db, WithMedia(folder))
		if file := pictureIn(t, reader); file != "" {
			t.Errorf("a folder without the picture offered %q", file)
		}
	})

	t.Run("with the folder, the file is offered and can be read", func(t *testing.T) {
		t.Parallel()

		folder := phone(t, "Media/WhatsApp Images/IMG-0001.jpg")
		reader := opened(t, db, WithMedia(folder))

		file := pictureIn(t, reader)
		if file != "Media/WhatsApp Images/IMG-0001.jpg" {
			t.Fatalf("it offered %q", file)
		}

		open, kind, err := reader.OpenMedia(file)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = open.Close() }()

		if kind != "image/jpeg" {
			t.Errorf("kind = %q", kind)
		}
		said, err := io.ReadAll(open)
		if err != nil {
			t.Fatal(err)
		}
		if string(said) != "not a real picture" {
			t.Errorf("it read %q", said)
		}
	})
}

// TestAskingForSomethingThatIsNotThere covers what the page does when a file is
// removed between being listed and being asked for, and what a request carrying
// something else entirely does.
func TestAskingForSomethingThatIsNotThere(t *testing.T) {
	t.Parallel()

	reader := opened(t, buildFixture(t), WithMedia(phone(t, "Media/WhatsApp Images/IMG-0001.jpg")))

	for _, ref := range []string{
		"Media/WhatsApp Images/gone.jpg",
		"../../../etc/passwd",
		"/etc/passwd",
		"",
	} {
		if _, _, err := reader.OpenMedia(ref); !errors.Is(err, media.ErrNotThere) {
			t.Errorf("OpenMedia(%q) = %v, want it refused", ref, err)
		}
	}
}

// TestAnArchiveWithNoFolderSaysThereIsNowhereToLook separates the two nothings: a
// file that is missing from a folder, and no folder at all.
func TestAnArchiveWithNoFolderSaysThereIsNowhereToLook(t *testing.T) {
	t.Parallel()

	reader := opened(t, buildFixture(t))
	if _, _, err := reader.OpenMedia("Media/WhatsApp Images/IMG-0001.jpg"); !errors.Is(err, media.ErrNowhere) {
		t.Errorf("OpenMedia with no folder = %v", err)
	}
	if !reader.Media().Empty() {
		t.Error("a reader with no folder reports one")
	}
}

// phone writes a folder as it comes off a device, holding the files named.
func phone(t *testing.T, files ...string) media.Folder {
	t.Helper()

	root := filepath.Join(t.TempDir(), "WhatsApp")
	if err := os.MkdirAll(filepath.Join(root, "Media"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, at := range files {
		path := filepath.Join(root, filepath.FromSlash(at))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("not a real picture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	folder, ok := media.In(root)
	if !ok {
		t.Fatalf("the folder at %s was not found", root)
	}
	return folder
}

// opened reads the fixture, with whatever options a case needs.
func opened(t *testing.T, path string, options ...Option) *Reader {
	t.Helper()

	reader, err := Open(context.Background(), path, options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	return reader
}

// pictureIn returns what the archive says about where its one photograph is.
func pictureIn(t *testing.T, reader *Reader) string {
	t.Helper()

	chats, err := reader.Chats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, chat := range chats {
		for message, err := range reader.Messages(context.Background(), chat) {
			if err != nil {
				t.Fatal(err)
			}
			if message.Kind == model.KindImage && message.Attachment != nil {
				return message.Attachment.File
			}
		}
	}
	return ""
}
