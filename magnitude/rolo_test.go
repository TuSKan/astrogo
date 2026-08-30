package magnitude_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/magnitude"
)

// reflectanceAt is a small helper: the whole 32-band vector for one geometry.
func reflectanceAt(t *testing.T, geom magnitude.ROLOGeometry) []float64 {
	t.Helper()

	dst := make([]float64, len(magnitude.ROLOBands()))
	if err := magnitude.ROLOReflectance(dst, geom); err != nil {
		t.Fatalf("ROLOReflectance(%+v): %v", geom, err)
	}

	return dst
}

// The band list must be the published one: 32 bands, 350 to 2383.6 nm,
// strictly ascending. A silently reordered or truncated table would make
// every downstream spectrum wrong in a way no physical test would catch.
func TestROLOBands(t *testing.T) {
	t.Parallel()

	bands := magnitude.ROLOBands()

	if len(bands) != 32 {
		t.Fatalf("got %d bands, want 32", len(bands))
	}

	if bands[0] != 350.0 {
		t.Errorf("first band = %v nm, want 350.0", bands[0])
	}

	if bands[31] != 2383.6 {
		t.Errorf("last band = %v nm, want 2383.6", bands[31])
	}

	for i := 1; i < len(bands); i++ {
		if bands[i] <= bands[i-1] {
			t.Errorf("band %d (%v nm) does not exceed band %d (%v nm)", i, bands[i], i-1, bands[i-1])
		}
	}

	// The returned slice must be a copy — a caller that sorts or scales it
	// must not corrupt the model for the rest of the process.
	bands[0] = 0

	if again := magnitude.ROLOBands(); again[0] != 350.0 {
		t.Errorf("ROLOBands returned an aliased slice: first band is now %v nm", again[0])
	}
}

// Eq. 10 evaluated by hand for the 665.1 nm band, independently of the
// implementation's loop structure, with every term written out. This is the
// test that would catch a coefficient assigned to the wrong term, an odd
// power dropped from the solar-longitude series, or the radians/degrees
// split in the phase angle applied the same way to both groups.
func TestROLOReflectanceEquation10(t *testing.T) {
	t.Parallel()

	const (
		gDeg = 30.0
		pDeg = -25.0 // solar longitude, waxing
		tDeg = 1.2   // libration latitude
		lDeg = -3.4  // libration longitude
	)

	got := reflectanceAt(t, magnitude.ROLOGeometry{
		PhaseAngle:         angle.Deg(gDeg),
		SolarLongitude:     angle.Deg(pDeg),
		LibrationLatitude:  angle.Deg(tDeg),
		LibrationLongitude: angle.Deg(lDeg),
	})

	// Table 4, 665.1 nm — index 12.
	var (
		a0, a1, a2, a3 = -1.88914, -1.58096, 0.30477, -0.17908
		b1, b2, b3     = 0.04415, 0.00983, -0.00389
		d1, d2, d3     = 0.37141, -0.13514, 0.01248
	)

	// Table 5's eight wavelength-independent coefficients.
	var (
		c1, c2, c3, c4 = 0.0003, -0.0013, 0.0010, 0.0006
		p1, p2, p3, p4 = 4.06, 12.88, -30.59, 16.75
	)

	g := gDeg * math.Pi / 180 // radians for the polynomial
	p := pDeg * math.Pi / 180 // radians for the solar-longitude series

	lnA := a0 + a1*g + a2*g*g + a3*g*g*g
	lnA += b1*p + b2*(p*p*p) + b3*(p*p*p*p*p)
	lnA += c1*tDeg + c2*lDeg + c3*p*tDeg + c4*p*lDeg
	lnA += d1*math.Exp(-gDeg/p1) + d2*math.Exp(-gDeg/p2) + d3*math.Cos((gDeg-p3)/p4)

	want := math.Exp(lnA)

	if rel := math.Abs(got[12]-want) / want; rel > 1e-12 {
		t.Errorf("A(665.1 nm) = %.10g, want %.10g (relative %.2e)", got[12], want, rel)
	}
}

// The phase angle enters the polynomial in radians and the exponential and
// cosine terms in degrees. If an implementation used one unit throughout, the
// hand-computed value above would still be reproducible by an equally wrong
// reference, so this asserts the split directly: swapping the units must
// change the answer by a large factor, not a rounding.
func TestROLOReflectanceUsesBothAngleUnits(t *testing.T) {
	t.Parallel()

	geom := magnitude.ROLOGeometry{PhaseAngle: angle.Deg(45)}
	got := reflectanceAt(t, geom)[12]

	gRad := 45 * math.Pi / 180

	// The all-radians reading: p1 = 4.06 rad would make the exponentials
	// nearly 1 across the whole valid phase range.
	allRad := math.Exp(-1.88914 - 1.58096*gRad + 0.30477*gRad*gRad - 0.17908*gRad*gRad*gRad +
		0.37141*math.Exp(-gRad/4.06) - 0.13514*math.Exp(-gRad/12.88) + 0.01248*math.Cos((gRad+30.59)/16.75))

	if rel := math.Abs(got-allRad) / allRad; rel < 0.1 {
		t.Errorf("the all-radians reading gives %.6g against the implementation's %.6g "+
			"(relative %.3f) — too close to distinguish the unit convention", allRad, got, rel)
	}
}

