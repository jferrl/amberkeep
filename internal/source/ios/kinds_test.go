package ios

import (
	"testing"

	"github.com/jferrl/amberkeep/internal/model"
)

// TestKnownIsEveryCodeTheNumberSettles keeps the list the canary reports against
// honest: every code in it must have a meaning from the number alone, and no code
// outside it may have one.
func TestKnownIsEveryCodeTheNumberSettles(t *testing.T) {
	t.Parallel()

	listed := make(map[int]bool, len(Known()))
	for _, code := range Known() {
		listed[code] = true
		if kinds[code] == model.KindUnknown {
			t.Errorf("code %d is listed and means nothing", code)
		}
	}

	for code := range 256 {
		_, settled := kinds[code]
		if settled != listed[code] {
			t.Errorf("code %d: the map says %v and Known() says %v", code, settled, listed[code])
		}
	}
}
