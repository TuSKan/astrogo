package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// TestRiseSetTransitNeedsASite: a Context with no site, one not built by
// coord.NewContext, gives no location to rise and set against, so the
// details keep no rise, set or transit, rather than computing them for a
// site that is not there.
func TestRiseSetTransitNeedsASite(t *testing.T) {
	t.Parallel()

	var d TargetDetails

	fillRiseSetTransit(&d, NewStar("Vega", angle.Deg(279.2347), angle.Deg(38.7837)), &coord.Context{})

	if d.RiseTime != nil || d.SetTime != nil || d.TransitTime != nil {
		t.Errorf("details without a site: rise %v, set %v, transit %v; want none", d.RiseTime, d.SetTime, d.TransitTime)
	}
}
