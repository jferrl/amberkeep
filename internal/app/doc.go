// Package app is the work this program does, with nothing about how it was asked.
//
// It implements the seams internal/api defines — Importer and Migrator — by joining
// the packages that each know one thing: crypt15 to unlock a backup, backupfs to
// read one, source to read a message store, migrate to write one, search to index
// one. None of it reads a flag or prints a line.
//
// It lives here rather than in a command because there are now two front doors, a
// terminal and a window, and they have to do the same thing. A browser and a
// terminal disagreeing about what a migration does is the sort of difference nobody
// finds until it matters.
package app
