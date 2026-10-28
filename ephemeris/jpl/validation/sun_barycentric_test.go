//go:build validation

package jpl_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// The distance coord.SunBarycentric's doc comment states between SOFA's Sun
// and DE440's, both about their own barycenter, over this file's quarterly
// epochs from 1972 to 2100.
const (
	sunBarycenterLowKm  = 107.0
	sunBarycenterHighKm = 122.0
)

// epv00WorstKm is the most Epv00 alone could move that distance: its
// documented worst cases against DE405 over 1900-2100, 13.4 km for the
// Earth's barycentric position and 11.2 km for its heliocentric, the two
// states SunBarycentric subtracts.
const epv00WorstKm = 13.4 + 11.2

// TestSunBarycentricAgainstDE440 holds coord.SunBarycentric's documented
// distance from DE440's Sun (#875). SOFA's barycenter is not a JPL kernel's,
// and the doc comment says by how much; this keeps that figure true.
//
// Every sample must lie within the stated range, widened by what Epv00's own
// error could add. A sign or frame error in SunBarycentric moves it by the
// Sun's whole barycentric distance, about a million kilometers; a change to
// either ephemeris's barycenter moves it out of the range, and the doc
// comment has to follow.
func TestSunBarycentricAgainstDE440(t *testing.T) {
	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("jpl.NewProvider: %v", err)
	}

	defer func() { _ = p.Close() }()

	low, high := math.Inf(1), 0.0

	for _, e := range sofaEpochs() {
		st, err := p.Locate(core.Sun, e.t)
		if err != nil {
			t.Fatalf("%s: DE440's Sun: %v", e.name, err)
		}

		if st.Center != core.SolarSystemBarycenter {
			t.Fatalf("%s: DE440 locates the Sun about %v, not the barycenter", e.name, st.Center)
		}

		sofa, err := coord.SunBarycentric(e.t)
		if err != nil {
			t.Fatalf("%s: SunBarycentric: %v", e.name, err)
		}

		d := sofa.Sub(st.Pos).MulScalar(kmPerAU)
		km := d.Norm()

		low, high = math.Min(low, km), math.Max(high, km)

		if km < sunBarycenterLowKm-epv00WorstKm || km > sunBarycenterHighKm+epv00WorstKm {
			t.Errorf("%s: SOFA's Sun is %.1f km (%+.1f, %+.1f, %+.1f) from DE440's, outside the documented %.0f to %.0f km by more than Epv00's %.1f",
				e.name, km, d.X, d.Y, d.Z, sunBarycenterLowKm, sunBarycenterHighKm, epv00WorstKm)
		}
	}

	t.Logf("SOFA's Sun is %.1f to %.1f km from DE440's; documented as %.0f to %.0f",
		low, high, sunBarycenterLowKm, sunBarycenterHighKm)
}
