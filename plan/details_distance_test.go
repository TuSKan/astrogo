package plan

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestStringOmitsAnUnknownDistance: a target nothing gave a distance has a
// zero Distance, which String printed as "Distance: 0.0000 pc", a
// measurement nobody made (#1132). M31 from OpenNGC and a star without a
// parallax both did. The line is now left out, as Magnitude and
// RadialVelocity are when unknown, and kept for a star with a parallax.
func TestStringOmitsAnUnknownDistance(t *testing.T) {
	loc, err := coord.NewEarthLocation(-23.5505, -46.6333, 760)
	testutil.AssertNoError(t, err)

	tm := time.Date(2026, time.April, 25, 20, 0, 0, 0, time.LocationUTC)
	ctx := coord.NewContext(tm, loc, atmosphere.Refraction{})

	ra, dec := angle.Deg(101.287155), angle.Deg(-16.716116)

	for _, c := range []struct {
		name     string
		target   *Star
		wantLine bool
	}{
		{"no parallax", NewStar("Sirius", ra, dec), false},
		{"parallax", NewStar("Sirius", ra, dec, WithParallax(angle.Arcsec(0.37921))), true},
	} {
		d, err := c.target.GetDetails(ctx, DetailOverrides{})
		testutil.AssertNoError(t, err)

		s := d.String()
		if got := strings.Contains(s, "Distance:"); got != c.wantLine {
			t.Errorf("%s: Distance = %v, line printed = %v, want %v:\n%s", c.name, d.Distance, got, c.wantLine, s)
		}

		if strings.Contains(s, "0.0000 pc") {
			t.Errorf("%s: String() reports a zero distance:\n%s", c.name, s)
		}
	}
}
