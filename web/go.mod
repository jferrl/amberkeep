// This directory holds a frontend, not a Go package, and this file exists only to
// keep the Go tool out of it.
//
// Without it, "go build ./..." walks node_modules, where npm dependencies sometimes
// ship Go packages of their own; one of them is in this tree today. Those would then
// be built, tested and linted along with this project, which is both noise and a way
// for somebody else's code to break this build. A nested module is excluded from the
// parent's package patterns, which is exactly what is wanted.
//
// Nothing imports this module and nothing ever should.
module github.com/jferrl/amberkeep/web

go 1.27
