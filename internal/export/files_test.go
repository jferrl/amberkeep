package export

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/model"
)

// folder is an archive whose files are a directory on disk, which is what a real one
// is. Written here rather than reused from the reader because this package must work
// for any archive that can produce a file, and a test against the one implementation
// would prove only that the two agree with each other.
type folder struct {
	root string
	// refuse names the paths this archive will not produce, standing in for the
	// reader's own rule about where a recorded path may lead.
	refuse map[string]bool
}

func (f folder) OpenMedia(ref string) (io.ReadSeekCloser, string, error) {
	if f.refuse[ref] {
		return nil, "", errors.New("that file is not in this folder")
	}
	file, err := os.Open(filepath.Join(f.root, filepath.FromSlash(ref)))
	if err != nil {
		return nil, "", err
	}
	return file, kindOfName(ref), nil
}

// kindOfName is enough of a media type for these tests: what the page does with a
// file depends only on the half before the slash.
func kindOfName(ref string) string {
	switch strings.ToLower(filepath.Ext(ref)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".mp4":
		return "video/mp4"
	case ".opus":
		return "audio/ogg; codecs=opus"
	default:
		return "application/pdf"
	}
}

// laid writes the files an archive refers to and returns it.
func laid(t *testing.T, files map[string]string) folder {
	t.Helper()

	root := t.TempDir()
	for ref, body := range files {
		at := filepath.Join(root, filepath.FromSlash(ref))
		if err := os.MkdirAll(filepath.Dir(at), 0o750); err != nil {
			t.Fatalf("laying out %s: %v", ref, err)
		}
		if err := os.WriteFile(at, []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", ref, err)
		}
	}
	return folder{root: root}
}

// attached returns a message with a file the archive may or may not have.
func attached(kind model.Kind, ref, mediaType string) model.Message {
	m := incoming("")
	m.Kind = kind
	m.Attachment = &model.Attachment{File: ref, MediaType: mediaType}
	return m
}

// TestAnExportCarriesThePhotographsOut is the point of the whole thing: a copy of
// somebody's history with the pictures taken out is not a copy of it.
func TestAnExportCarriesThePhotographsOut(t *testing.T) {
	t.Parallel()

	const picture = "Media/WhatsApp Images/IMG-20190614-WA0001.jpg"
	opts := testOptions(t)
	opts.Files = laid(t, map[string]string{picture: "a photograph"})

	result, err := WriteHTML(conversationOf(theChat, attached(model.KindImage, picture, "image/jpeg")), opts)
	if err != nil {
		t.Fatalf("WriteHTML() failed: %v", err)
	}

	at := filepath.Join(opts.Directory, filepath.FromSlash(picture))
	if body, err := os.ReadFile(at); err != nil || string(body) != "a photograph" {
		t.Fatalf("the file did not travel: %q, %v", body, err)
	}
	if result.Carried != 1 || result.CarriedBytes != int64(len("a photograph")) {
		t.Errorf("it reported %d files and %d bytes carried, want 1 and 12",
			result.Carried, result.CarriedBytes)
	}

	page := readFile(t, result.Files[0])
	// The spaces are escaped, because a href with a raw space in it is a href
	// browsers disagree about.
	if !strings.Contains(page, `src="Media/WhatsApp%20Images/IMG-20190614-WA0001.jpg"`) {
		t.Errorf("the page does not point at the file it carried:\n%s", excerpt(page, "Media/"))
	}
	// And it is not called a recovered preview, because it is not one.
	if strings.Contains(page, "recovered preview") {
		t.Error("the page calls the photograph itself a recovered preview")
	}
}

