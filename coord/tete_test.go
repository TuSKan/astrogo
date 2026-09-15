package coord_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// teteEpoch is fixed so the equation of the origins is a fixed number: it
// grows at about 46 arcseconds a year, so a test run against "now" would be
// comparing against a moving target.
var teteEpoch = time.Date(2026, 4, 15, 22, 0, 0, 0, time.LocationUTC)

func teteContext(t *testing.T) *coord.Context {
	t.Helper()

	return coord.NewContext(
		teteEpoch,
		coord.MustGeodetic(angle.Deg(-70.40417), angle.Deg(-24.62722), 2635),
		atmosphere.Refraction{},
	)
}

// TestEquationOfOriginsIsEraMinusGST is the external check, and it uses SOFA's
// own definition of the quantity rather than a second call to the routine that
// produced it.
//
// The equation of the origins is defined as ERA − GST: the difference between
// the Earth rotation angle, measured from the Celestial Intermediate Origin,
// and apparent sidereal time, measured from the true equinox. astrogo can
// produce both sides independently — [time.Time.GAST] for one and SOFA's Era00
// for the other — so the shift this conversion applies can be checked against
// the definition instead of against the implementation.
func TestEquationOfOriginsIsEraMinusGST(t *testing.T) {
	t.Parallel()

	ctx := teteContext(t)

	// The shift the conversion actually applies, recovered from it.
	place := coord.NewApparent(angle.Deg(100), angle.Deg(20))
	applied := place.RA().Sub(ctx.ApparentToTETE(place).RA()).Wrap180()

	// The same quantity from its definition.
	eop := ctx.Time().EOP()
	ut1, ut2 := ctx.Time().UTC().JDParts()
	ut2 += eop.DUT1 / 86400.0

	era := angle.Rad(gofaext.Era00(ut1, ut2))

	gst, err := ctx.Time().GAST()
	if err != nil {
		t.Fatalf("GAST: %v", err)
	}

	want := era.Sub(gst).Wrap180()

	t.Logf("equation of the origins: applied %v, ERA-GST %v (%.3f arcmin)",
		applied, want, applied.Arcseconds()/60)

	// A microarcsecond. The two sides run through different SOFA assemblies —
	// one via Apco13, one via Gst06a — so they agree to their shared model and
	// not to the bit.
	if diff := math.Abs(applied.Sub(want).Wrap180().Arcseconds()); diff > 1e-6 {
		t.Errorf("the conversion shifted RA by %v, but ERA-GST is %v (differ by %g arcsec)",
			applied, want, diff)
	}
}

// TestTETEDiffersFromCIRSByTwentyArcminutes states the size of the trap.
//
// "Apparent" names both systems in common use, and a caller who reads a CIRS
// right ascension as an almanac's apparent one is not slightly wrong. The
// equation of the origins is the precession in right ascension accumulated
// since J2000.0, so it grows without bound: a fifth of a degree by 2026, and
// half a degree by 2040.
func TestTETEDiffersFromCIRSByTwentyArcminutes(t *testing.T) {
	t.Parallel()

	ctx := teteContext(t)

	place := coord.NewApparent(angle.Deg(83.8), angle.Deg(-5.4))
	tete := ctx.ApparentToTETE(place)

	shift := tete.RA().Sub(place.RA()).Wrap180()

	t.Logf("CIRS RA %v -> TETE RA %v, a shift of %.3f arcmin",
		place.RA().HMSString(3), tete.RA().HMSString(3), shift.Arcseconds()/60)

	// Between ten arcminutes and a degree: comfortably wide, and it excludes
	// both "no shift at all" and anything that would mean the wrong quantity
	// was applied.
	if arcmin := math.Abs(shift.Arcseconds() / 60); arcmin < 10 || arcmin > 60 {
		t.Errorf("CIRS and TETE right ascensions differ by %.3f arcmin at %v, "+
			"want the tens of arcminutes the equation of the origins is",
			arcmin, teteEpoch)
	}

	// The equator is shared, so declination must not move at all.
	if tete.Dec() != place.Dec() {
		t.Errorf("declination changed from %v to %v; the two systems share an "+
			"equator and differ only in the origin of right ascension",
			place.Dec(), tete.Dec())
	}
}

// TestTETERoundTripsThroughCIRS covers the inverse.
func TestTETERoundTripsThroughCIRS(t *testing.T) {
	t.Parallel()

	ctx := teteContext(t)

	for _, tc := range []struct {
		name    string
		ra, dec float64
	}{
		{"an ordinary place", 83.8, -5.4},
		{"just above the RA wrap", 0.05, 12},
		{"just below it", 359.95, -12},
		{"the north celestial pole", 0, 89.9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := coord.NewApparent(angle.Deg(tc.ra), angle.Deg(tc.dec))
			back := ctx.TETEToApparent(ctx.ApparentToTETE(src))

			// Compared as directions: a place near the wrap comes back on the
			// other side of it and is the same place.
			if diff := math.Abs(back.RA().Sub(src.RA()).Wrap180().Arcseconds()); diff > 1e-9 {
				t.Errorf("RA round trip: %v -> %v (differ by %g arcsec)",
					src.RA(), back.RA(), diff)
			}

			if back.Dec() != src.Dec() {
				t.Errorf("Dec round trip: %v -> %v", src.Dec(), back.Dec())
			}
		})
	}
}

// TestTETEStaysInRange keeps the wrap honest: subtracting twenty arcminutes
// from a right ascension just above zero has to land just below 360, not at a
// negative angle that formats as "-00h20m".
func TestTETEStaysInRange(t *testing.T) {
	t.Parallel()

	ctx := teteContext(t)

	for _, raDeg := range []float64{0, 0.01, 0.2, 359.99, 180} {
		got := ctx.ApparentToTETE(coord.NewApparent(angle.Deg(raDeg), 0)).RA().Degrees()

		if got < 0 || got >= 360 {
			t.Errorf("CIRS RA %g deg converted to %g deg, want [0, 360)", raDeg, got)
		}
	}
}

// TestTETENamesItsSystem keeps the two apart in a log line, since the numbers
// alone do not say which origin the right ascension is measured from.
func TestTETENamesItsSystem(t *testing.T) {
	t.Parallel()

	s := coord.NewTETE(angle.Hour(5.5), angle.Deg(-5.4)).String()

	if !strings.Contains(s, "TETE") {
		t.Errorf("String() = %q, want it to name the system", s)
	}
}

// TestTETEAccessorsCarryWhatTheyWereGiven covers the constructor.
func TestTETEAccessorsCarryWhatTheyWereGiven(t *testing.T) {
	t.Parallel()

	c := coord.NewTETE(angle.Deg(83.8), angle.Deg(-5.4))

	testutil.AssertNear(t, "RA (deg)", c.RA().Degrees(), 83.8, 1e-12)
	testutil.AssertNear(t, "Dec (deg)", c.Dec().Degrees(), -5.4, 1e-12)
}

func BenchmarkApparentToTETE(b *testing.B) {
	ctx := coord.NewContext(
		teteEpoch,
		coord.MustGeodetic(angle.Deg(-70.40417), angle.Deg(-24.62722), 2635),
		atmosphere.Refraction{},
	)

	c := coord.NewApparent(angle.Deg(83.8), angle.Deg(-5.4))

	b.ReportAllocs()

	for range b.N {
		_ = ctx.ApparentToTETE(c)
	}
}
