package skybrightness_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// zodiacalAt is the 500 nm brightness for a direction at 1 AU.
func zodiacalAt(t *testing.T, dlon, beta float64) float64 {
	t.Helper()

	v, err := skybrightness.ZodiacalBrightnessAt(skybrightness.ZodiacalGeometry{
		DifferentialLongitude: angle.Deg(dlon),
		EclipticLatitude:      angle.Deg(beta),
		SunDistanceAU:         1,
	})
	if err != nil {
		t.Fatalf("ZodiacalBrightnessAt(%v, %v): %v", dlon, beta, err)
	}

	return v
}

// toMagPerArcsec converts a V-band spectral radiance to mag/arcsec^2 against
// the Johnson V zero point, 3.63e-9 erg s^-1 cm^-2 A^-1 (Bessell 1979).
func toMagPerArcsec(radiance float64) float64 {
	const (
		vZeroPointCGS = 3.63e-9
		arcsecSqPerSr = 4.25451702961522e10
	)

	return -2.5 * math.Log10(radiance*1e2/arcsecSqPerSr/vZeroPointCGS)
}

// The check against a number from outside Leinert's table: zodiacal light at
// the ecliptic pole is about 23.3 mag/arcsec^2 in V.
//
// That is independently reasonable rather than circular — a dark site's total
// V sky brightness is around 22.0, and zodiacal light is roughly a quarter of
// it at high ecliptic latitude, which puts this component near 23.5. Landing
// there exercises the table's unit prefix, the per-micron to per-nanometre
// conversion and the pole value all at once; a factor of ten anywhere shows
// up as 2.5 magnitudes.
func TestZodiacalPoleMatchesKnownBrightness(t *testing.T) {
	t.Parallel()

	pole, err := skybrightness.ZodiacalBrightnessAt(skybrightness.ZodiacalGeometry{
		DifferentialLongitude: angle.Deg(90),
		EclipticLatitude:      angle.Deg(90),
		SunDistanceAU:         1,
	})
	if err != nil {
		t.Fatalf("ZodiacalBrightnessAt(pole): %v", err)
	}

	mag := toMagPerArcsec(pole)
	t.Logf("ecliptic pole: %.4g W m^-2 sr^-1 nm^-1 = %.2f mag/arcsec^2", pole, mag)

	if mag < 22.5 || mag > 24 {
		t.Errorf("pole = %.2f mag/arcsec^2, want about 23.3", mag)
	}

	// The table quotes the pole separately from its grid; it must be exactly
	// that value, not an extrapolation of the 75-degree column.
	want := skybrightness.ZodiacalPoleBrightness * 1e-8 / 1000
	if rel := math.Abs(pole-want) / want; rel > 1e-12 {
		t.Errorf("pole = %.6g, want the quoted %.6g", pole, want)
	}
}

// Tabulated grid points must come back exactly, which pins the axes against
// the published table rather than only the interpolation between them.
func TestZodiacalTableCorners(t *testing.T) {
	t.Parallel()

	const unitScale = 1e-8 / 1000

	cases := []struct {
		dlon, beta, want float64
	}{
		{15, 0, 11500}, // the brightest tabulated cell
		{180, 0, 230},  // the gegenschein
		{90, 0, 259},
		{180, 75, 72}, // the darkest tabulated cell
		{45, 30, 250},
		{120, 45, 90},
	}

	for _, tc := range cases {
		got := zodiacalAt(t, tc.dlon, tc.beta)

		if rel := math.Abs(got-tc.want*unitScale) / (tc.want * unitScale); rel > 1e-12 {
			t.Errorf("(%v, %v) = %.6g, want the tabulated %.6g",
				tc.dlon, tc.beta, got, tc.want*unitScale)
		}
	}
}

// The gegenschein: the anti-solar point is brighter than the sky either side
// of it, from backscattering off interplanetary dust. It is a real, observable
// feature and a model that smoothed it away would be wrong.
func TestZodiacalGegenschein(t *testing.T) {
	t.Parallel()

	anti := zodiacalAt(t, 180, 0)
	before := zodiacalAt(t, 150, 0)
	after := zodiacalAt(t, 165, 0)

	if anti <= after || after <= before {
		t.Errorf("no gegenschein: 150 deg %.4g, 165 deg %.4g, 180 deg %.4g", before, after, anti)
	}
}

// Away from the gegenschein the sky darkens with distance from the Sun, and
// darkens with ecliptic latitude at fixed elongation.
func TestZodiacalFallsWithElongationAndLatitude(t *testing.T) {
	t.Parallel()

	prev := math.Inf(1)

	for _, dlon := range []float64{15, 20, 25, 30, 45, 60, 75, 90, 105, 120} {
		v := zodiacalAt(t, dlon, 0)
		if v >= prev {
			t.Errorf("%v deg from the Sun gives %.4g, not less than %.4g nearer it", dlon, v, prev)
		}

		prev = v
	}

	prev = math.Inf(1)

	for _, beta := range []float64{0, 5, 10, 15, 20, 25, 30, 45, 60, 75} {
		v := zodiacalAt(t, 90, beta)
		if v >= prev {
			t.Errorf("latitude %v gives %.4g, not less than %.4g nearer the ecliptic", beta, v, prev)
		}

		prev = v
	}
}