// The Moon fades as it moves away from full. This is the most basic thing the
// model must get right, and the sign of a1 alone does not guarantee it once
// the exponential opposition terms are added.
func TestROLOReflectanceFadesWithPhase(t *testing.T) {
	t.Parallel()

	const band = 12 // 665.1 nm

	prev := math.Inf(1)

	for _, gDeg := range []float64{2, 10, 20, 40, 60, 80, 95} {
		a := reflectanceAt(t, magnitude.ROLOGeometry{PhaseAngle: angle.Deg(gDeg)})[band]

		if a >= prev {
			t.Errorf("A at %v deg = %.6g, expected less than %.6g at the previous phase", gDeg, a, prev)
		}

		if a <= 0 {
			t.Errorf("A at %v deg = %.6g, must be positive", gDeg, a)
		}

		prev = a
	}
}

// The opposition surge: reflectance rises much faster near full Moon than a
// linear extrapolation of the mid-phase slope predicts. That non-linearity is
// what the d1 exp(-g/4.06) term exists for, and it is the reason a smooth
// phase curve like Allen's polynomial cannot stand in for this model.
func TestROLOReflectanceOppositionSurge(t *testing.T) {
	t.Parallel()

	const band = 12

	at := func(gDeg float64) float64 {
		return reflectanceAt(t, magnitude.ROLOGeometry{PhaseAngle: angle.Deg(gDeg)})[band]
	}

	nearFull := (at(2) - at(6)) / 4    // per degree, inside the surge
	midPhase := (at(40) - at(44)) / 4  // per degree, outside it
	scaled := nearFull / at(4) * 4     // fractional slope near full
	scaledMid := midPhase / at(42) * 4 // fractional slope at mid phase

	if scaled <= scaledMid {
		t.Errorf("fractional slope near full Moon (%.4g) does not exceed mid-phase (%.4g): "+
			"the opposition surge is missing", scaled, scaledMid)
	}
}

// The Moon is red: its reflectance climbs from the near-UV into the
// near-infrared. Kieffer & Stone's a0 column shows this directly, rising from
// -2.675 at 350 nm to -1.084 at 2383.6 nm.
//
// The rise is *not* monotonic band to band, and asserting that it is would be
// testing an assumption rather than the model. Each ROLO band is an
// independent fit to its own filter, so neighbouring pairs invert where the
// filters are close together and the fits differ by more than the underlying
// spectral slope: 355.1 nm falls below 350.0 nm, 414.4 below 412.3, and
// 553.8 below 549.1, all as published. What is real is the trend across
// well-separated bands, which is what this checks.
func TestROLOReflectanceRisesWithWavelength(t *testing.T) {
	t.Parallel()

	a := reflectanceAt(t, magnitude.ROLOGeometry{
		PhaseAngle:     angle.Deg(30),
		SolarLongitude: angle.Deg(-30),
	})

	// Indices of 350.0, 475.0, 665.1, 865.3, 1243.2 and 2383.6 nm.
	widelySeparated := []int{0, 7, 12, 18, 25, 31}

	for k := 1; k < len(widelySeparated); k++ {
		lo, hi := widelySeparated[k-1], widelySeparated[k]
		if a[hi] <= a[lo] {
			t.Errorf("band %d is not brighter than band %d (%.5g vs %.5g)", hi, lo, a[hi], a[lo])
		}
	}

	// Reflectance stays in a physically sensible range for a very dark
	// surface at every band and this geometry — the Moon's geometric albedo
	// is around 0.12 in V and it is brighter in the infrared.
	for i, v := range a {
		if v <= 0.005 || v >= 0.5 {
			t.Errorf("band %d (%v nm): A = %.5g, outside a plausible lunar range", i, magnitude.ROLOBands()[i], v)
		}
	}
}

