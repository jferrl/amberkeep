package canary

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// The shapes this build was developed against.
//
// One file per database somebody has actually looked at, holding its table and
// column names and nothing else. They are what "new" is measured against: a table
// in none of them is a table this program has never seen, which is the whole signal
// this package exists to produce.
//
// They are contributed rather than invented. `amberkeep canary --emit` writes one
// from a database, and what it writes is safe to send to a stranger: names, and the
// version of WhatsApp that made them. Nobody has to trust that — the file is short
// enough to read before sending.
//
//go:embed corpus/*.json
var seen embed.FS

// Entry is one database's shape, as somebody found it.
type Entry struct {
	// Platform is "android" or "iphone".
	Platform string `json:"platform"`
	// WhatsApp is the version that wrote it, as the app reports itself. This is the
	// axis the compatibility matrix is written along, so an entry without one is of
	// little use to anybody.
	WhatsApp string `json:"whatsapp"`
	// OS is the operating system version, when it is known.
	OS string `json:"os,omitempty"`
	// Device is what kind of phone it was, when it is known. A model, never a name.
	Device string `json:"device,omitempty"`
	// Seen is the day it was looked at, so an entry can be read as evidence of a
	// moment rather than of now.
	Seen string `json:"seen"`
	// Tables is the shape itself.
	Tables map[string][]string `json:"tables"`
}

// Name is how an entry is referred to in a report.
func (e Entry) Name() string {
	name := e.Platform + " WhatsApp " + e.WhatsApp
	if e.OS != "" {
		name += " on " + e.OS
	}
	return name
}

// Corpus is every shape this build carries, in a stable order.
func Corpus() []Entry {
	names, err := seen.ReadDir("corpus")
	if err != nil {
		// Embedded at build time: a missing directory is a broken build rather than
		// something somebody could be experiencing.
		panic(fmt.Sprintf("the corpus is missing from this build: %v", err))
	}

	entries := make([]Entry, 0, len(names))
	for _, file := range names {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		raw, err := seen.ReadFile("corpus/" + file.Name())
		if err != nil {
			panic(fmt.Sprintf("the corpus entry %s is missing: %v", file.Name(), err))
		}
		var entry Entry
		if err := json.Unmarshal(raw, &entry); err != nil {
			panic(fmt.Sprintf("the corpus entry %s will not parse: %v", file.Name(), err))
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries
}

// corpusFor is every shape for one platform.
func corpusFor(platform string) []Entry {
	var out []Entry
	for _, entry := range Corpus() {
		if entry.Platform == platform {
			out = append(out, entry)
		}
	}
	return out
}

// About is what a database cannot say about itself.
//
// Which version of WhatsApp wrote a database is not written in it anywhere, and it
// is the one thing a corpus entry most needs, so it is asked for rather than
// guessed.
type About struct {
	WhatsApp string
	OS       string
	Device   string
}

// Entry turns what was found into something that can be sent to this project.
func (r Report) Entry(about About) Entry {
	return Entry{
		Platform: r.Schema.Platform,
		WhatsApp: about.WhatsApp,
		OS:       about.OS,
		Device:   about.Device,
		Seen:     time.Now().UTC().Format(time.DateOnly),
		Tables:   r.Schema.Tables,
	}
}