// TestWhatAPageDoesWithEachKindOfFile: a picture is shown, a recording is played,
// and a document is named rather than embedded, because an <img> pointing at a PDF
// is a broken picture where a sentence would have done.
func TestWhatAPageDoesWithEachKindOfFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ref    string
		kind   model.Kind
		wants  string
		absent string
	}{
		{
			name: "a photograph", ref: "Media/WhatsApp Images/IMG-1.jpg",
			kind: model.KindImage, wants: `<img class="shot"`,
		},
		{
			name: "a video", ref: "Media/WhatsApp Video/VID-1.mp4",
			kind: model.KindVideo, wants: `<video class="shot" controls preload="metadata"`,
		},
		{
			name: "a voice note", ref: "Media/WhatsApp Voice Notes/AUD-1.opus",
			kind: model.KindVoice, wants: `<audio controls preload="metadata"`,
		},
		{
			name: "a document", ref: "Media/WhatsApp Documents/contract.pdf",
			kind: model.KindDocument, wants: `<a href="Media/WhatsApp%20Documents/contract.pdf"`,
			absent: `<img class="shot"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := testOptions(t)
			opts.Files = laid(t, map[string]string{tt.ref: "the file"})

			result, err := WriteHTML(conversationOf(theChat, attached(tt.kind, tt.ref, "")), opts)
			if err != nil {
				t.Fatalf("WriteHTML() failed: %v", err)
			}
			page := readFile(t, result.Files[0])

			if !strings.Contains(page, tt.wants) {
				t.Errorf("the page does not hold %q:\n%s", tt.wants, excerpt(page, "Media/"))
			}
			if tt.absent != "" && strings.Contains(page, tt.absent) {
				t.Errorf("the page holds %q, which it should not", tt.absent)
			}
		})
	}
}

// TestAFileIsCarriedOnce: one photograph is referred to by every message that quotes
// it, and an archive of a million messages must not copy a picture a hundred times
// or report one picture as a hundred.
func TestAFileIsCarriedOnce(t *testing.T) {
	t.Parallel()

	const picture = "Media/WhatsApp Images/IMG-1.jpg"
	opts := testOptions(t)
	opts.Files = laid(t, map[string]string{picture: "a photograph"})

	same := []model.Message{
		attached(model.KindImage, picture, "image/jpeg"),
		attached(model.KindImage, picture, "image/jpeg"),
		attached(model.KindImage, picture, "image/jpeg"),
	}
	result, err := WriteHTML(conversationOf(theChat, same...), opts)
	if err != nil {
		t.Fatalf("WriteHTML() failed: %v", err)
	}
	if result.Carried != 1 {
		t.Errorf("it carried %d files for one photograph mentioned three times", result.Carried)
	}

	// And a second export into the same folder copies nothing again, because the
	// bytes are already there. Several gigabytes of work not done twice.
	again, err := WriteJSON(conversationOf(theChat, same...), opts)
	if err != nil {
		t.Fatalf("WriteJSON() failed: %v", err)
	}
	if again.Carried != 0 {
		t.Errorf("it copied %d files that were already on disk", again.Carried)
	}
}

// TestAnArchiveWithNoFilesExportsExactlyAsBefore. Most archives are a database
// somebody copied on its own, and nothing about this may change what they produce.
func TestAnArchiveWithNoFilesExportsExactlyAsBefore(t *testing.T) {
	t.Parallel()

	message := attached(model.KindImage, "Media/WhatsApp Images/IMG-1.jpg", "image/jpeg")
	message.Attachment.Preview = model.Thumbnail{Data: []byte{0xff, 0xd8, 0xff, 0xe0, 1, 2, 3}}

	tests := []struct {
		name  string
		files Files
	}{
		{name: "an archive that cannot produce files at all"},
		{name: "an archive whose folder does not hold that one", files: folder{
			root:   t.TempDir(),
			refuse: map[string]bool{"Media/WhatsApp Images/IMG-1.jpg": true},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := testOptions(t)
			opts.Files = tt.files

			result, err := WriteHTML(conversationOf(theChat, message), opts)
			if err != nil {
				t.Fatalf("WriteHTML() failed: %v", err)
			}
			if result.Carried != 0 {
				t.Errorf("it carried %d files it does not have", result.Carried)
			}

			page := readFile(t, result.Files[0])
			if !strings.Contains(page, "recovered preview") {
				t.Error("the page dropped the thumbnail it has always shown")
			}
			if !strings.Contains(page, "data:image/jpeg;base64,") {
				t.Error("the thumbnail is no longer inside the page")
			}
		})
	}
}

// TestAPathIsNotAllowedToClimbOutOfTheExport. These paths come out of a database
// that came off somebody's phone; one carrying `../../..` must not write a file
// anywhere but under the folder being written.
func TestAPathIsNotAllowedToClimbOutOfTheExport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  string
	}{
		{name: "climbing out with dots", ref: "../../../../etc/passwd"},
		{name: "climbing out halfway along", ref: "Media/../../../etc/passwd"},
		{name: "an absolute path", ref: "/etc/passwd"},
		{name: "a home-relative path", ref: "~/.ssh/id_rsa"},
		{name: "a Windows drive", ref: `C:\Windows\System32\config\SAM`},
		{name: "a Windows share", ref: `\\server\share\secret`},
		{name: "nothing at all", ref: "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := beneath(tt.ref); ok {
				t.Errorf("beneath(%q) allowed a path that leaves the folder", tt.ref)
			}
		})
	}

	t.Run("and an ordinary path is allowed", func(t *testing.T) {
		t.Parallel()

		within, ok := beneath("Media/WhatsApp Images/IMG-1.jpg")
		if !ok || within != "Media/WhatsApp Images/IMG-1.jpg" {
			t.Errorf("beneath() = %q, %v on an ordinary recorded path", within, ok)
		}
	})
}

// excerpt is the part of a page around a word, for a failure that has to show what
// the page actually says without printing the whole thing.
func excerpt(page, around string) string {
	at := strings.Index(page, around)
	if at < 0 {
		return "(" + around + " does not appear at all)"
	}
	return page[max(0, at-200):min(len(page), at+200)]
}

// TestTheRecordedPathSurvivesWithoutTheFiles is a regression with its own history:
// making the structured format record the path only when the file had just been
// copied quietly took it away from the viewer, which shapes its replies with the
// same code and has no directory to copy anything into. Every photograph in the page
// went back to being a line of text and no test said a word.
func TestTheRecordedPathSurvivesWithoutTheFiles(t *testing.T) {
	t.Parallel()

	const picture = "Media/WhatsApp Images/IMG-1.jpg"
	message := attached(model.KindImage, picture, "image/jpeg")

	// No directory and no files: the shape the server asks for, one message at a
	// time, with nothing being written anywhere.
	value := MessageValue(message, Options{Location: madrid})

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshalling the message: %v", err)
	}
	var out struct {
		Attachment struct {
			File string `json:"file"`
		} `json:"attachment"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if out.Attachment.File != picture {
		t.Errorf("the reference to the file is %q, want %q", out.Attachment.File, picture)
	}
}
