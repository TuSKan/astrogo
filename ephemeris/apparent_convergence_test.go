package ephemeris_test

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestApparentStateHasConverged checks the loop's exit condition against the
// thing it is a proxy for.
//
// ApparentState stops when the light time stops changing by more than a
// tolerance, which replaced a flat five passes. The tolerance is stated in
// days and the quantity anybody cares about is arcseconds, so this asserts the
// translation rather than the tolerance: one more pass, done here by hand, must
// not move the answer by as much as a microarcsecond.
//
// It is written as "iterate again and see" rather than as a comparison against
// a stored five-pass answer on purpose. A stored answer pins today's provider;
// this pins the property, and keeps holding when the ephemeris behind it
// changes.
func TestApparentStateHasConverged(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	// The fastest relative motion available: the Moon settles in one pass and
	// the inner planets are where the iteration has the most to do.
	for _, body := range []struct {
		name string
		id   eph.ID
	}{
		{"Moon", eph.Moon},
		{"Sun", eph.Sun},
		{"Venus", eph.Venus},
		{"Mars", eph.Mars},
		{"Jupiter", eph.Jupiter},
		{"Neptune", eph.Neptune},
	} {
		t.Run(body.name, func(t *testing.T) {
			t.Parallel()

			var worst float64

			for d := 0; d < 365; d += 7 {
				tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC).
					AddDays(float64(d))

				settled, err := eph.ApparentState(prov, body.id, tm)
				testutil.AssertNoError(t, err)

				// One more pass, by hand: retard by the light time the settled
				// answer implies and ask again.
				tau := settled.Pos.Norm() / lightAUPerDay(t)

				again, err := prov.State(body.id, tm.AddDays(-tau))
				testutil.AssertNoError(t, err)

				moved := again.Pos.Sub(settled.Pos).Norm() / settled.Pos.Norm()
				worst = math.Max(worst, moved*180/math.Pi*3600)
			}

			t.Logf("%s: one further pass moves the answer by at most %.3g arcsec", body.name, worst)

			// A microarcsecond is four orders of magnitude below the smallest
			// thing this library claims to resolve, and six below what the
			// first pass corrects.
			const tolArcsec = 1e-6

			if worst > tolArcsec {
				t.Errorf("a further light-time pass moves %s by %.3g\", over the %.0e\" "+
					"this loop's exit condition is supposed to guarantee",
					body.name, worst, tolArcsec)
			}
		})
	}
}

// lightAUPerDay is the speed of light in the units the light-time iteration
// works in, rebuilt here from the same constants ApparentState uses so the
// test's hand-rolled pass matches the one under test.
func lightAUPerDay(t *testing.T) float64 {
	t.Helper()

	// 173.1446... AU/day. Written as the round-trip through Earth's own orbit
	// rather than as a literal: one astronomical unit takes 499.005 s of light
	// time, so this is 86400/499.005.
	const auLightSeconds = 499.00478383615643

	return 86400.0 / auLightSeconds
}
