package magnitude_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
)

// TestMarsReproducesItsAuthorsTestData runs the Mars test cases Mallama and
// Hilton publish with their reference code (Ap_Mag_Input_V3.txt, SourceForge
// planetary-magnitudes, Ap_Mag_Current_Version), on their own inputs, to their
// own criterion: their code counts a difference over 0.001 mag as an error.
//
// This is the check that does not depend on reading the tables right. The
// inputs are theirs, the expected magnitudes are what their code computes, and
// the cases reach both correction tables at different places, both phase-curve
// regimes, and a phase angle only an observer beyond Mars sees (from Jupiter).
func TestMarsReproducesItsAuthorsTestData(t *testing.T) {
	cases := []struct {
		name                   string
		r, delta, phase        float64
		subObserver, subSolar  float64
		eclipticLon, apparentV float64
	}{
		{"2003-Aug-28, very bright", 1.381191244505, 0.37274381097911, 4.8948, 329.27, 330.76, 334.4996, -2.862},
		{"2004-Jul-19, faint", 1.664150453905, 2.58995164518460, 11.5877, 76.84, 64.66, 147.3485, 1.788},
		{"2018-Feb-11, from Jupiter", 1.591952180003, 3.85882552272013, 167.9000, 29.50, 221.11, 212.8886, 8.977},
	}

	for _, tc := range cases {
		cm := magnitude.MarsEffectiveCentralMeridian(tc.subObserver, tc.subSolar)
		ls := magnitude.MarsSolarLongitude(tc.eclipticLon)

		got := magnitude.MarsMag(tc.r, tc.delta, tc.phase, cm, ls)
		if diff := got - tc.apparentV; math.Abs(diff) > 0.001 {
			t.Errorf("%s: V = %+.4f, the authors' code gives %+.3f (off by %+.4f)", tc.name, got, tc.apparentV, diff)
		}
	}
}

// marsHorizons is Horizons' geometry and APmag for Mars, OBSERVER tables with
// CENTER='500@399', QUANTITIES='9,14,15,18,19,20,24', EXTRA_PREC, 00:00 UT,
// queried 2026-09-23. The first two dates are the authors' own test cases;
// Horizons reproduces their expected magnitudes exactly.
var marsHorizons = []struct {
	y, m, d               int
	subObserver, subSolar float64 // ObsSub-LON, SunSub-LON: west longitudes
	eclipticLon           float64 // hEcl-Lon: heliocentric J2000 ecliptic
	sto                   float64 // S-T-O
	apparentV             float64 // APmag
}{
	{2003, 8, 28, 329.270132, 330.759653, 334.499639, 4.8948, -2.862},
	{2004, 7, 19, 76.839736, 64.666443, 147.348491, 11.5877, 1.788},
	{2020, 1, 1, 44.669891, 68.092854, 213.854912, 24.2590, 1.548},
	{2021, 3, 15, 179.351186, 146.789539, 102.277632, 36.2079, 1.037},
	{2022, 6, 15, 28.640534, 75.683773, 332.054963, 43.1263, 0.577},
	{2023, 1, 20, 141.536748, 116.243991, 97.038706, 28.9234, -0.683},
	{2023, 8, 13, 331.767435, 311.467825, 188.844261, 18.4201, 1.768},
	{2024, 3, 10, 72.891785, 93.228237, 298.870492, 20.6988, 1.222},
	{2025, 3, 23, 79.998224, 46.212631, 145.267094, 34.6780, 0.231},
	{2025, 11, 20, 249.936596, 241.851914, 259.668797, 8.8424, 1.421},
	{2026, 9, 23, 113.515923, 145.641709, 81.294513, 35.2941, 1.086},
}

// TestMarsModelOnHorizonsGeometryReproducesHorizons feeds the model Horizons'
// own geometry, so that what is left is the model alone: the tables, the
// interpolation, and the V(1,0) and phase curve.
//
// It is what decided the interpolation. Every residual here is within
// Horizons' rounding of APmag to 0.001; interpolating by textbook Stirling
// instead leaves 0.0029 on 2024-03-10 (see marsCorrection).
func TestMarsModelOnHorizonsGeometryReproducesHorizons(t *testing.T) {
	// Horizons' r and delta for the same rows, AU.
	distances := [][2]float64{
		{1.381191244490, 0.37274381097652},
		{1.664150453907, 2.58995164518292},
		{1.589806088677, 2.18441290381004},
		{1.598404188476, 1.60154314986098},
		{1.381615839871, 1.38212079937081},
		{1.586497224879, 0.77271401214696},
		{1.639179336869, 2.42618324443049},
		{1.406019637686, 2.17513675885868},
		{1.662971122009, 1.05476620144016},
		{1.477948981361, 2.42213129313410},
		{1.548263212821, 1.71873361313683},
	}

	for i, h := range marsHorizons {
		cm := magnitude.MarsEffectiveCentralMeridian(h.subObserver, h.subSolar)
		ls := magnitude.MarsSolarLongitude(h.eclipticLon)

		got := magnitude.MarsMag(distances[i][0], distances[i][1], h.sto, cm, ls)
		if diff := got - h.apparentV; math.Abs(diff) > 0.0007 {
			t.Errorf("%04d-%02d-%02d: V = %+.4f on Horizons' geometry, Horizons gives %+.3f (off by %+.4f)",
				h.y, h.m, h.d, got, h.apparentV, diff)
		}
	}
}

