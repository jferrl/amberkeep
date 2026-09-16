package api

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// Filed is an archive whose messages' files are somewhere this program can read.
//
// A separate interface rather than more of Archive, for the same reason Exportable
// is separate: an archive with no files beside it is still a perfectly good archive
// to read, and most of them have none. A reader that cannot do this simply is not
// asked.
type Filed interface {
	// OpenMedia returns one file and what kind of thing it is. The reference is
	// whatever the reader put on the attachment, and the reader is the only thing
	// that decides where such a reference may lead.
	OpenMedia(ref string) (io.ReadSeekCloser, string, error)
}

// handleMedia serves one file an attachment referred to.
//
// # What is served inline, and why most things are not
//
// These bytes came off somebody's phone and are served from the same address as the
// program's own page, which is the address the launch secret is scoped to. A file
// served as text/html or as an SVG is a script running with everything that page can
// reach, and "it came from your own phone" is not an argument — the whole point of a
// message archive is that other people sent things to it.
//
// So a small list of kinds is served as what they are, because a picture has to
// arrive as a picture to be shown as one, and everything else is served as bytes to
// be saved. A photograph is a photograph; anything this program is not certain about
// is a download.
func (s *server) handleMedia(w http.ResponseWriter, r *http.Request) {
	open, ok := s.reading(w)
	if !ok {
		return
	}
	files, can := open.archive.(Filed)
	if !can {
		http.Error(w, "this archive has no files with it", http.StatusNotFound)
		return
	}

	ref := r.URL.Query().Get("ref")
	if ref == "" {
		http.Error(w, "which file is needed", http.StatusBadRequest)
		return
	}

	file, kind, err := files.OpenMedia(ref)
	if err != nil {
		// Not there and not allowed are the same answer on purpose: a request that
		// tried to climb out of the folder learns nothing from the reply.
		http.Error(w, "that file is not in this archive", http.StatusNotFound)
		return
	}
	defer func() { _ = file.Close() }()

	if shown(kind) {
		w.Header().Set("Content-Type", kind)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment")
	}
	// The browser is never to second-guess what was just decided.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// It came from a file on this computer and this program cannot write to it, so
	// a page may keep it for as long as it is open. Private: there is no shared
	// cache between here and the page, and there is not going to be one.
	w.Header().Set("Cache-Control", "private, max-age=3600")

	// ServeContent rather than a copy: it answers range requests, which is what a
	// video somebody drags the middle of actually sends.
	http.ServeContent(w, r, ref, time.Time{}, file)
}

// shown reports whether a kind of file may be served as itself.
//
// An allowlist rather than a list of dangerous types, because the dangerous list is
// the one that gets out of date: text/html and image/svg+xml are the two everybody
// remembers, and the next one will be something nobody has thought of yet.
func shown(kind string) bool {
	if cut := strings.IndexByte(kind, ';'); cut >= 0 {
		kind = strings.TrimSpace(kind[:cut])
	}
	switch strings.ToLower(kind) {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/heic", "image/heif",
		"video/mp4", "video/quicktime", "video/3gpp", "video/webm",
		"audio/mpeg", "audio/mp4", "audio/aac", "audio/ogg", "audio/opus", "audio/wav",
		"audio/x-wav", "audio/amr", "audio/3gpp":
		return true
	default:
		// Everything else, including image/svg+xml, which is a document that can
		// carry script rather than a picture.
		return false
	}
}
