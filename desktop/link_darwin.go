//go:build darwin

package main

// Wails 2.16 reaches for UTType when it builds a file dialog's filters, but does not
// ask the linker for the framework that defines it. On macOS 15's SDK that is a
// link failure — "Undefined symbols: _OBJC_CLASS_$_UTType" — rather than a warning,
// so the framework is named here instead.
//
// It belongs in this module rather than in a patched dependency: it costs one line,
// it is inert on every other platform, and it disappears on its own when Wails links
// the framework itself.

// #cgo LDFLAGS: -framework UniformTypeIdentifiers
import "C"