// Interpolation between two tabulated cells lands between them, and at the
// midpoint of a linear pair lands on the mean.
func TestZodiacalInterpolates(t *testing.T) {
	t.Parallel()

	lo := zodiacalAt(t, 90, 0)  // 259
	hi := zodiacalAt(t, 105, 0) // 212

	mid := zodiacalAt(t, 97.5, 0)

	if want := (lo + hi) / 2; math.Abs(mid-want)/want > 1e-12 {
		t.Errorf("midpoint = %.6g, want the mean %.6g of its neighbours", mid, want)
	}

	// And in latitude, between the 75-degree column and the quoted pole.
	between := zodiacalAt(t, 90, 82.5)

	edge := zodiacalAt(t, 90, 75)
	pole := skybrightness.ZodiacalPoleBrightness * 1e-8 / 1000

	if between >= edge || between <= pole {
		t.Errorf("82.5 deg = %.6g, want between the 75-deg %.6g and the pole %.6g", between, edge, pole)
	}
}

// The solar vicinity is not tabulated and must be refused rather than
// extrapolated — the brightness there climbs by another order of magnitude.
func TestZodiacalRefusesTheSolarVicinity(t *testing.T) {
	t.Parallel()

	for _, tc := range [][2]float64{{0, 0}, {0, 5}, {5, 10}, {10, 0}, {2, 3}} {
		_, err := skybrightness.ZodiacalBrightnessAt(skybrightness.ZodiacalGeometry{
			DifferentialLongitude: angle.Deg(tc[0]),
			EclipticLatitude:      angle.Deg(tc[1]),
			SunDistanceAU:         1,
		})
		if !errors.Is(err, skybrightness.ErrZodiacalGeometry) {
			t.Errorf("(%v, %v): err = %v, want ErrZodiacalGeometry", tc[0], tc[1], err)
		}
	}
}

// The cloud is symmetric about the Sun-Earth line, so the sign of the
// differential longitude and of the latitude cannot matter.
func TestZodiacalIsSymmetric(t *testing.T) {
	t.Parallel()

	for _, tc := range [][2]float64{{45, 20}, {120, 5}, {90, 60}} {
		base := zodiacalAt(t, tc[0], tc[1])

		for _, mirror := range [][2]float64{
			{-tc[0], tc[1]}, {tc[0], -tc[1]}, {-tc[0], -tc[1]}, {tc[0] + 360, tc[1]},
		} {
			if got := zodiacalAt(t, mirror[0], mirror[1]); got != base {
				t.Errorf("(%v, %v) = %.6g, want the same as (%v, %v)'s %.6g",
					mirror[0], mirror[1], got, tc[0], tc[1], base)
			}
		}
	}
}

// Leinert et al. state the sign convention outright: f_co below 1 blueward of
// 500 nm and above 1 redward, with the reddening stronger at small
// elongations. That is the paper's own words, so it is the right thing to
// assert rather than the coefficients themselves.
func TestZodiacalColourCorrectionSign(t *testing.T) {
	t.Parallel()

	for _, elongation := range []float64{15, 30, 60, 90, 150} {
		e := angle.Deg(elongation)

		if got := skybrightness.ZodiacalColourCorrection(500, e); math.Abs(got-1) > 1e-12 {
			t.Errorf("at %v deg, 500 nm gives %v, want exactly 1", elongation, got)
		}

		if got := skybrightness.ZodiacalColourCorrection(400, e); got >= 1 {
			t.Errorf("at %v deg, 400 nm gives %v, want below 1", elongation, got)
		}

		if got := skybrightness.ZodiacalColourCorrection(800, e); got <= 1 {
			t.Errorf("at %v deg, 800 nm gives %v, want above 1", elongation, got)
		}
	}

	// Reddening is stronger at small elongations.
	near := skybrightness.ZodiacalColourCorrection(1000, angle.Deg(30))
	far := skybrightness.ZodiacalColourCorrection(1000, angle.Deg(90))

	if near <= far {
		t.Errorf("30 deg gives %v at 1000 nm, want more reddening than 90 deg's %v", near, far)
	}
}

// The heliocentric factor is a -2.3 power, so perihelion is measurably
// brighter than aphelion — about 8 per cent over Earth's orbit.
func TestZodiacalHeliocentricScaling(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(500, 1, 2)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	at := func(au float64) float64 {
		dst := skybrightness.NewSpectralRadiance(grid)

		if _, err := skybrightness.ZodiacalRadiance(dst, grid, skybrightness.ZodiacalGeometry{
			DifferentialLongitude: angle.Deg(90),
			EclipticLatitude:      angle.Deg(0),
			SunDistanceAU:         au,
		}); err != nil {
			t.Fatalf("ZodiacalRadiance(%v AU): %v", au, err)
		}

		return dst[0]
	}

	one := at(1.0)

	if got, want := at(0.9832), one*math.Pow(0.9832, -2.3); math.Abs(got-want)/want > 1e-12 {
		t.Errorf("perihelion gave %.6g, want %.6g", got, want)
	}

	ratio := at(0.9832) / at(1.0167)
	if ratio < 1.06 || ratio > 1.10 {
		t.Errorf("perihelion/aphelion = %.4f, want about 1.08", ratio)
	}
}

