package kepler_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// jovianMeanElements is the Galilean satellites' row from JPL's satellite
// mean-element table, referred to the local Laplace planes.
//
//	https://ssd.jpl.nasa.gov/sats/elem/  — epoch 2000-01-01.5 TDB, JUP365
//
// The pole differs per satellite, which is the whole reason the plane has to
// be carried alongside the elements rather than assumed: Io's is (268.1,
// 64.5) and Callisto's is (268.7, 64.8), and neither is Jupiter's equator or
// the ecliptic.
var jovianMeanElements = []struct {
	name            string
	aKM, e          float64
	argp, meanAnom  float64 // degrees
	incl, node      float64 // degrees, in the Laplace plane
	poleRA, poleDec float64 // degrees, ICRF
	periodDays      float64
	apsisYears      float64 // "P apsis", years
	nodeYears       float64 // "P node", years
}{
	{"Io", 421_800, 0.004, 49.1, 330.9, 0.0, 0.0, 268.1, 64.5, 1.762732, 1.333, 0.000},
	{"Europa", 671_100, 0.009, 45.0, 345.4, 0.5, 184.0, 268.1, 64.5, 3.525463, 1.394, 30.202},
	{"Ganymede", 1_070_400, 0.001, 198.3, 324.8, 0.2, 58.5, 268.2, 64.6, 7.155588, 68.301, 137.812},
	{"Callisto", 1_882_700, 0.007, 43.8, 87.4, 0.3, 309.1, 268.7, 64.8, 16.690440, 277.921, 577.264},
}

// jovianBare builds the elements with neither the published period nor the
// apsis precession applied — plain two-body motion in the right plane.
func jovianBare(t *testing.T, i int) kepler.Elements {
	t.Helper()

	s := jovianMeanElements[i]

	jupiter, ok := kepler.CentralBodyFor(core.Jupiter)
	if !ok {
		t.Fatal("no central body for Jupiter")
	}

	el, err := kepler.NewElements(time.J2000, kmToAU(s.aKM), s.e,
		angle.Deg(s.incl), angle.Deg(s.node), angle.Deg(s.argp), angle.Deg(s.meanAnom))
	if err != nil {
		t.Fatalf("%s: NewElements: %v", s.name, err)
	}

	return el.WithCentralBody(jupiter).
		WithLaplacePlane(kepler.LaplacePlane{RA: angle.Deg(s.poleRA), Dec: angle.Deg(s.poleDec)})
}

func jovianElements(t *testing.T, i int) kepler.Elements {
	t.Helper()

	s := jovianMeanElements[i]

	jupiter, ok := kepler.CentralBodyFor(core.Jupiter)
	if !ok {
		t.Fatal("no central body for Jupiter")
	}

	el, err := kepler.NewElements(time.J2000, kmToAU(s.aKM), s.e,
		angle.Deg(s.incl), angle.Deg(s.node), angle.Deg(s.argp), angle.Deg(s.meanAnom))
	if err != nil {
		t.Fatalf("%s: NewElements: %v", s.name, err)
	}

	return el.WithCentralBody(jupiter).
		WithLaplacePlane(kepler.LaplacePlane{RA: angle.Deg(s.poleRA), Dec: angle.Deg(s.poleDec)}).
		WithPeriod(s.periodDays).
		WithSecularPrecession(kepler.SecularPrecession{ApsisPeriod: s.apsisYears})
}

// TestOrbitInThePlaneHasThePlanesPole is the check that decides whether the
// rotation is right, and it needs no external position to make it.
//
// Io's published inclination and node are both zero: its orbit lies in its
// Laplace plane by construction. The orbit's angular momentum must therefore
// point along the plane's pole — so if the rotation is built wrongly, the two
// directions separate and say by how much.
func TestOrbitInThePlaneHasThePlanesPole(t *testing.T) {
	el := jovianElements(t, 0) // Io: i = 0, node = 0

	pos, vel, err := el.StateAt(time.J2000)
	if err != nil {
		t.Fatalf("StateAt: %v", err)
	}

	plane, ok := el.LaplacePlane()
	if !ok {
		t.Fatal("elements report no Laplace plane")
	}

	h := pos.Cross(vel).Unit()

	sep := poleSeparation(h, plane.Pole())
	if sep.Degrees() > 1e-9 {
		t.Errorf("orbit normal is %.6f° from the plane's pole; the rotation is wrong",
			sep.Degrees())
	}
}

// TestInclinedOrbitTiltsFromThePole is the same check with the inclination
// switched on: Callisto's orbit is inclined 0.3° to its plane, so its normal
// must sit 0.3° from the pole — not 0, and not 23°.
func TestInclinedOrbitTiltsFromThePole(t *testing.T) {
	for i, s := range jovianMeanElements {
		t.Run(s.name, func(t *testing.T) {
			el := jovianElements(t, i)

			pos, vel, err := el.StateAt(time.J2000)
			if err != nil {
				t.Fatalf("StateAt: %v", err)
			}

			plane, _ := el.LaplacePlane()
			h := pos.Cross(vel).Unit()

			sep := poleSeparation(h, plane.Pole()).Degrees()
			if math.Abs(sep-s.incl) > 1e-9 {
				t.Errorf("orbit normal is %.6f° from the pole, published inclination is %.1f°",
					sep, s.incl)
			}
		})
	}
}

