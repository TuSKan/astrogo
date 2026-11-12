package kepler

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestProvidersShareOneSOFABaseAndNothingElse: every Provider built without a
// base of its own shares one SOFA base, and with it the memo of the last Epv00
// evaluation, so a provider built for one check finds the instant the previous
// check evaluated (#913). What a caller registers stays on its own Provider.
func TestProvidersShareOneSOFABaseAndNothingElse(t *testing.T) {
	a, b := New(), New()

	if a.base != sharedSOFA || b.base != sharedSOFA {
		t.Fatalf("two Providers built with no base hold %p and %p, not the shared base %p", a.base, b.base, sharedSOFA)
	}

	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	el, err := NewElements(epoch, unit.AU(2.77), 0.1, angle.Zero(), angle.Zero(), angle.Zero(), angle.Zero())
	if err != nil {
		t.Fatal(err)
	}

	const id = core.ID(2000001)

	if err := a.Register(id, el); err != nil {
		t.Fatal(err)
	}

	if _, ok := b.elements[id]; ok {
		t.Error("a body registered on one Provider appeared on another: the base is shared, the elements must not be")
	}
}
