package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/licence"
)

// runLicence shows what this computer has been given, and takes a key.
//
// It exists before there is anywhere to buy one, which is deliberate: the machinery
// has to be real and tested before a shop can rely on it, and a person who has one
// from somewhere else has to be able to see what it says. While nothing is for sale
// the command says so, because a licence screen in a program that asks for no licence
// is otherwise a puzzle.
func runLicence(ctx context.Context, args []string) error {
	_ = ctx

	fs := newFlagSet("licence", "show the licence on this computer, or put one here")
	var (
		add    = fs.String("add", "", "a licence key to keep on this computer")
		remove = fs.Bool("remove", false, "forget the licence on this computer")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	switch {
	case *remove:
		if err := licence.Forget(); err != nil {
			return err
		}
		fmt.Println("the licence on this computer has been forgotten.")
		return nil

	case *add != "":
		held, err := licence.Keep(*add)
		if err != nil {
			return err
		}
		fmt.Printf("kept. %s\n", describe(held))
		fmt.Printf("it is in %s\n", licence.Where())
		return nil

	default:
		return showLicence()
	}
}

// showLicence says what is here.
func showLicence() error {
	held, err := licence.Held()
	switch {
	case errors.Is(err, licence.ErrNoLicence):
		fmt.Printf("there is no licence on this computer.\n\nOne would go in %s\n",
			licence.Where())
	case err != nil:
		return err
	default:
		fmt.Printf("%s\nit is in %s\n", describe(held), licence.Where())
	}

	if !licence.Sells() {
		fmt.Print("\nNothing is for sale yet, and nothing in this program asks for a licence:\n" +
			"reading, searching, exporting and migrating are all free in this build.\n" +
			"What will cost what, when there is somewhere to buy it, is written down in\n" +
			"docs/PRICING_AND_LICENSING.md.\n")
	}
	return nil
}

// describe says what a licence covers in a sentence.
func describe(held licence.Licence) string {
	what := make([]string, 0, len(held.Grants))
	for _, grant := range held.Grants {
		switch grant {
		case licence.Archive:
			what = append(what, "writing the archive out")
		case licence.Migration:
			what = append(what, "moving a history onto an iPhone")
		default:
			what = append(what, string(grant))
		}
	}
	if len(what) == 0 {
		return "this licence covers nothing, which is not a licence anybody sold."
	}

	said := "this licence covers " + join(what)
	if held.Issued != "" {
		said += ", bought on " + held.Issued
	}
	if held.Updates != "" {
		said += ", with updates until " + held.Updates
		if built := builtOn(); !held.Current(built) {
			said += " — this build is newer than that, so it is not covered by it. " +
				"The build you had goes on working."
		}
	}
	return said + "."
}

// builtOn is the day this build was made, when it says.
//
// A release carries its version rather than a date, so this is empty for now and a
// licence's updates are not enforced against anything. It is a function rather than
// nothing so that the rule is written where it belongs, and the day a build stamps
// itself there is one place to change.
func builtOn() time.Time { return time.Time{} }

func join(what []string) string {
	switch len(what) {
	case 0:
		return ""
	case 1:
		return what[0]
	default:
		return strings.Join(what[:len(what)-1], ", ") + " and " + what[len(what)-1]
	}
}