// TestReadingLaplaceElementsAsEclipticIsBadlyWrong measures what the plane is
// worth. The same numbers read against the ecliptic put the satellite in a
// different place, and the size of that difference is the argument for
// carrying the plane at all.
func TestReadingLaplaceElementsAsEclipticIsBadlyWrong(t *testing.T) {
	s := jovianMeanElements[0]

	jupiter, _ := kepler.CentralBodyFor(core.Jupiter)

	base, err := kepler.NewElements(time.J2000, kmToAU(s.aKM), s.e,
		angle.Deg(s.incl), angle.Deg(s.node), angle.Deg(s.argp), angle.Deg(s.meanAnom))
	if err != nil {
		t.Fatalf("NewElements: %v", err)
	}

	ecliptic := base.WithCentralBody(jupiter)
	laplace := ecliptic.WithLaplacePlane(kepler.LaplacePlane{
		RA: angle.Deg(s.poleRA), Dec: angle.Deg(s.poleDec),
	})

	pe, _, err := ecliptic.StateAt(time.J2000)
	if err != nil {
		t.Fatalf("StateAt(ecliptic): %v", err)
	}

	pl, _, err := laplace.StateAt(time.J2000)
	if err != nil {
		t.Fatalf("StateAt(laplace): %v", err)
	}

	sepKM := pe.Sub(pl).Norm() * auMeters / 1e3
	sepDeg := poleSeparation(pe.Unit(), pl.Unit()).Degrees()

	t.Logf("Io read as ecliptic vs Laplace: %.0f km apart, %.1f° as seen from Jupiter", sepKM, sepDeg)

	// Jupiter's obliquity is 3.1°, so its Laplace plane cannot be far from
	// the ecliptic and the measured separation is 2.2°. That is the honest
	// size of the effect — an earlier version of this test expected tens of
	// degrees, having confused this angle with Earth's obliquity — and it is
	// still 16,500 km, four percent of Io's orbital radius.
	if sepDeg < 1.5 || sepDeg > 3.0 {
		t.Errorf("reading Laplace elements as ecliptic moved Io %.2f°, expected about 2.2°", sepDeg)
	}

	if sepKM < 12_000 || sepKM > 22_000 {
		t.Errorf("that is %.0f km, expected about 16,500", sepKM)
	}
}

// TestNoPlaneStillMeansEcliptic keeps the heliocentric path untouched: an
// Elements that never names a plane must behave exactly as before.
func TestNoPlaneStillMeansEcliptic(t *testing.T) {
	el, err := kepler.NewElements(time.J2000, 2.7658, 0.07839,
		angle.Deg(10.587), angle.Deg(80.393), angle.Deg(73.597), angle.Deg(77.372))
	if err != nil {
		t.Fatalf("NewElements: %v", err)
	}

	if _, ok := el.LaplacePlane(); ok {
		t.Error("a plain Elements reports a Laplace plane")
	}
}

// TestPoleIsAUnitVectorOnTheSky guards the pole construction itself, which is
// easy to write with the sine and cosine transposed and still look plausible.
func TestPoleIsAUnitVectorOnTheSky(t *testing.T) {
	cases := []struct {
		ra, dec float64
		x, y, z float64
	}{
		{0, 0, 1, 0, 0},
		{90, 0, 0, 1, 0},
		{0, 90, 0, 0, 1},
		{0, -90, 0, 0, -1},
		{180, 0, -1, 0, 0},
	}

	for _, c := range cases {
		p := kepler.LaplacePlane{RA: angle.Deg(c.ra), Dec: angle.Deg(c.dec)}.Pole()

		if math.Abs(p.Norm()-1) > 1e-12 {
			t.Errorf("pole at (%v, %v) has length %v", c.ra, c.dec, p.Norm())
		}

		if math.Abs(p.X-c.x) > 1e-12 || math.Abs(p.Y-c.y) > 1e-12 || math.Abs(p.Z-c.z) > 1e-12 {
			t.Errorf("pole at (%v, %v) = %v, want (%v, %v, %v)", c.ra, c.dec, p, c.x, c.y, c.z)
		}
	}
}

// poleSeparation is the angle between two unit vectors, computed from the
// cross product rather than the dot product.
//
// acos loses most of its precision near zero separation — a dot product one
// float64 step below 1 comes back as 8e-07 degrees rather than 0 — which is
// exactly the regime these tests work in, since a satellite in its own
// Laplace plane should be at zero. asin of the cross-product magnitude stays
// accurate there.
func poleSeparation(a, b vector.Vec3) angle.Angle {
	return angle.Rad(math.Atan2(a.Cross(b).Norm(), a.Dot(b)))
}

