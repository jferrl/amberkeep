package export

import (
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Carrying the archive's own files out with it.
//
// A WhatsApp database records where a photograph was and keeps at most a thumbnail
// of it, so an exported page has always shown the thumbnail and said the file itself
// is not here. When somebody has brought the phone's folder, it is here, and an
// export that left it behind would be a copy of somebody's history with the
// photographs taken out.
//
// So the files travel too: each one is copied beside the pages, under the path the
// phone recorded, and the page links to it. The folder as a whole is what is
// self-contained now rather than each page on its own — it still opens with the
// network switched off, from a USB stick, in ten years, which is the promise that
// actually matters. A page whose file is not there is exactly what it was before.

// Files is an archive that can hand over the files its messages refer to.
//
// The archive is asked rather than a directory being read, because where a recorded
// path is allowed to lead is the reader's business and nobody else's: these paths
// came off somebody's phone, and one carrying `../../../..` must not produce a file
// outside the folder it names. An archive that cannot do this is nil here, which is
// the ordinary case.
type Files interface {
	// OpenMedia returns one file and what kind of thing it is. The caller closes it.
	OpenMedia(ref string) (io.ReadSeekCloser, string, error)
}

// carried is one file that travelled, as the page needs it.
type carried struct {
	// Link is where the page points, relative to the page itself.
	Link string
	// Kind is the media type the archive gave it, for choosing a picture over a
	// player.
	Kind string
	// Bytes is how large it is.
	Bytes int64
}

// carrier copies an archive's files into the directory being written.
//
// It remembers what it has already carried, because one photograph is referred to by
// every message that quotes it and an archive of a million messages must not copy a
// picture a hundred times. That memory is also what lets the result say how many
// files travelled rather than how many times one was mentioned.
type carrier struct {
	from Files
	into string
	seen map[string]carried

	// What was actually copied, which is not the same as what was linked to: a file
	// already on disk from an earlier format or an earlier export is linked and not
	// copied. Counting the links instead would report one photograph several times
	// over, once per format, and call it several photographs.
	wrote int
	bytes int64
}

// newCarrier prepares to copy an archive's files into a directory.
func newCarrier(from Files, into string) *carrier {
	return &carrier{from: from, into: into, seen: map[string]carried{}}
}

// carried reports how many files were copied and what they came to.
func (c *carrier) carried() (files int, bytes int64) {
	if c == nil {
		return 0, 0
	}
	return c.wrote, c.bytes
}

// carry copies one file and says where it landed, or reports that it did not.
//
// Not carrying is ordinary and never an error: an archive may have no folder, a
// folder copied off a phone is routinely partial, and a path that tries to climb out
// of one is refused. In all three the page falls back to the thumbnail, which is what
// it showed before any of this existed.
func (c *carrier) carry(ref string) (carried, bool) {
	if c == nil || c.from == nil || ref == "" {
		return carried{}, false
	}
	within, ok := beneath(ref)
	if !ok {
		return carried{}, false
	}
	if already, done := c.seen[within]; done {
		return already, true
	}

	file, kind, err := c.from.OpenMedia(ref)
	if err != nil {
		return carried{}, false
	}
	defer func() { _ = file.Close() }()

	at := filepath.Join(c.into, filepath.FromSlash(within))
	out := carried{Link: linkTo(within), Kind: kind}

	// Already on disk, from an earlier export into the same folder. Copying it again
	// would be several gigabytes of work to produce the bytes that are already there.
	// A file that is there is a file that is complete: these are written to a
	// temporary name and renamed into place, so an interrupted export leaves nothing
	// half-written to mistake for the real thing.
	if there, err := os.Stat(at); err == nil && there.Mode().IsRegular() {
		out.Bytes = there.Size()
		c.seen[within] = out
		return out, true
	}

	written, err := atomicWrite(at, true, func(w io.Writer) error {
		_, err := io.Copy(w, file)
		return err
	})
	if err != nil {
		return carried{}, false
	}
	out.Bytes = written
	c.seen[within] = out
	c.wrote++
	c.bytes += written
	return out, true
}

// beneath reports whether a recorded path stays inside the folder it names, and
// returns it in the shape a link and a filename both want.
//
// The reader already refuses to open a path that climbs out, and this refuses to
// write one: the same rule on both sides, because one of them is about reading
// somebody's disk and the other is about writing to it, and a check that exists only
// at the far end is a check somebody will one day route around.
func beneath(ref string) (string, bool) {
	clean := path.Clean(strings.ReplaceAll(ref, `\`, "/"))
	switch {
	case clean == "" || clean == ".":
		return "", false
	case path.IsAbs(clean), clean == "..", strings.HasPrefix(clean, "../"):
		return "", false
	case strings.HasPrefix(clean, "~"):
		return "", false
	// A Windows drive or share, which is absolute on the machine this may be read on
	// even though it is not absolute here.
	case len(clean) > 1 && clean[1] == ':', strings.HasPrefix(clean, "//"):
		return "", false
	}
	return clean, true
}

// linkTo is a relative path as a URL: the segments escaped, the slashes kept.
//
// These names hold spaces almost without exception — `Media/WhatsApp Images/` — and
// a href with a raw space in it is a href browsers disagree about.
func linkTo(within string) string {
	parts := strings.Split(within, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// shows reports whether a kind of file is one a page can put on the screen itself,
// and as what.
//
// Unlike the viewer, nothing here is a security decision: these pages are files on
// somebody's own disk opened from their own filesystem, not documents served from an
// origin that holds a secret. It is only about which element to write, and anything
// this does not recognise is named rather than embedded, because an <img> pointing at
// a PDF is a broken picture where a sentence would have done.
func shows(kind string) string {
	if cut := strings.IndexByte(kind, ';'); cut >= 0 {
		kind = strings.TrimSpace(kind[:cut])
	}
	switch base, _, _ := strings.Cut(strings.ToLower(kind), "/"); base {
	case "image":
		return "image"
	case "video":
		return "video"
	case "audio":
		return "audio"
	default:
		return ""
	}
}