// The one check in this file against a number from outside Kieffer & Stone.
//
// At zero phase, disk-equivalent reflectance is by definition the geometric
// albedo — both are the body's brightness relative to a Lambert disk of the
// same angular size under full illumination. The Moon's V-band geometric
// albedo is independently known to be about 0.12, so evaluating the model at
// the smallest phase angle it was fitted over must land near that, at the
// band nearest V. Nothing in the coefficient table forces this: a transposed
// column, a radians/degrees slip or a dropped exponential would all still
// pass the shape tests above and fail here.
//
// The same evaluation must also reproduce the Moon's red slope — it is
// roughly twice as reflective at 2.4 microns as in V, which is why a
// V-calibrated moonlight model cannot be extrapolated into the near-infrared.
func TestROLOReflectanceMatchesKnownGeometricAlbedo(t *testing.T) {
	t.Parallel()

	a := reflectanceAt(t, magnitude.ROLOGeometry{
		PhaseAngle:     angle.Deg(magnitude.ROLOMinPhaseDeg),
		SolarLongitude: angle.Deg(-magnitude.ROLOMinPhaseDeg),
	})

	const vBand = 11 // 553.8 nm, the ROLO band closest to Johnson V

	if v := a[vBand]; v < 0.10 || v > 0.16 {
		t.Errorf("A(553.8 nm) near full Moon = %.4f, want the lunar geometric albedo, about 0.12", v)
	}

	if ratio := a[31] / a[vBand]; ratio < 1.8 || ratio > 3.0 {
		t.Errorf("A(2383.6 nm)/A(553.8 nm) = %.2f, want roughly 2.4 for the lunar red slope", ratio)
	}
}

// Waxing and waning Moons of the same phase angle are not equally bright.
// The whole reason the model carries a solar longitude separate from the
// phase angle is that the two hemispheres differ; a model symmetric in phase
// would return identical values here.
func TestROLOReflectanceWaxingWaningAsymmetry(t *testing.T) {
	t.Parallel()

	const band = 12

	waxing := reflectanceAt(t, magnitude.ROLOGeometry{
		PhaseAngle: angle.Deg(45), SolarLongitude: angle.Deg(-45),
	})[band]

	waning := reflectanceAt(t, magnitude.ROLOGeometry{
		PhaseAngle: angle.Deg(45), SolarLongitude: angle.Deg(45),
	})[band]

	if waxing == waning {
		t.Fatal("waxing and waning give identical reflectance: the solar-longitude terms are inert")
	}

	if rel := math.Abs(waxing-waning) / waning; rel < 0.01 || rel > 0.5 {
		t.Errorf("waxing/waning difference is %.1f%%, expected a few per cent to tens of per cent", rel*100)
	}
}

// The libration terms are the smallest in the model. Table 5 puts their
// combined effect on ln A at about 0.03 at most, so a caller who cannot
// supply lunar orientation and passes zeroes is making a bounded error. This
// pins that bound so a future coefficient change cannot silently make the
// omission significant.
func TestROLOLibrationIsASmallCorrection(t *testing.T) {
	t.Parallel()

	const band = 12

	base := magnitude.ROLOGeometry{PhaseAngle: angle.Deg(30), SolarLongitude: angle.Deg(-30)}

	without := reflectanceAt(t, base)[band]

	tipped := base
	tipped.LibrationLatitude = angle.Deg(7) // near the maximum real libration
	tipped.LibrationLongitude = angle.Deg(8)

	with := reflectanceAt(t, tipped)[band]

	lnDiff := math.Abs(math.Log(with) - math.Log(without))
	if lnDiff > 0.05 {
		t.Errorf("extreme libration changes ln A by %.4f, expected under 0.05", lnDiff)
	}

	if lnDiff == 0 {
		t.Error("libration made no difference at all: the c terms are inert")
	}
}

// Outside the fitted range the model still answers, but says so. A silent
// extrapolation past 97 degrees is how a crescent-Moon query turns into an
// unflagged wrong number.
func TestROLOReflectancePhaseRange(t *testing.T) {
	t.Parallel()

	dst := make([]float64, 32)

	for _, gDeg := range []float64{0, 1.0, 120, 179} {
		err := magnitude.ROLOReflectance(dst, magnitude.ROLOGeometry{PhaseAngle: angle.Deg(gDeg)})
		if !errors.Is(err, magnitude.ErrROLOPhaseRange) {
			t.Errorf("phase %v deg: err = %v, want ErrROLOPhaseRange", gDeg, err)
		}

		if dst[12] <= 0 || math.IsNaN(dst[12]) {
			t.Errorf("phase %v deg: reflectance %v, want a usable value alongside the error", gDeg, dst[12])
		}
	}

	// Just inside both edges there must be no error at all.
	for _, gDeg := range []float64{1.55, 50, 97} {
		if err := magnitude.ROLOReflectance(dst, magnitude.ROLOGeometry{PhaseAngle: angle.Deg(gDeg)}); err != nil {
			t.Errorf("phase %v deg is inside the fitted range but returned %v", gDeg, err)
		}
	}
}