// TestTwoBodyPeriodMissesThePublishedOne quantifies, for every Galilean
// satellite, what plain two-body motion gets wrong.
func TestTwoBodyPeriodMissesThePublishedOne(t *testing.T) {
	jupiter, ok := kepler.CentralBodyFor(core.Jupiter)
	if !ok {
		t.Fatal("no central body for Jupiter")
	}

	for i, s := range jovianMeanElements {
		t.Run(s.name, func(t *testing.T) {
			a := s.aKM * 1e3
			twoBody := 2 * math.Pi * math.Sqrt(a*a*a/jupiter.GM) / 86400

			got := periodDays(t, jovianBare(t, i), twoBody)
			if rel := math.Abs(got-twoBody) / twoBody; rel > 1e-5 {
				t.Errorf("propagated period %.6f d, two-body formula says %.6f", got, twoBody)
			}

			gap := 100 * (twoBody - s.periodDays) / s.periodDays
			t.Logf("two-body %.6f d, published %.6f d, gap %+.3f%%", twoBody, s.periodDays, gap)

			// The gap is not uniform, and assuming it was is how this test
			// first got written. Io and Europa are off by 0.4% and 0.8%;
			// Ganymede and Callisto agree to about one part in 100,000.
			if math.Abs(gap) > 1.0 {
				t.Errorf("gap is %+.3f%%, larger than any of the four measured", gap)
			}

			if (s.name == "Io" || s.name == "Europa") && gap < 0.3 {
				t.Errorf("%s gap is %+.3f%%; it was 0.4-0.8%% when measured", s.name, gap)
			}
		})
	}
}

// TestCorrectedPropagationRecoversTheSiderealPeriod is the check that says the
// two corrections belong together.
//
// The table's period is anomalistic — measured periapsis to periapsis — and
// the apsis is itself turning. Advancing the mean anomaly at that rate while
// walking the apsis back by its own rate should leave the sidereal period,
// the one an inertial observer sees. The table's own two columns predict it:
//
//	1/P_sidereal = 1/P_anomalistic - 1/P_apsis
//
// For Io that gives 1.769137 days, which is Io's sidereal period. Nothing
// here was given that number.
func TestCorrectedPropagationRecoversTheSiderealPeriod(t *testing.T) {
	const daysPerJulianYear = 365.25

	for i, s := range jovianMeanElements {
		t.Run(s.name, func(t *testing.T) {
			want := 1 / (1/s.periodDays - 1/(s.apsisYears*daysPerJulianYear))

			got := periodDays(t, jovianElements(t, i), want)

			t.Logf("propagated %.6f d, relation predicts %.6f d", got, want)

			// Ganymede and Callisto land within 3e-07 and Io within 3e-05.
			// Europa is the loosest at 1.2e-04, and it has the largest
			// eccentricity of the four — the relation assumes uniform rates,
			// which a more eccentric orbit satisfies least well.
			if rel := math.Abs(got-want) / want; rel > 2e-4 {
				t.Errorf("propagated %.6f d, relation predicts %.6f (relative %.2g)", got, want, rel)
			}
		})
	}
}

// TestEitherCorrectionAloneIsWorseThanNeither is why they are documented as a
// pair rather than two options.
//
// Advancing the mean anomaly at the anomalistic rate without turning the apsis
// back — or turning the apsis without the anomalistic rate — leaves the orbit
// rotating at the wrong rate, and the error is far larger than the two-body
// error it was meant to remove.
func TestEitherCorrectionAloneIsWorseThanNeither(t *testing.T) {
	const idx = 0 // Io, where the apsis moves fastest

	s := jovianMeanElements[idx]
	bare := jovianBare(t, idx)

	both := jovianElements(t, idx)
	periodOnly := bare.WithPeriod(s.periodDays)
	apsisOnly := bare.WithSecularPrecession(kepler.SecularPrecession{ApsisPeriod: s.apsisYears})

	// Ten days on: how far each has rotated away from the fully corrected one.
	at := time.J2000.AddDays(10)

	ref, _, err := both.StateAt(at)
	if err != nil {
		t.Fatalf("StateAt: %v", err)
	}

	for _, c := range []struct {
		name string
		el   kepler.Elements
	}{{"period only", periodOnly}, {"apsis only", apsisOnly}} {
		pos, _, serr := c.el.StateAt(at)
		if serr != nil {
			t.Fatalf("%s: %v", c.name, serr)
		}

		sep := poleSeparation(pos.Unit(), ref.Unit()).Degrees()
		t.Logf("%-12s is %.2f° from the corrected orbit after 10 days", c.name, sep)

		if sep < 1 {
			t.Errorf("%s differs by only %.3f°; the two corrections should not be "+
				"separable without consequence", c.name, sep)
		}
	}
}
