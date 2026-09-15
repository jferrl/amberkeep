package thanks

import "testing"

// TestNothingIsShownWithoutSomewhereToPointAt is the whole contract: the line is
// worth showing once, at a good moment, and only when it leads somewhere.
func TestNothingIsShownWithoutSomewhereToPointAt(t *testing.T) {
	t.Parallel()

	if Asked() != (Address != "") {
		t.Errorf("Asked() = %v with an address of %q", Asked(), Address)
	}
}
