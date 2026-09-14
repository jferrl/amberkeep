package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// The menu bar, which is not decoration.
//
// Without an Edit menu a macOS webview has no standard editing selectors, so copy
// and paste do not work — and the one thing this program asks anybody to paste is a
// sixty-four character key they are reading off a phone. Typing it by hand is the
// single most error-prone step in the program, and a window that would not let them
// paste it would be worse than the browser it replaces.
//
// The rest is what a person expects to find: a way to quit, a way to hide, and the
// two things this program does, each with the shortcut it would have anywhere else.
func menus(reach func(string)) *menu.Menu {
	bar := menu.NewMenu()
	bar.Append(menu.AppMenu())

	file := bar.AddSubmenu("File")
	// These reach the page rather than doing anything here. The window and the
	// browser run the same pages, and a menu that did its own work would be a
	// second implementation of two screens that already exist.
	file.AddText("Keep a copy…", keys.CmdOrCtrl("s"), func(*menu.CallbackData) {
		reach("keep")
	})
	file.AddSeparator()
	file.AddText("Close the archive", keys.CmdOrCtrl("w"), func(*menu.CallbackData) {
		reach("close")
	})

	bar.Append(menu.EditMenu())
	bar.Append(menu.WindowMenu())
	return bar
}

// asking emits what a menu item wants the page to do.
//
// An event rather than a bound call, because this travels the other way: Go is
// telling the page something happened, and the page decides what that means on
// whichever screen it is showing.
func asking(ctx *context.Context) func(string) {
	return func(what string) {
		if *ctx == nil {
			return
		}
		runtime.EventsEmit(*ctx, "amberkeep:menu", what)
	}
}
