package backupfs

import (
	"testing"

	"howett.net/plist"
)

// TestParseMBFile protects the keyed-archiver walk in mbfile.go against every shape
// of untrusted input it has to tolerate: a well-formed archive in both the numeric
// representations howett.net/plist can hand back, and every way the graph can be
// malformed short of a truncated byte stream (which FuzzParseMBFile covers instead).
func TestParseMBFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     func(t *testing.T) []byte
		wantSize int64
		wantErr  bool
	}{
		{
			name:     "a well-formed archive yields its Size",
			data:     func(t *testing.T) []byte { return buildMBFileArchive(t, 12345) },
			wantSize: 12345,
		},
		{
			name: "Size archived as a signed integer is still read",
			data: func(t *testing.T) []byte {
				return buildKeyedArchive(t, map[string]any{"Size": int64(9999)})
			},
			wantSize: 9999,
		},
		{
			name: "Size archived as a real number is still read",
			data: func(t *testing.T) []byte {
				return buildKeyedArchive(t, map[string]any{"Size": float64(42)})
			},
			wantSize: 42,
		},
		{
			name:    "not a plist at all",
			data:    func(t *testing.T) []byte { return []byte("definitely not a property list") },
			wantErr: true,
		},
		{
			name:    "empty input",
			data:    func(t *testing.T) []byte { return nil },
			wantErr: true,
		},
		{
			name: "a plist with no $top",
			data: func(t *testing.T) []byte {
				return marshalOrFatal(t, map[string]any{"$objects": []any{"$null"}})
			},
			wantErr: true,
		},
		{
			name: "$top has no root key",
			data: func(t *testing.T) []byte {
				return marshalOrFatal(t, map[string]any{
					"$top":     map[string]any{"notroot": plist.UID(1)},
					"$objects": []any{"$null", map[string]any{"Size": uint64(1)}},
				})
			},
			wantErr: true,
		},
		{
			name: "$top.root is not a UID",
			data: func(t *testing.T) []byte {
				return marshalOrFatal(t, map[string]any{
					"$top":     map[string]any{"root": "not a uid"},
					"$objects": []any{"$null", map[string]any{"Size": uint64(1)}},
				})
			},
			wantErr: true,
		},
		{
			name: "$top.root references an index outside $objects",
			data: func(t *testing.T) []byte {
				return marshalOrFatal(t, map[string]any{
					"$top":     map[string]any{"root": plist.UID(99)},
					"$objects": []any{"$null"},
				})
			},
			wantErr: true,
		},
		{
			name: "the root object is not a dictionary",
			data: func(t *testing.T) []byte {
				return marshalOrFatal(t, map[string]any{
					"$top":     map[string]any{"root": plist.UID(1)},
					"$objects": []any{"$null", "just a string, not an MBFile dictionary"},
				})
			},
			wantErr: true,
		},
		{
			name: "the root dictionary has no Size field",
			data: func(t *testing.T) []byte {
				return buildKeyedArchive(t, map[string]any{"Mode": uint64(0o644)})
			},
			wantErr: true,
		},
		{
			name: "Size is the wrong type",
			data: func(t *testing.T) []byte {
				return buildKeyedArchive(t, map[string]any{"Size": "twelve"})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseMBFile(tt.data(t))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseMBFile() = %+v, nil, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMBFile() unexpected error: %v", err)
			}
			if got.Size != tt.wantSize {
				t.Errorf("Size = %d, want %d", got.Size, tt.wantSize)
			}
		})
	}
}

func marshalOrFatal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := plist.Marshal(v, plist.BinaryFormat)
	if err != nil {
		t.Fatalf("marshalling a synthetic plist: %v", err)
	}
	return data
}

// FuzzParseMBFile is the fuzz test the principles require for any parser reading
// untrusted bytes. Manifest.db's "file" column is exactly that: bytes controlled by
// whatever produced the backup, not by this package. The only contract under fuzzing
// is that parseMBFile never panics and never returns a non-zero result alongside an
// error.
func FuzzParseMBFile(f *testing.F) {
	f.Add(buildMBFileArchive(f, 42))
	f.Add(buildMBFileArchive(f, 0))
	f.Add(buildKeyedArchive(f, map[string]any{"Size": int64(-1)}))
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte("bplist00"))
	f.Add([]byte("bplist00\x00\x00\x00\x00"))

	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := parseMBFile(data)
		if err != nil {
			if got != (mbfile{}) {
				t.Errorf("parseMBFile(%q) returned both a non-zero result %+v and an error: %v", data, got, err)
			}
			return
		}
	})
}
