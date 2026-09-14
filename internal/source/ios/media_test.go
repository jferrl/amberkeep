package ios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// TestFolderMediaStaysWhereItBelongs is the security property.
//
// The path comes out of a database, and a database is a file somebody was handed.
// A store carrying "../../../../etc/passwd" where a picture's name should be would
// otherwise have this read it and put it in an archive, which is the whole of the
// vulnerability. Nothing outside the directory may be reachable by any spelling.
func TestFolderMediaStaysWhereItBelongs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inside := filepath.Join(root, "Media", "a", "b")
	if err := os.MkdirAll(inside, 0o700); err != nil {
		t.Fatalf("preparing the directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inside, "one.thumb"), []byte("a picture"), 0o600); err != nil {
		t.Fatalf("writing the picture: %v", err)
	}

	// Something worth stealing, one level above where the pictures are.
	secret := filepath.Join(filepath.Dir(root), "secret.txt")
	if err := os.WriteFile(secret, []byte("not yours"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(secret) })

	media, found := MediaIn(root)
	if !found {
		t.Fatal("MediaIn() found no pictures where some were put")
	}

	t.Run("a picture that is there is read", func(t *testing.T) {
		contents, ok := media.ReadFile("Media/a/b/one.thumb")
		if !ok {
			t.Fatal("a picture that exists was not read")
		}
		if string(contents) != "a picture" {
			t.Errorf("read %q", contents)
		}
	})

	escapes := []struct {
		name string
		path string
	}{
		{name: "climbing out", path: "../secret.txt"},
		{name: "climbing out the long way", path: "Media/a/../../../secret.txt"},
		{name: "an absolute path", path: secret},
		{name: "a root path", path: "/etc/passwd"},
		{name: "a windows-shaped climb", path: `..\secret.txt`},
		{name: "climbing then coming back", path: "Media/../../secret.txt"},
		{name: "nothing at all", path: ""},
		{name: "just a dot", path: "."},
	}

	for _, tt := range escapes {
		t.Run(tt.name, func(t *testing.T) {
			if contents, ok := media.ReadFile(tt.path); ok {
				t.Errorf("reading %q was allowed and returned %d bytes", tt.path, len(contents))
			}
		})
	}

	t.Run("a directory is not a picture", func(t *testing.T) {
		if _, ok := media.ReadFile("Media/a"); ok {
			t.Error("a directory was read as a picture")
		}
	})

	t.Run("something absurdly large is refused", func(t *testing.T) {
		// A store naming an enormous file under a picture's name is not describing a
		// picture, and reading it would be the bug rather than the point.
		huge := filepath.Join(inside, "huge.thumb")
		if err := os.WriteFile(huge, make([]byte, maxPictureBytes+1), 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
		if _, ok := media.ReadFile("Media/a/b/huge.thumb"); ok {
			t.Error("a file larger than any picture was read")
		}
	})
}

func TestMediaInFindsNothingWhenThereIsNothing(t *testing.T) {
	t.Parallel()

	t.Run("an empty directory has no pictures", func(t *testing.T) {
		t.Parallel()
		// A store copied out on its own is the ordinary case, and it is not a failure.
		if _, found := MediaIn(filepath.Join(t.TempDir(), "absent")); found {
			t.Error("MediaIn() claimed pictures in a directory that is not there")
		}
	})
}

// TestMediaFromAFileSystem covers the adapter the tests and any future source use.
func TestMediaFromAFileSystem(t *testing.T) {
	t.Parallel()

	media := MediaFrom(fstest.MapFS{
		"Media/a/b/one.thumb":  {Data: []byte("a picture")},
		"Media/a/b/huge.thumb": {Data: make([]byte, maxPictureBytes+1)},
	})

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "a picture that is there", path: "Media/a/b/one.thumb", want: true},
		{name: "one that is not", path: "Media/a/b/missing.thumb"},
		{name: "climbing out", path: "../elsewhere"},
		{name: "an absolute path", path: "/etc/passwd"},
		{name: "something absurdly large", path: "Media/a/b/huge.thumb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, ok := media.ReadFile(tt.path); ok != tt.want {
				t.Errorf("reading %q was %v, want %v", tt.path, ok, tt.want)
			}
		})
	}
}

// TestPicturesReachTheMessagesTheyBelongTo is the recovery itself: a store that
// holds only a path, plus somewhere to follow it, gives an archive with photographs
// in it. Without the second half there are none at all.
func TestPicturesReachTheMessagesTheyBelongTo(t *testing.T) {
	t.Parallel()

	// A single-pixel JPEG. What it depicts does not matter; that it arrives does.
	picture := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0xff, 0xd9}

	tests := []struct {
		name  string
		media Media
		want  bool
	}{
		{
			name:  "with somewhere to follow the path",
			media: MediaFrom(fstest.MapFS{fixtureThumbPath: {Data: picture}}),
			want:  true,
		},
		{
			name:  "with nowhere to follow it",
			media: nil,
		},
		{
			name:  "when the picture is not where the store says",
			media: MediaFrom(fstest.MapFS{"Media/elsewhere.thumb": {Data: picture}}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := openFixtureWith(t, tt.media)
			found := messagesByID(t, r)

			photograph, ok := found[msgWithPicture]
			if !ok {
				t.Fatalf("the message carrying a picture was not read at all")
			}
			if got := photograph.Attachment != nil && photograph.Attachment.HasPreview(); got != tt.want {
				t.Errorf("the picture arrived = %v, want %v", got, tt.want)
			}
			if tt.want && !strings.HasPrefix(string(photograph.Attachment.Preview.Data), "\xff\xd8\xff") {
				t.Error("what arrived is not the picture that was put there")
			}
		})
	}
}
