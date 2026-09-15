// Command licencer issues Amberkeep licences.
//
// The seller's side of internal/licence, and the only thing in this repository that
// touches a private key. It is here rather than hidden because hiding it would
// protect nothing: what cannot be forged is a signature, and the secret is the key
// rather than the method. It is not part of the released program — the release builds
// ./cmd/amberkeep and nothing else — so a licence cannot be issued by a copy of the
// thing a licence unlocks.
//
//	licencer keys --into ~/.amberkeep-signing     # once, and keep the file
//	licencer issue --key ~/.amberkeep-signing --grants archive,migration --order pdl_0001
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/licence"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "licencer: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("what to do is needed: keys, or issue")
	}

	switch args[0] {
	case "keys":
		return makeKeys(args[1:])
	case "issue":
		return issue(args[1:])
	default:
		return fmt.Errorf("there is no %q to do here: keys, or issue", args[0])
	}
}

// makeKeys writes a signing key, once, and prints the half that goes into the
// program.
func makeKeys(args []string) error {
	fs := flag.NewFlagSet("keys", flag.ContinueOnError)
	into := fs.String("into", "", "where to write the signing key. It never goes in the repository")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *into == "" {
		return errors.New("--into is needed: somewhere outside this repository")
	}

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("making a signing key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(*into), 0o700); err != nil {
		return fmt.Errorf("making somewhere to keep it: %w", err)
	}
	signing := base64.StdEncoding.EncodeToString(private)
	if err := os.WriteFile(*into, []byte(signing+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing the signing key: %w", err)
	}

	fmt.Printf("the signing key is in %s, readable by nobody else. Back it up:\n"+
		"a lost one means every licence already sold has to be reissued.\n\n"+
		"The public half goes into internal/licence/licence.go:\n\n"+
		"  var verifier = ed25519.PublicKey{%s}\n\n", *into, bytesAsGo(public))
	return nil
}

// issue writes one licence.
func issue(args []string) error {
	fs := flag.NewFlagSet("issue", flag.ContinueOnError)
	var (
		key    = fs.String("key", "", "the signing key made by `licencer keys`")
		grants = fs.String("grants", "archive", "what it covers: archive, migration, or both")
		order  = fs.String("order", "", "the seller's reference, for reissuing a lost key")
		months = fs.Int("updates", 12, "months of updates")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *key == "" {
		return errors.New("--key is needed")
	}

	// #nosec G304 -- the path is the seller's own key, named by the seller.
	raw, err := os.ReadFile(*key)
	if err != nil {
		return fmt.Errorf("reading the signing key: %w", err)
	}
	signing, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(signing) != ed25519.PrivateKeySize {
		return errors.New("that file is not a signing key")
	}

	var covers []licence.Grant
	for _, grant := range strings.Split(*grants, ",") {
		switch strings.TrimSpace(grant) {
		case string(licence.Archive):
			covers = append(covers, licence.Archive)
		case string(licence.Migration):
			covers = append(covers, licence.Migration)
		case "":
		default:
			return fmt.Errorf("nobody sells %q: archive, migration, or both", grant)
		}
	}
	if len(covers) == 0 {
		return errors.New("a licence covering nothing is not one")
	}

	today := time.Now().UTC()
	written, err := licence.Write(licence.Licence{
		Grants:  covers,
		Issued:  today.Format(time.DateOnly),
		Updates: today.AddDate(0, *months, 0).Format(time.DateOnly),
		Order:   *order,
	}, signing)
	if err != nil {
		return err
	}
	fmt.Println(written)
	return nil
}

// bytesAsGo writes a key as Go source, for pasting into the program.
func bytesAsGo(key []byte) string {
	var b strings.Builder
	for i, c := range key {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d", c)
	}
	return b.String()
}
