package coord_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// The system is *defined* by two directions — where its pole is and where its
// longitude starts — so those two are the exact checks, at float precision.
// Everything after them is about whether the rotation means what the system is
// for, which the definition alone cannot say.

// TestSupergalacticPoleAndOriginAreWhereTheLiteraturePutsThem pins the
// definition from both ends.
//
// de Vaucouleurs puts the north supergalactic pole at Galactic l = 47.37°,
// b = +6.32°, and supergalactic longitude zero at Galactic l = 137.37°, b = 0.
// A rotation built from those two statements has to send the first to the pole
// and the second to the origin, and nothing else it does matters if it does
// not.
func TestSupergalacticPoleAndOriginAreWhereTheLiteraturePutsThem(t *testing.T) {
	t.Parallel()

	pole := coord.ICRSToSupergalactic(
		coord.GalacticToICRS(coord.NewGalactic(angle.Deg(47.37), angle.Deg(6.32))))

	// Only the latitude is meaningful at a pole; every longitude names it.
	testutil.AssertNear(t, "north supergalactic pole, SGB (deg)", pole.SGB().Degrees(), 90, 1e-9)

	origin := coord.ICRSToSupergalactic(
		coord.GalacticToICRS(coord.NewGalactic(angle.Deg(137.37), angle.Deg(0))))

	testutil.AssertNear(t, "longitude origin, SGL (deg)",
		origin.SGL().Wrap180().Degrees(), 0, 1e-9)
	testutil.AssertNear(t, "longitude origin, SGB (deg)", origin.SGB().Degrees(), 0, 1e-9)
}

// TestTheSuperclusterPlaneIsNearlyPerpendicularToTheGalacticOne checks the
// entered pole against a consequence of itself rather than against a
// restatement of the number.
//
// The pole sits at Galactic b = +6.32°, so it is 83.68° from the north
// Galactic pole and the two planes are inclined by that much. That is a fact
// about the sky with a visible consequence — the Local Supercluster is cut in
// half by the Milky Way's own dust, which is why the zone of avoidance matters
// for extragalactic surveys — and a mistyped or transposed pole would not
// reproduce it.
func TestTheSuperclusterPlaneIsNearlyPerpendicularToTheGalacticOne(t *testing.T) {
	t.Parallel()

	nsgp := coord.GalacticToICRS(coord.NewGalactic(angle.Deg(47.37), angle.Deg(6.32)))
	ngp := coord.GalacticToICRS(coord.NewGalactic(angle.Deg(0), angle.Deg(90)))

	between := coord.Separation(nsgp, ngp).Degrees()

	t.Logf("the two poles are %.4f degrees apart, so the planes are inclined by that much", between)

	testutil.AssertNear(t, "inclination to the galactic plane (deg)", between, 90-6.32, 1e-6)
}

// TestVirgoLiesOnTheSuperclusterPlane is the test that would fail if the
// rotation were self-consistent and pointed at nothing.
//
// The Virgo cluster is the center of the Local Supercluster. That is the
// structure this coordinate system exists to flatten, so Virgo has to come out
// on the equator — and it is the one claim here that no amount of internal
// consistency can produce. A rotation about the wrong pole would still send
// its own pole to 90 and its own origin to 0, and would scatter the galaxies
// it was built to line up.
func TestVirgoLiesOnTheSuperclusterPlane(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		ra, dec float64
	}{
		{"M87, the cluster's central galaxy", 187.705930, 12.391123},
		{"M49", 187.444882, 8.000410},
		{"M60", 190.916708, 11.552706},
		{"M84", 186.265597, 12.886983},
	} {
		sg := coord.ICRSToSupergalactic(coord.NewICRS(angle.Deg(tc.ra), angle.Deg(tc.dec)))

		t.Logf("%-34s SGL %8.4f SGB %+8.4f", tc.name, sg.SGL().Degrees(), sg.SGB().Degrees())

		// The cluster has real depth and width on the sky, so this is a band
		// rather than a line. Five degrees is loose enough to hold the whole
		// cluster and far tighter than the 42° a Galactic-center direction
		// lands at, which is the scale a wrong rotation would produce.
		if sgb := math.Abs(sg.SGB().Degrees()); sgb > 5 {
			t.Errorf("%s has supergalactic latitude %+.4f, %.4f degrees off the plane — "+
				"the Virgo cluster defines that plane, so this is the rotation "+
				"pointing somewhere else", tc.name, sg.SGB().Degrees(), sgb)
		}

		// And they are all in the same part of it, near SGL 100, rather than
		// spread around a circle that happens to include the plane.
		if sgl := sg.SGL().Degrees(); sgl < 90 || sgl > 115 {
			t.Errorf("%s has supergalactic longitude %.4f, outside the 90-115 degrees "+
				"the Virgo cluster occupies", tc.name, sgl)
		}
	}
}

