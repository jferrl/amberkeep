package media

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// phone builds the folder as it comes off an Android device: a WhatsApp directory
// with Media inside it. Everything written here is invented.
func phone(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "WhatsApp")
	for _, at := range []string{
		"Media/WhatsApp Images/IMG-20190614-WA0001.jpg",
		"Media/WhatsApp Images/Sent/IMG-20190615-WA0002.jpg",
		"Media/WhatsApp Video/VID-20190616-WA0003.mp4",
		"Media/WhatsApp Voice Notes/20190617/PTT-20190617-WA0004.opus",
		"Media/WhatsApp Documents/a recipe.pdf",
	} {
		path := filepath.Join(root, filepath.FromSlash(at))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("not a real picture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestTheFolderIsFoundHoweverSomebodyCopiedIt(t *testing.T) {
	t.Parallel()

	root := phone(t)

	tests := []struct {
		name  string
		at    string
		found bool
	}{
		{name: "the WhatsApp folder, which is what the phone calls it", at: root, found: true},
		{name: "the Media folder inside it, which is what people copy", at: filepath.Join(root, "Media"), found: true},
		{name: "a folder with neither in it", at: filepath.Dir(root), found: false},
		{name: "nothing at all", at: "", found: false},
		{name: "somewhere that is not there", at: filepath.Join(root, "nope"), found: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			folder, ok := In(tt.at)
			if ok != tt.found {
				t.Fatalf("In(%q) = %v, want %v", tt.at, ok, tt.found)
			}
			if !ok {
				return
			}
			// Whichever was handed over, the paths the database records resolve.
			if !folder.Holds("Media/WhatsApp Images/IMG-20190614-WA0001.jpg") {
				t.Errorf("it does not hold a file it should, from root %q", folder.Where())
			}
		})
	}
}

func TestWhatIsThereAndWhatIsNot(t *testing.T) {
	t.Parallel()

	folder, ok := In(phone(t))
	if !ok {
		t.Fatal("the folder was not found")
	}

	if !folder.Holds("Media/WhatsApp Voice Notes/20190617/PTT-20190617-WA0004.opus") {
		t.Error("a voice note that is there was not found")
	}
	// A folder copied off a phone is routinely partial: somebody copies the images
	// and not the video, and an archive that offers a picture it cannot produce is
	// worse than one that offers nothing.
	if folder.Holds("Media/WhatsApp Images/IMG-20200101-WA9999.jpg") {
		t.Error("a file that is not there was reported as being there")
	}
	if folder.Size("Media/WhatsApp Images/IMG-20190614-WA0001.jpg") != 18 {
		t.Errorf("size = %d", folder.Size("Media/WhatsApp Images/IMG-20190614-WA0001.jpg"))
	}
}

// TestNothingLeadsOutOfTheFolder is the one that matters. These paths come out of a
// database that came off somebody's phone, and this turns them into files on the
// reader's own disk.
func TestNothingLeadsOutOfTheFolder(t *testing.T) {
	t.Parallel()

	root := phone(t)
	outside := filepath.Join(filepath.Dir(root), "secrets.txt")
	if err := os.WriteFile(outside, []byte("not for you"), 0o600); err != nil {
		t.Fatal(err)
	}

	folder, ok := In(root)
	if !ok {
		t.Fatal("the folder was not found")
	}

	escapes := []string{
		"../secrets.txt",
		"Media/../../secrets.txt",
		"Media/WhatsApp Images/../../../secrets.txt",
		"..",
		"../",
		"/etc/passwd",
		"~/.ssh/id_rsa",
		"Media/./../../secrets.txt",
		`..\secrets.txt`,
		"C:\\Windows\\win.ini",
	}

	for _, escape := range escapes {
		t.Run(escape, func(t *testing.T) {
			t.Parallel()

			if folder.Holds(escape) {
				t.Errorf("%q was resolved to something", escape)
			}
			if _, _, err := folder.Open(escape); !errors.Is(err, ErrNotThere) {
				t.Errorf("Open(%q) = %v, want it refused", escape, err)
			}
			if got := folder.Size(escape); got != 0 {
				t.Errorf("Size(%q) = %d", escape, got)
			}
		})
	}
}

func TestOpeningWhatIsThere(t *testing.T) {
	t.Parallel()

	folder, ok := In(phone(t))
	if !ok {
		t.Fatal("the folder was not found")
	}

	file, kind, err := folder.Open("Media/WhatsApp Video/VID-20190616-WA0003.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	if kind != "video/mp4" {
		t.Errorf("kind = %q, want video/mp4", kind)
	}
	said, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(said) != "not a real picture" {
		t.Errorf("it read %q", said)
	}
}

// TestAnArchiveWithNoFilesSaysSo covers the ordinary case, which is not an error:
// a database copied off a phone on its own has no photographs with it.
func TestAnArchiveWithNoFilesSaysSo(t *testing.T) {
	t.Parallel()

	var nowhere Folder
	if !nowhere.Empty() {
		t.Error("the zero folder is not empty")
	}
	if nowhere.Holds("Media/WhatsApp Images/IMG-20190614-WA0001.jpg") {
		t.Error("the zero folder holds something")
	}
	if _, _, err := nowhere.Open("Media/anything.jpg"); !errors.Is(err, ErrNowhere) {
		t.Errorf("Open on nothing = %v, want it to say there is nowhere to look", err)
	}
}

func TestWhatKindOfFileItIs(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, want string }{
		{name: "IMG-20190614-WA0001.jpg", want: "image/jpeg"},
		{name: "VID-20190616-WA0003.mp4", want: "video/mp4"},
		{name: "STK-20190618-WA0005.webp", want: "image/webp"},
		{name: "a recipe.pdf", want: "application/pdf"},
		{name: "IMG-20190614-WA0001.JPG", want: "image/jpeg"},
		{name: "a file nobody named", want: "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := KindOf(tt.name); !strings.HasPrefix(got, tt.want) {
				t.Errorf("KindOf(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestCountingWhatArrived is what a program tells somebody who has just copied a
// folder across: how much of their archive it filled in.
func TestCountingWhatArrived(t *testing.T) {
	t.Parallel()

	folder, ok := In(phone(t))
	if !ok {
		t.Fatal("the folder was not found")
	}

	recorded := []string{
		"Media/WhatsApp Images/IMG-20190614-WA0001.jpg",
		"Media/WhatsApp Images/Sent/IMG-20190615-WA0002.jpg",
		"Media/WhatsApp Images/IMG-20200101-WA9999.jpg", // never copied
		"../secrets.txt", // never anything
	}
	if got := Count(folder, recorded); got != 2 {
		t.Errorf("Count = %d, want 2 of the four", got)
	}
}

// TestASymbolicLinkOutOfTheFolderIsNotFollowed is the other way out, and the one a
// path check alone does not catch.
func TestASymbolicLinkOutOfTheFolderIsNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need a privilege on Windows that a test should not ask for")
	}
	t.Parallel()

	root := phone(t)
	outside := filepath.Join(filepath.Dir(root), "secrets.txt")
	if err := os.WriteFile(outside, []byte("not for you"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "Media", "WhatsApp Images", "escape.jpg")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	folder, ok := In(root)
	if !ok {
		t.Fatal("the folder was not found")
	}

	file, _, err := folder.Open("Media/WhatsApp Images/escape.jpg")
	if err == nil {
		_ = file.Close()
		t.Error("a link pointing out of the folder was followed")
	}
}
