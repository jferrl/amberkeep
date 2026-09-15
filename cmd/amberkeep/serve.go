package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/jferrl/amberkeep/internal/api"
	"github.com/jferrl/amberkeep/internal/app"
	"github.com/jferrl/amberkeep/internal/model"
	"github.com/jferrl/amberkeep/internal/search"
	"github.com/jferrl/amberkeep/internal/source"
)

// runServe opens the archive in a browser, on this machine only.
//
// The alternative, writing the whole archive to disk first, is a fine thing to have
// and a poor way to look around: it costs a few hundred megabytes and several
// minutes before the first conversation can be read. This costs a second.
//
// Without --db it starts at the beginning instead, with the page that finds a
// backup and brings the messages out of it. That is the case this program exists
// for: somebody whose phone has died does not have a database to point at, and
// getting from what they do have to one is the part they need help with.
func runServe(ctx context.Context, args []string) error {
	fs := newFlagSet("serve", "bring an archive in, and read it in a browser on this machine")
	var (
		db       = fs.String("db", "", "the decrypted message database (default: start with the page that finds one)")
		work     = fs.String("workspace", "", "where to write anything brought out of a backup (default: ~/Amberkeep)")
		indexAt  = fs.String("index", "", "where to keep the search index (default: beside the database)")
		bookPath = fs.String("contacts", "", "an address book, so conversations show names instead of numbers")
		waPath   = fs.String("whatsapp-contacts", "", "WhatsApp's own contacts database, usually wa.db")
		country  = fs.String("country", "", "dialling code for numbers saved without one, such as 34")
		zone     = fs.String("timezone", "", "time zone for timestamps (default: this machine's)")
		me       = fs.String("me", "You", "what to call yourself")
		port     = fs.Int("port", 0, "port to listen on (default: one the system picks)")
		noSearch = fs.Bool("no-search", false, "skip the search index, which takes minutes to build the first time")
		noOpen   = fs.Bool("no-open", false, "do not open a browser")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	location, err := parseZone(*zone)
	if err != nil {
		return err
	}

	bring := app.Importer{Me: *me, Country: *country, NoSearch: *noSearch}
	move := app.Migrator{Me: *me, Country: *country}

	// Without a database there is nothing to open yet, and the page starts at the
	// beginning. Everything below this point is the same either way.
	var (
		reader source.Archive
		names  *model.Directory
		index  *search.Index
	)
	if *db != "" {
		if reader, err = source.Open(ctx, *db); err != nil {
			return err
		}
		if names, err = app.LoadNames(ctx, reader.Directory(), *bookPath, *waPath, *country); err != nil {
			return err
		}
		if !*noSearch {
			index, err = app.OpenIndex(ctx, *db, app.IndexPath(*db, *indexAt), app.IndexSettings{
				Me: *me, Book: *bookPath, WhatsApp: *waPath, Country: *country,
			})
			if err != nil {
				return err
			}
			defer func() { _ = index.Close() }()
		}
	}

	token, err := secret()
	if err != nil {
		return err
	}

	// A nil reader has to arrive as a nil interface rather than as an interface
	// holding a nil pointer, which would be a non-nil value that panics on first
	// use. This is the one place the distinction matters.
	var archive api.Archive
	if reader != nil {
		// Wrapped rather than handed over raw, so an archive opened here is the same
		// object as one the wizard opens — including being able to write itself out.
		archive = app.Readable(reader, index)
	}

	// From here the server owns the archive and is what closes it. Until here it is
	// this function's, which matters only on the paths that fail before handing it
	// over.
	handler, err := api.New(ctx, archive, api.Options{
		Names:     names,
		Location:  location,
		Me:        *me,
		Token:     token,
		Index:     index,
		Importer:  bring,
		Phones:    app.Phones{Workspace: *work},
		Migrator:  move,
		Workspace: *work,
		Advise:    app.GuidanceFor,
	})
	if err != nil {
		if reader != nil {
			_ = reader.Close()
		}
		return err
	}
	defer func() { _ = handler.Close() }()

	// Loopback only, and said explicitly rather than left to the default: an
	// archive reachable from the network is somebody's whole history reachable from
	// the network.
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return fmt.Errorf("could not start the viewer: %w", err)
	}

	address := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", portOf(listener), token)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if *db == "" {
		fmt.Printf("open %s to bring your messages in\n", address)
	} else {
		fmt.Printf("reading the archive at %s\n", address)
	}
	fmt.Printf("\nThis address works in this browser only, and only from this computer.\n")
	fmt.Printf("Press control-C to stop.\n")

	if !*noOpen {
		openBrowser(ctx, address)
	}

	// Serving stops when the command does, and anything still being read is given a
	// moment to finish rather than cut off mid-response.
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		// Anything still being read is given a moment to finish rather than cut off
		// mid-response. A failure to stop tidily is not worth reporting: the process
		// is ending either way.
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			fmt.Fprintf(os.Stderr, "the viewer did not stop tidily: %v\n", err)
		}
		fmt.Println("\nstopped.")
		return nil
	}
}

// portOf is the port the viewer actually got, which is the one the system picked
// unless the caller asked for a particular one.
func portOf(listener net.Listener) int {
	if address, ok := listener.Addr().(*net.TCPAddr); ok {
		return address.Port
	}
	return 0
}

// secret makes the single-use password for this launch.
//
// It is what keeps another program on the same computer from reading somebody's
// messages by trying ports, which is otherwise all that stands between them.
func secret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("could not make a secret for this session: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// openBrowser asks the desktop to open the viewer.
//
// A failure here is not a failure of the command: the address is already printed,
// and somebody can paste it themselves.
func openBrowser(ctx context.Context, address string) {
	var argv []string
	switch runtime.GOOS {
	case "darwin":
		argv = []string{"open", address}
	case "windows":
		argv = []string{"rundll32", "url.dll,FileProtocolHandler", address}
	default:
		argv = []string{"xdg-open", address}
	}

	// The context is deliberately one that is never cancelled. Control-C here should
	// stop serving the archive, not close the window somebody is reading it in.
	//
	// #nosec G204 -- the address is this function's own, built from a port the
	// system assigned and a secret this program generated. Nothing a user typed or
	// a file contained reaches it.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "could not open a browser (%v); open the address above yourself\n", err)
		return
	}
	// Waited on in the background so the browser process is not left behind as a
	// zombie. Whether it exited well is the desktop's business, not this program's.
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "the browser reported: %v\n", err)
		}
	}()
}