// TestSupergalacticRoundTrips covers the inverse, at the places a rotation
// composed of three others is most likely to lose one.
func TestSupergalacticRoundTrips(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		ra, dec float64
	}{
		{"M87", 187.705930, 12.391123},
		{"the north celestial pole", 0, 89.9},
		{"the south celestial pole", 0, -89.9},
		{"just above the RA wrap", 0.01, 0},
		{"just below it", 359.99, 0},
		{"the galactic center", 266.405, -28.936},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := coord.NewICRS(angle.Deg(tc.ra), angle.Deg(tc.dec))

			back := coord.SupergalacticToICRS(coord.ICRSToSupergalactic(src))

			// Compared as a separation rather than coordinate by coordinate,
			// so a pole — where right ascension means nothing — is judged on
			// where it actually is.
			if sep := coord.Separation(src, back).Arcseconds(); sep > 1e-6 {
				t.Errorf("round trip moved %s by %g arcsec", tc.name, sep)
			}
		})
	}
}

// TestSupergalacticLongitudeStaysInRange keeps the wrap honest.
func TestSupergalacticLongitudeStaysInRange(t *testing.T) {
	t.Parallel()

	for ra := 0.0; ra < 360; ra += 7 {
		for dec := -80.0; dec <= 80; dec += 7 {
			sgl := coord.ICRSToSupergalactic(
				coord.NewICRS(angle.Deg(ra), angle.Deg(dec))).SGL().Degrees()

			if sgl < 0 || sgl >= 360 {
				t.Fatalf("RA %g Dec %g gave supergalactic longitude %g, want [0, 360)",
					ra, dec, sgl)
			}
		}
	}
}

// TestSupergalacticAccessorsAndString covers the value type itself.
func TestSupergalacticAccessorsAndString(t *testing.T) {
	t.Parallel()

	c := coord.NewSupergalactic(angle.Deg(102.88), angle.Deg(-2.35))

	testutil.AssertNear(t, "SGL (deg)", c.SGL().Degrees(), 102.88, 1e-12)
	testutil.AssertNear(t, "SGB (deg)", c.SGB().Degrees(), -2.35, 1e-12)

	if s := c.String(); !strings.Contains(s, "Supergalactic") {
		t.Errorf("String() = %q, want it to name the system", s)
	}

	// The vector round trip, since both directions are exported.
	var back coord.Supergalactic

	back.FromUnitVector(c.ToUnitVector())

	testutil.AssertNear(t, "SGL through a vector", back.SGL().Degrees(), 102.88, 1e-9)
	testutil.AssertNear(t, "SGB through a vector", back.SGB().Degrees(), -2.35, 1e-9)
}

func BenchmarkICRSToSupergalactic(b *testing.B) {
	c := coord.NewICRS(angle.Deg(187.705930), angle.Deg(12.391123))

	b.ReportAllocs()

	for range b.N {
		_ = coord.ICRSToSupergalactic(c)
	}
}