// The seasonal term applies only at high ecliptic latitude, where the Earth's
// excursion out of the dust plane matters, and is bounded at 10 per cent.
func TestZodiacalSeasonalTerm(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(500, 1, 2)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	at := func(beta, earthLon float64) float64 {
		dst := skybrightness.NewSpectralRadiance(grid)

		if _, err := skybrightness.ZodiacalRadiance(dst, grid, skybrightness.ZodiacalGeometry{
			DifferentialLongitude: angle.Deg(90),
			EclipticLatitude:      angle.Deg(beta),
			SunDistanceAU:         1,
			EarthLongitude:        angle.Deg(earthLon),
		}); err != nil {
			t.Fatalf("ZodiacalRadiance: %v", err)
		}

		return dst[0]
	}

	// Low latitude: no seasonal dependence at all.
	if a, b := at(20, 0), at(20, 186); a != b {
		t.Errorf("at 20 deg latitude the season changed the answer: %v vs %v", a, b)
	}

	// High latitude: a bounded swing, extreme a quarter-turn from the node.
	high, low := at(75, 96+90), at(75, 96-90)

	if high <= low {
		t.Errorf("no seasonal variation at 75 deg: %v vs %v", high, low)
	}

	if ratio := high / low; ratio < 1.1 || ratio > 1.3 {
		t.Errorf("seasonal swing is %.3f, want about 1.22 for a plus/minus 10 per cent term", ratio)
	}
}

func TestZodiacalLightRejectsBadInput(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	good := skybrightness.ZodiacalGeometry{
		DifferentialLongitude: angle.Deg(90),
		EclipticLatitude:      angle.Deg(30),
		SunDistanceAU:         1,
	}

	if _, err := skybrightness.ZodiacalRadiance(make(skybrightness.SpectralRadiance, 3), grid, good); !errors.Is(err, unit.ErrGridMismatch) {
		t.Errorf("short destination: err = %v, want ErrGridMismatch", err)
	}

	dst := skybrightness.NewSpectralRadiance(grid)

	for _, au := range []float64{0, -1, math.NaN()} {
		bad := good
		bad.SunDistanceAU = au

		if _, err := skybrightness.ZodiacalRadiance(dst, grid, bad); !errors.Is(err, skybrightness.ErrZodiacalGeometry) {
			t.Errorf("%v AU: err = %v, want ErrZodiacalGeometry", au, err)
		}
	}
}

// Accumulation, not assignment.
func TestZodiacalLightAccumulates(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(500, 1, 2)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	geom := skybrightness.ZodiacalGeometry{
		DifferentialLongitude: angle.Deg(90),
		EclipticLatitude:      angle.Deg(30),
		SunDistanceAU:         1,
	}

	once := skybrightness.NewSpectralRadiance(grid)
	if _, err := skybrightness.ZodiacalRadiance(once, grid, geom); err != nil {
		t.Fatalf("ZodiacalRadiance: %v", err)
	}

	twice := skybrightness.NewSpectralRadiance(grid)
	for range 2 {
		if _, err := skybrightness.ZodiacalRadiance(twice, grid, geom); err != nil {
			t.Fatalf("ZodiacalRadiance: %v", err)
		}
	}

	if rel := math.Abs(twice[0]-2*once[0]) / (2 * once[0]); rel > 1e-12 {
		t.Errorf("two calls gave %v, want twice the %v of one", twice[0], once[0])
	}
}

// The elongation the colour correction uses follows from the geometry:
// cos(eps) = cos(dlon)*cos(beta).
func TestZodiacalElongation(t *testing.T) {
	t.Parallel()

	cases := []struct{ dlon, beta, want float64 }{
		{90, 0, 90},
		{0, 30, 30},
		{180, 0, 180},
		{90, 90, 90},
	}

	for _, tc := range cases {
		got := skybrightness.ZodiacalElongation(skybrightness.ZodiacalGeometry{
			DifferentialLongitude: angle.Deg(tc.dlon),
			EclipticLatitude:      angle.Deg(tc.beta),
		})

		if math.Abs(got.Degrees()-tc.want) > 1e-9 {
			t.Errorf("(%v, %v) gave %v deg, want %v", tc.dlon, tc.beta, got.Degrees(), tc.want)
		}
	}
}

func BenchmarkZodiacalLight(b *testing.B) {
	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	geom := skybrightness.ZodiacalGeometry{
		DifferentialLongitude: angle.Deg(90),
		EclipticLatitude:      angle.Deg(30),
		SunDistanceAU:         1,
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := skybrightness.ZodiacalRadiance(dst, grid, geom); err != nil {
			b.Fatal(err)
		}
	}
}
