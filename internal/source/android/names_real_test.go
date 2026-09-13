package android

import (
	"context"
	"os"
	"testing"

	"github.com/jferrl/amberkeep/internal/contacts"
	"github.com/jferrl/amberkeep/internal/model"
)

// TestAddressBookNamesTheRealArchive measures how much of a real archive stops
// reading as phone numbers once the phone's own address book is loaded.
func TestAddressBookNamesTheRealArchive(t *testing.T) {
	db := os.Getenv("AMBERKEEP_REAL_MSGSTORE")
	book := os.Getenv("AMBERKEEP_REAL_CONTACTS")
	if db == "" || book == "" {
		t.Skip("set AMBERKEEP_REAL_MSGSTORE and AMBERKEEP_REAL_CONTACTS to run this")
	}

	ctx := context.Background()
	r, err := Open(ctx, db)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer r.Close()

	chats, err := r.Chats(ctx)
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}

	countNamed := func() (saved, anyName, total int) {
		for _, c := range chats {
			if c.Kind != model.ChatDirect || !c.Includable() {
				continue
			}
			total++
			n := r.Directory().Lookup(c.JID)
			if n.Name != "" {
				saved++
			}
			if n.IsIdentified() {
				anyName++
			}
		}
		return saved, anyName, total
	}

	beforeNamed, beforeAny, total := countNamed()
	beforeIdentified := r.Directory().Identified()

	b := contacts.New("34")
	read, err := b.ReadVCardFile(book)
	if err != nil {
		t.Fatalf("ReadVCardFile() failed: %v", err)
	}
	applied := b.ApplyTo(r.Directory())

	afterNamed, afterAny, _ := countNamed()
	t.Logf("address book: %d numbers read, %d applied to this archive", read, applied)
	t.Logf("direct conversations with a saved name: %d -> %d of %d", beforeNamed, afterNamed, total)
	t.Logf("direct conversations with any name at all: %d -> %d of %d (%.0f%%)",
		beforeAny, afterAny, total, float64(afterAny)*100/float64(total))
	t.Logf("people with any human name: %d -> %d of %d known",
		beforeIdentified, r.Directory().Identified(), r.Directory().Len())

	if afterNamed <= beforeNamed {
		t.Errorf("the address book named nothing new")
	}
}
