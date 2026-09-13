// Package viewer holds the built browser viewer, compiled into the binary.
//
// The source of it is the React application in web/ at the top of the repository,
// which builds into the dist directory beside this file. It builds to here rather
// than to beside its own source so that Go never has to look inside web/: npm
// dependencies occasionally contain Go packages of their own, and this module has no
// business building, testing or linting those. The built files are committed on purpose, so that anybody
// can build this program with nothing but a Go toolchain. Requiring Node.js to
// compile an archive reader would be a strange thing to ask of somebody who only
// wants to read their own messages.
//
// Everything the page needs is in here. No stylesheet, script, font or image is
// fetched from anywhere, which is what lets the viewer work with the network
// switched off and is the reason a page holding somebody's entire history can be
// trusted at all.
package viewer

import (
	"embed"
	"io/fs"
)

// built is the output of the frontend build.
//
// The all: prefix matters: without it embed skips files whose names begin with an
// underscore or a dot, and a build tool is entitled to produce those.
//
//go:embed all:dist
var built embed.FS

// Assets returns the viewer's files, rooted so that "index.html" is at the top.
//
// A caller that cannot find index.html has a binary built without the frontend,
// which is a build mistake rather than a runtime condition, so the error says so.
func Assets() (fs.FS, error) {
	return fs.Sub(built, "dist")
}