// TestMarsGeometryAgreesWithHorizons checks astrogo's own geometry — the
// sub-observer and sub-solar longitudes from Mars's IAU rotation model, and the
// heliocentric ecliptic longitude — against the columns Horizons publishes.
func TestMarsGeometryAgreesWithHorizons(t *testing.T) {
	p := defaultProvider()

	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	// Measured over these dates: sub-observer longitude 0.0074° worst, sub-solar
	// 0.015°, ecliptic longitude 0.0088°. The residuals are systematic and have
	// sources: the pole model (IAU 2009 here), the Sun-to-Mars light time
	// Horizons includes in the sub-solar point, and the down-leg light time it
	// includes in hEcl-Lon. None of it matters to the magnitude: the tables
	// change by at most 0.009 mag per degree, so 0.03° is 0.0003 mag.
	const (
		// Longitude on Mars, degrees.
		tolLongitude = 0.03
		// Heliocentric ecliptic longitude, degrees.
		tolEcliptic = 0.02
	)

	for _, h := range marsHorizons {
		tm := time.Date(h.y, time.Month(h.m), h.d, 0, 0, 0, 0, time.LocationUTC)
		sunToPlanet, observerToPlanet, delta := marsVectors(t, p, tm)

		subObserver, subSolar := magnitude.MarsSubLongitudes(sunToPlanet, observerToPlanet, delta, tm)
		eclipticLon := magnitude.EclipticLongitude(sunToPlanet)

		for _, c := range []struct {
			name      string
			got, want float64
			tol       float64
		}{
			{"sub-observer longitude", subObserver, h.subObserver, tolLongitude},
			{"sub-solar longitude", subSolar, h.subSolar, tolLongitude},
			{"heliocentric ecliptic longitude", eclipticLon, h.eclipticLon, tolEcliptic},
		} {
			if diff := angle.Deg(c.got - c.want).Wrap180().Degrees(); math.Abs(diff) > c.tol {
				t.Errorf("%04d-%02d-%02d: %s %.4f°, Horizons gives %.4f° (off by %+.4f°, limit %.2f°)",
					h.y, h.m, h.d, c.name, c.got, c.want, diff, c.tol)
			}
		}
	}
}

// marsVectors is the geometry PlanetApparent builds for Mars at tm: the
// heliocentric and geocentric vectors to Mars and the geocentric distance.
func marsVectors(t *testing.T, p eph.Provider, tm time.Time) (sunToPlanet, observerToPlanet [3]float64, delta float64) {
	t.Helper()

	mars, err := p.State(eph.Mars, tm)
	if err != nil {
		t.Fatalf("Mars state at %s: %v", tm, err)
	}

	sun, err := p.State(eph.Sun, tm)
	if err != nil {
		t.Fatalf("Sun state at %s: %v", tm, err)
	}

	sunToPlanet = [3]float64{mars.Pos.X - sun.Pos.X, mars.Pos.Y - sun.Pos.Y, mars.Pos.Z - sun.Pos.Z}
	observerToPlanet = [3]float64{mars.Pos.X, mars.Pos.Y, mars.Pos.Z}

	return sunToPlanet, observerToPlanet, mars.Pos.Norm()
}

// TestMarsCorrectionIsItsTableAtEveryNode checks the interpolation against the
// tables where it has to agree with them exactly, and the tables' padding
// against the tables: the first and last two entries repeat the other end, and
// a transcription slip there would shift the correction near 0° and 360° only.
func TestMarsCorrectionIsItsTableAtEveryNode(t *testing.T) {
	tables := []struct {
		name  string
		table *[40]float64
	}{
		{"rotation", magnitude.MarsRotationCorrection},
		{"orbital", magnitude.MarsOrbitalCorrection},
	}

	for _, tb := range tables {
		for k := range 36 {
			deg := 10 * float64(k)
			if got, want := magnitude.MarsCorrection(tb.table, deg), tb.table[k+2]; got != want {
				t.Errorf("%s correction at %v°: %v, table gives %v", tb.name, deg, got, want)
			}
		}

		for k := range 4 {
			if tb.table[k] != tb.table[k+36] {
				t.Errorf("%s table: entry %d (%v°) is %v, entry %d (%v°) is %v; they are the same longitude",
					tb.name, k, 10*(k-2), tb.table[k], k+36, 10*(k+34), tb.table[k+36])
			}
		}
	}
}

// TestMarsCorrectionWrapsTheCircle checks angles outside [0°, 360°), including
// one so close below zero that wrapping it lands on 360° exactly — past the
// last interval the table has, unless the index is held inside it.
func TestMarsCorrectionWrapsTheCircle(t *testing.T) {
	table := magnitude.MarsRotationCorrection

	for _, c := range []struct {
		deg, same float64
	}{
		{360, 0},
		{-1e-14, 0},
		{725, 5},
		{-35, 325},
	} {
		got := magnitude.MarsCorrection(table, c.deg)
		want := magnitude.MarsCorrection(table, c.same)

		if math.Abs(got-want) > 1e-12 {
			t.Errorf("correction at %v°: %v, at %v°: %v; they are the same longitude", c.deg, got, c.same, want)
		}
	}
}

// TestMarsCorrectionRefusesANonFiniteAngle: NaN, not a panic, for an angle
// that indexes nothing.
func TestMarsCorrectionRefusesANonFiniteAngle(t *testing.T) {
	for _, deg := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := magnitude.MarsCorrection(magnitude.MarsRotationCorrection, deg); !math.IsNaN(got) {
			t.Errorf("correction at %v°: %v, want NaN", deg, got)
		}
	}
}
