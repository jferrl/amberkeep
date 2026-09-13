package backupfs

import (
	"fmt"
	"math"

	"howett.net/plist"
)

// mbfile is the metadata carried by the "file" BLOB in Manifest.db's Files table: an
// NSKeyedArchiver binary plist archiving a serialized MBFile object. A real MBFile
// also carries Mode, UserID, GroupID, Birth and last-modified times and, for an
// encrypted backup, a wrapped per-file encryption key; only Size is read here because
// it is the only field this package's API needs, and reading the rest into a struct
// nothing uses would just be more surface for a parser that already has to treat its
// input as hostile.
type mbfile struct {
	Size int64
}

// parseMBFile decodes one "file" BLOB.
//
// The blob comes straight out of Manifest.db, which this package treats as untrusted
// input: every step here is defensive, and an unexpected shape is reported as an
// error rather than as an index panic, a wrong-type panic or a silently wrong Size.
// See FuzzParseMBFile.
func parseMBFile(data []byte) (mbfile, error) {
	// An NSKeyedArchiver plist is a flat object graph: "$top" names the entry point,
	// "$objects" is the graph's storage, and every reference between objects is a
	// plist.UID index into it. howett.net/plist decodes that shape into ordinary Go
	// values (map[string]any, []any, plist.UID) without knowing anything about
	// NSKeyedArchiver's conventions, so this function does the graph walk by hand.
	var root struct {
		Top     map[string]any `plist:"$top"`
		Objects []any          `plist:"$objects"`
	}
	if _, err := plist.Unmarshal(data, &root); err != nil {
		return mbfile{}, fmt.Errorf("decoding the keyed archive: %w", err)
	}

	ref, ok := root.Top["root"]
	if !ok {
		return mbfile{}, fmt.Errorf("keyed archive has no $top.root reference")
	}
	obj, err := resolveArchiveRef(root.Objects, ref)
	if err != nil {
		return mbfile{}, err
	}

	dict, ok := obj.(map[string]any)
	if !ok {
		return mbfile{}, fmt.Errorf("archived MBFile root is a %T, not a dictionary", obj)
	}

	size, err := archiveInt(dict, "Size")
	if err != nil {
		return mbfile{}, err
	}
	return mbfile{Size: size}, nil
}

// resolveArchiveRef follows one NSKeyedArchiver reference into $objects. A reference
// is always a plist.UID; anything else, or an index outside the array, means the
// input is not the shape this package expects.
func resolveArchiveRef(objects []any, ref any) (any, error) {
	uid, ok := ref.(plist.UID)
	if !ok {
		return nil, fmt.Errorf("archive reference is a %T, not a UID", ref)
	}
	i := uint64(uid)
	if i >= uint64(len(objects)) {
		return nil, fmt.Errorf("archive reference %d is outside $objects (length %d)", i, len(objects))
	}
	return objects[i], nil
}

// archiveInt reads one integer field defensively. howett.net/plist's interface{}
// decoding produces uint64 for a plist integer that was archived unsigned and int64
// for one archived signed (see its Unmarshal doc); a float64 is accepted too, since a
// hand-built or non-Apple-Cocoa producer of this format is not guaranteed to pick the
// same representation Apple's own writer does.
func archiveInt(dict map[string]any, key string) (int64, error) {
	v, ok := dict[key]
	if !ok {
		return 0, fmt.Errorf("archived MBFile has no %q field", key)
	}
	switch n := v.(type) {
	case uint64:
		// A real file's Size never approaches this, but the input is untrusted, so the
		// conversion is checked rather than silently wrapping into a negative number.
		if n > math.MaxInt64 {
			return 0, fmt.Errorf("%q field %d overflows a signed 64-bit size", key, n)
		}
		return int64(n), nil
	case int64:
		return n, nil
	case float64:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("%q field is a %T, not a number", key, v)
	}
}
