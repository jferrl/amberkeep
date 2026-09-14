// Command amberkeep-desktop is Amberkeep in a window of its own.
//
// It is the same program as the command, opened a different way. The engine, the
// wizard, the migration and the pages are all the ones `amberkeep serve` uses; what
// this adds is a window, a dock icon, and the file pickers a browser cannot offer.
//
// It is a separate module on purpose. Wails needs cgo on macOS and Linux, and a
// program that needs cgo cannot be cross-compiled from one laptop to six platforms.
// Keeping it out of the main module means `amberkeep` itself stays a single static
// binary that builds for all of them from anywhere, which is the thing people
// download. Only this window has to be built on the machine it runs on — and even
// then only on macOS, because Wails reaches Windows through WebView2 with no cgo at
// all.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/jferrl/amberkeep/internal/api"
	"github.com/jferrl/amberkeep/internal/app"
)

// me is what somebody is called in their own archive until they say otherwise.
const me = "You"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "amberkeep: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	// The window opens on the wizard, with nothing read yet. There is no way to pass
	// it a database and no reason to want one: this is the front door for somebody
	// whose phone has died, and finding what they have is the part they need help
	// with.
	server, err := api.New(ctx, nil, api.Options{
		Location: time.Local,
		Me:       me,
		Title:    "Archive",
		Importer: app.Importer{Me: me},
		Migrator: app.Migrator{Me: me},
		Advise:   app.AdviseOn,

		// No launch secret, and nothing is weakened by its absence. The secret exists
		// because `serve` opens a port on the loopback address, where anything else
		// running on the machine could reach it. Nothing is listening here: the
		// webview calls this handler in the same process, across no socket at all,
		// so there is no port to guess and no request to forge.
		Token: "",
	})
	if err != nil {
		return err
	}
	defer func() { _ = server.Close() }()

	pick := &Picker{}

	return wails.Run(&options.App{
		Title:  "Amberkeep",
		Width:  1100,
		Height: 820,
		// Below this the wizard's own screens start to wrap badly. Nothing breaks;
		// it just stops being pleasant, and a window somebody has to fight is a
		// window they use once.
		MinWidth:  680,
		MinHeight: 560,

		// The whole program, handed over as it is. Assets is deliberately nil: with
		// it unset every GET falls through to this handler, which already serves the
		// built pages as well as the API. So the page talks to Amberkeep exactly as
		// it does in a browser — same addresses, same shapes, one contract — and
		// nothing in web/ knows which of the two it is running in.
		AssetServer: &assetserver.Options{Handler: server},

		OnStartup:  pick.opened,
		OnShutdown: func(context.Context) { _ = server.Close() },
		Bind:       []any{pick},

		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title: "Amberkeep",
				Message: "Your WhatsApp history, kept on your own computer.\n\n" +
					"Not affiliated with, endorsed by, or connected to " +
					"WhatsApp LLC or Meta Platforms, Inc.",
			},
		},
	})
}