// Sign of the phase angle must not matter — Kieffer & Stone's g is an
// absolute value, and the waxing/waning distinction lives in the solar
// longitude instead.
func TestROLOReflectanceIgnoresPhaseSign(t *testing.T) {
	t.Parallel()

	pos := reflectanceAt(t, magnitude.ROLOGeometry{PhaseAngle: angle.Deg(40)})
	neg := reflectanceAt(t, magnitude.ROLOGeometry{PhaseAngle: angle.Deg(-40)})

	for i := range pos {
		if pos[i] != neg[i] {
			t.Fatalf("band %d: +40 deg gives %v, -40 deg gives %v", i, pos[i], neg[i])
		}
	}
}

func TestROLOReflectanceBandCount(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, 31, 33} {
		err := magnitude.ROLOReflectance(make([]float64, n), magnitude.ROLOGeometry{PhaseAngle: angle.Deg(30)})
		if !errors.Is(err, magnitude.ErrROLOBandCount) {
			t.Errorf("%d slots: err = %v, want ErrROLOBandCount", n, err)
		}
	}
}

// The irradiance conversion is inverse-square in both legs and linear in
// reflectance and in the solar spectrum. Checking it against a hand-computed
// value for one band also pins the Omega/pi factor, which is the part a
// reader is most likely to drop.
func TestROLOIrradiance(t *testing.T) {
	t.Parallel()

	const (
		refl  = 0.12
		solar = 1500.0 // W m^-2 nm^-1 at 1 AU, an arbitrary round number
	)

	dst := make([]float64, 1)
	if err := magnitude.ROLOIrradiance(dst, []float64{refl}, []float64{solar}, 1, magnitude.ROLOStandardDistanceKM); err != nil {
		t.Fatalf("ROLOIrradiance: %v", err)
	}

	want := refl * solar * magnitude.ROLOSolidAngleSR / math.Pi

	if rel := math.Abs(dst[0]-want) / want; rel > 1e-14 {
		t.Errorf("E = %.10g, want %.10g", dst[0], want)
	}

	// Twice the distance to the Moon is a quarter of the irradiance.
	far := make([]float64, 1)
	if err := magnitude.ROLOIrradiance(far, []float64{refl}, []float64{solar}, 1, 2*magnitude.ROLOStandardDistanceKM); err != nil {
		t.Fatalf("ROLOIrradiance: %v", err)
	}

	if rel := math.Abs(far[0]-dst[0]/4) / (dst[0] / 4); rel > 1e-14 {
		t.Errorf("doubling the Moon distance gave %.10g, want %.10g", far[0], dst[0]/4)
	}

	// And the Sun-Moon leg scales the same way.
	aphelion := make([]float64, 1)
	if err := magnitude.ROLOIrradiance(aphelion, []float64{refl}, []float64{solar}, 2, magnitude.ROLOStandardDistanceKM); err != nil {
		t.Fatalf("ROLOIrradiance: %v", err)
	}

	if rel := math.Abs(aphelion[0]-dst[0]/4) / (dst[0] / 4); rel > 1e-14 {
		t.Errorf("doubling the Sun distance gave %.10g, want %.10g", aphelion[0], dst[0]/4)
	}
}

func TestROLOIrradianceRejectsBadInput(t *testing.T) {
	t.Parallel()

	one := []float64{1}

	if err := magnitude.ROLOIrradiance(make([]float64, 2), one, one, 1, 1); !errors.Is(err, magnitude.ErrROLOBandCount) {
		t.Errorf("mismatched lengths: err = %v, want ErrROLOBandCount", err)
	}

	for _, d := range [][2]float64{{0, 1}, {1, 0}, {-1, 1}, {1, -1}} {
		if err := magnitude.ROLOIrradiance(one, one, one, d[0], d[1]); !errors.Is(err, magnitude.ErrROLODistance) {
			t.Errorf("distances %v: err = %v, want ErrROLODistance", d, err)
		}
	}
}

// Aliasing dst onto reflectance is documented as allowed, so it has to work.
func TestROLOIrradianceAliases(t *testing.T) {
	t.Parallel()

	buf := []float64{0.12, 0.13}
	solar := []float64{1500, 1600}

	if err := magnitude.ROLOIrradiance(buf, buf, solar, 1, magnitude.ROLOStandardDistanceKM); err != nil {
		t.Fatalf("ROLOIrradiance: %v", err)
	}

	want := 0.13 * 1600 * magnitude.ROLOSolidAngleSR / math.Pi
	if rel := math.Abs(buf[1]-want) / want; rel > 1e-14 {
		t.Errorf("aliased result = %.10g, want %.10g", buf[1], want)
	}
}

func BenchmarkROLOReflectance(b *testing.B) {
	dst := make([]float64, 32)
	geom := magnitude.ROLOGeometry{PhaseAngle: angle.Deg(30), SolarLongitude: angle.Deg(-30)}

	b.ReportAllocs()

	for b.Loop() {
		if err := magnitude.ROLOReflectance(dst, geom); err != nil {
			b.Fatal(err)
		}
	}
}
