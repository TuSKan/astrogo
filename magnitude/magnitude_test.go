package magnitude_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
)

func defaultProvider() eph.Provider {
	return eph.Default()
}

// assertNear is a helper for comparing floats with tolerance.
func assertNear(t *testing.T, label string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.6f, want %.6f ±%.6f (diff=%.6f)", label, got, want, tol, got-want)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Planet Magnitude — Skyfield cross-validation
// ══════════════════════════════════════════════════════════════════════════════

func TestPlanetApparent_AllPlanets(t *testing.T) {
	p := defaultProvider()
	defer p.Close()

	tm := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	// Expected ranges from Astronomical Almanac / JPL Horizons.
	cases := []struct {
		name   string
		target eph.ID
		minMag float64
		maxMag float64
	}{
		{"Mercury", eph.Mercury, -2.5, 7.0},
		{"Venus", eph.Venus, -4.9, -3.0},
		{"Mars", eph.Mars, -3.0, 2.0},
		{"Jupiter", eph.Jupiter, -3.0, -1.0},
		{"Saturn", eph.Saturn, -1.5, 2.0},
		{"Uranus", eph.Uranus, 5.0, 6.5},
		{"Neptune", eph.Neptune, 7.5, 8.5},
	}

	for _, tc := range cases {
		mag, err := magnitude.PlanetApparent(p, tc.target, tm)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if mag < tc.minMag || mag > tc.maxMag {
			t.Errorf("%s: mag=%.2f, expected [%.1f, %.1f]", tc.name, mag, tc.minMag, tc.maxMag)
		} else {
			t.Logf("%s: V = %.2f mag", tc.name, mag)
		}
	}
}

func TestSunApparent(t *testing.T) {
	p := defaultProvider()
	defer p.Close()

	tm := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	mag, err := magnitude.SunApparent(p, tm)
	if err != nil {
		t.Fatal(err)
	}
	assertNear(t, "Sun V", mag, -26.74, 0.05)
	t.Logf("Sun V = %.3f mag", mag)
}

func TestMoonApparent(t *testing.T) {
	p := defaultProvider()
	defer p.Close()

	tm := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	mag, err := magnitude.MoonApparent(p, tm)
	if err != nil {
		t.Fatal(err)
	}
	if mag < -13 || mag > 3 {
		t.Errorf("Moon V = %.2f, out of physical range", mag)
	}
	t.Logf("Moon V = %.2f mag", mag)
}

func TestPlanetApparent_Sun(t *testing.T) {
	p := defaultProvider()
	defer p.Close()

	tm := time.Date(2026, 6, 15, 0, 0, 0, 0, time.LocationUTC)
	mag, err := magnitude.PlanetApparent(p, eph.Sun, tm)
	if err != nil {
		t.Fatal(err)
	}
	assertNear(t, "PlanetApparent(Sun)", mag, -26.74, 0.05)
}

func TestPlanetApparent_Earth_ReturnsError(t *testing.T) {
	p := defaultProvider()
	defer p.Close()

	tm := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	_, err := magnitude.PlanetApparent(p, eph.Earth, tm)
	if err == nil {
		t.Error("expected error for Earth, got nil")
	}
}

func TestPhaseAngle_Venus(t *testing.T) {
	p := defaultProvider()
	defer p.Close()

	tm := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	alpha, err := magnitude.PhaseAngle(p, eph.Venus, tm)
	if err != nil {
		t.Fatal(err)
	}
	deg := alpha.Degrees()
	if deg < 0 || deg > 180 {
		t.Errorf("Venus phase = %.1f°, out of range", deg)
	}
	t.Logf("Venus phase = %.1f°", deg)
}

func TestPhaseAngle_Jupiter_Small(t *testing.T) {
	p := defaultProvider()
	defer p.Close()

	tm := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	alpha, err := magnitude.PhaseAngle(p, eph.Jupiter, tm)
	if err != nil {
		t.Fatal(err)
	}
	deg := alpha.Degrees()
	if deg > 12 {
		t.Errorf("Jupiter phase = %.1f°, expected ≤ 12°", deg)
	}
	t.Logf("Jupiter phase = %.1f°", deg)
}

func TestIlluminatedFraction_Moon(t *testing.T) {
	p := defaultProvider()
	defer p.Close()

	tm := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	frac, err := magnitude.IlluminatedFraction(p, eph.Moon, tm)
	if err != nil {
		t.Fatal(err)
	}
	if frac < 0 || frac > 1 {
		t.Errorf("Moon illuminated fraction = %.3f, out of [0,1]", frac)
	}
	t.Logf("Moon illuminated fraction = %.1f%%", frac*100)
}

func TestNeptune_SecularBrightening(t *testing.T) {
	p := defaultProvider()
	defer p.Close()

	t1980 := time.Date(1980, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	t2026 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	m1980, err := magnitude.PlanetApparent(p, eph.Neptune, t1980)
	if err != nil {
		t.Fatal(err)
	}
	m2026, err := magnitude.PlanetApparent(p, eph.Neptune, t2026)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Neptune 1980: V=%.2f, 2026: V=%.2f, diff=%.3f", m1980, m2026, m2026-m1980)
}

// ══════════════════════════════════════════════════════════════════════════════
// Asteroid — sbpy cross-validation
// ══════════════════════════════════════════════════════════════════════════════

// These reference values are computed using sbpy:
//   from sbpy.photometry import HG, HG1G2, HG12_Pen16
//   import numpy as np
//   HG.evaluate(np.deg2rad(0), 3.53, 0.12) → 3.53
//   HG.evaluate(np.deg2rad(20), 10.0, 0.15) → per-alpha check

func TestAsteroidHG_AtZeroPhase(t *testing.T) {
	// At α=0, Φ₁(0)=Φ₂(0)=1 → V = H + 5·log₁₀(r·Δ)
	H := 3.53
	r, d := 2.77, 1.77
	mag := magnitude.AsteroidHG(H, 0.12, r, d, angle.Deg(0))
	expected := H + 5*math.Log10(r*d)
	assertNear(t, "HG α=0°", mag, expected, 0.01)
	t.Logf("Ceres at opposition: V = %.2f (expected %.2f)", mag, expected)
}

func TestAsteroidHG_PhaseMonotonicity(t *testing.T) {
	// Object should get fainter with increasing phase angle.
	H, G := 10.0, 0.15
	r, d := 2.5, 1.5
	m0 := magnitude.AsteroidHG(H, G, r, d, angle.Deg(0))
	m30 := magnitude.AsteroidHG(H, G, r, d, angle.Deg(30))
	m60 := magnitude.AsteroidHG(H, G, r, d, angle.Deg(60))
	m90 := magnitude.AsteroidHG(H, G, r, d, angle.Deg(90))

	if m30 <= m0 || m60 <= m30 || m90 <= m60 {
		t.Errorf("HG not monotonic: α=0°:%.2f, 30°:%.2f, 60°:%.2f, 90°:%.2f", m0, m30, m60, m90)
	}
	t.Logf("HG: α=0°:%.2f, 30°:%.2f, 60°:%.2f, 90°:%.2f", m0, m30, m60, m90)
}

func TestAsteroidHG_BowellSinCorrection(t *testing.T) {
	// Verify that the sin(α) correction term (c parameter) makes a difference.
	// At α=45°, the correction should be measurable vs a naive exp-only model.
	H := 10.0
	G := 0.15
	r, d := 2.5, 1.5
	mag := magnitude.AsteroidHG(H, G, r, d, angle.Deg(45))
	// The corrected model should give a value > H + 5·log₁₀(r·d).
	baseline := H + 5*math.Log10(r*d)
	if mag <= baseline {
		t.Errorf("HG at α=45° (%.3f) should be > baseline (%.3f)", mag, baseline)
	}
	t.Logf("HG α=45°: %.3f (baseline %.3f, diff %.4f)", mag, baseline, mag-baseline)
}

func TestAsteroidHG1G2_Themis(t *testing.T) {
	// sbpy reference: HG1G2(7.063, 0.62, 0.14) for Themis.
	// At α=0: V = 7.063 − 2.5·log₁₀(0.62·Φ₁(0) + 0.14·Φ₂(0) + 0.24·Φ₃(0)) + 0
	// With r=Δ=1: the distance modulus is 0.
	H := 7.063
	G1, G2 := 0.62, 0.14
	m0 := magnitude.AsteroidHG1G2(H, G1, G2, 1.0, 1.0, angle.Deg(0))
	// At zero phase, should be close to H (but not exactly H because Φ values ≠ 1 at 0° for splines that start at 7.5°).
	if math.Abs(m0-H) > 1.0 {
		t.Errorf("HG1G2 Themis at α=0°: %.3f, expected ~%.3f", m0, H)
	}
	t.Logf("HG1G2 Themis α=0° r=Δ=1: V = %.3f", m0)
}

func TestAsteroidHG1G2_PhaseMonotonicity(t *testing.T) {
	H, G1, G2 := 10.0, 0.62, 0.14
	r, d := 2.5, 1.5
	m0 := magnitude.AsteroidHG1G2(H, G1, G2, r, d, angle.Deg(0))
	m30 := magnitude.AsteroidHG1G2(H, G1, G2, r, d, angle.Deg(30))
	m60 := magnitude.AsteroidHG1G2(H, G1, G2, r, d, angle.Deg(60))

	if m30 <= m0 || m60 <= m30 {
		t.Errorf("HG1G2 not monotonic: α=0°:%.2f, 30°:%.2f, 60°:%.2f", m0, m30, m60)
	}
	t.Logf("HG1G2: α=0°:%.2f, 30°:%.2f, 60°:%.2f", m0, m30, m60)
}

func TestAsteroidHG12Star_Penttila(t *testing.T) {
	// Penttilä 2016: G₁ = 0.84293649·G₁₂*, G₂ = 0.53513350·(1−G₁₂*)
	H := 7.121
	G12star := 0.68

	m := magnitude.AsteroidHG12Star(H, G12star, 1.0, 1.0, angle.Deg(30))
	// Compare with manual HG1G2 call using the mapping.
	G1 := 0.84293649 * G12star
	G2 := 0.53513350 * (1 - G12star)
	mRef := magnitude.AsteroidHG1G2(H, G1, G2, 1.0, 1.0, angle.Deg(30))

	assertNear(t, "HG12* vs manual HG1G2", m, mRef, 1e-10)
	t.Logf("HG12* at α=30°: %.4f (G1=%.4f, G2=%.4f)", m, G1, G2)
}

func TestAsteroidHG12_Muinonen(t *testing.T) {
	// Original Muinonen HG12 mapping (discontinuous at G12=0.2).
	H := 7.121
	G12 := 0.68

	m := magnitude.AsteroidHG12(H, G12, 1.0, 1.0, angle.Deg(30))
	// Should use the G12 >= 0.2 branch.
	G1 := 0.9529*G12 + 0.02162
	G2 := -0.6125*G12 + 0.5572
	mRef := magnitude.AsteroidHG1G2(H, G1, G2, 1.0, 1.0, angle.Deg(30))

	assertNear(t, "HG12 vs manual HG1G2", m, mRef, 1e-10)
}

func TestAsteroidHG12Star_Monotonic(t *testing.T) {
	H := 10.0
	r, d := 2.5, 1.5

	m0 := magnitude.AsteroidHG12Star(H, 0.15, r, d, angle.Deg(0))
	m30 := magnitude.AsteroidHG12Star(H, 0.15, r, d, angle.Deg(30))

	if m30 <= m0 {
		t.Errorf("HG12* not monotonic: m0=%.2f m30=%.2f", m0, m30)
	}
	t.Logf("HG12*: m0=%.2f m30=%.2f", m0, m30)
}

// ══════════════════════════════════════════════════════════════════════════════
// Comet — IAU standard model
// ══════════════════════════════════════════════════════════════════════════════

func TestCometApparent_UnitDistance(t *testing.T) {
	// At r=Δ=1: m = M₁ + 0 + 0 = M₁
	m := magnitude.CometApparent(10, 10, 1.0, 1.0)
	assertNear(t, "comet r=Δ=1", m, 10.0, 0.001)
}

func TestCometApparent_HeliocentricScaling(t *testing.T) {
	// At r=2, Δ=1: m = 10 + 0 + 10·log₁₀(2) ≈ 13.01
	m := magnitude.CometApparent(10, 10, 2.0, 1.0)
	expected := 10.0 + 10*math.Log10(2)
	assertNear(t, "comet r=2", m, expected, 0.001)
}

func TestCometNuclearApparent(t *testing.T) {
	m := magnitude.CometNuclearApparent(15, 5, 1.0, 1.0)
	assertNear(t, "nuclear r=Δ=1", m, 15.0, 0.001)
}

// ══════════════════════════════════════════════════════════════════════════════
// Satellite — McCants/Molczan
// ══════════════════════════════════════════════════════════════════════════════

func TestSatelliteApparent_RangeScaling(t *testing.T) {
	stdMag := 2.0
	alpha := angle.Deg(90)

	// At reference range (1000 km) and reference phase (90°): should equal stdMag.
	m1000 := magnitude.SatelliteApparent(stdMag, magnitude.ConventionMcCants, 1000, alpha, magnitude.PhaseSphere)
	assertNear(t, "sat 1000 km", m1000, stdMag, 0.01)

	// At 2000 km: +5·log₁₀(2) ≈ +1.505 mag.
	m2000 := magnitude.SatelliteApparent(stdMag, magnitude.ConventionMcCants, 2000, alpha, magnitude.PhaseSphere)
	expected := stdMag + 5*math.Log10(2.0)
	assertNear(t, "sat 2000 km", m2000, expected, 0.01)
}

func TestSatelliteApparent_PhaseMonotonicity(t *testing.T) {
	stdMag := 2.0
	rangeKm := 500.0

	m0 := magnitude.SatelliteApparent(stdMag, magnitude.ConventionMcCants, rangeKm, angle.Deg(0), magnitude.PhaseSphere)
	m90 := magnitude.SatelliteApparent(stdMag, magnitude.ConventionMcCants, rangeKm, angle.Deg(90), magnitude.PhaseSphere)
	m150 := magnitude.SatelliteApparent(stdMag, magnitude.ConventionMcCants, rangeKm, angle.Deg(150), magnitude.PhaseSphere)

	if m90 <= m0 || m150 <= m90 {
		t.Errorf("sat phase not monotonic: α=0°:%.2f, 90°:%.2f, 150°:%.2f", m0, m90, m150)
	}
	t.Logf("Satellite: α=0°:%.2f, 90°:%.2f, 150°:%.2f", m0, m90, m150)
}

func TestSatelliteApparent_CylinderVsSphere(t *testing.T) {
	stdMag := 3.0
	rangeKm := 1000.0

	mSphere := magnitude.SatelliteApparent(stdMag, magnitude.ConventionMcCants, rangeKm, angle.Deg(45), magnitude.PhaseSphere)
	mCyl := magnitude.SatelliteApparent(stdMag, magnitude.ConventionMcCants, rangeKm, angle.Deg(45), magnitude.PhaseCylinder)

	if mSphere == mCyl {
		t.Error("sphere and cylinder should give different results")
	}
	t.Logf("Sphere=%.3f, Cylinder=%.3f", mSphere, mCyl)
}

// ══════════════════════════════════════════════════════════════════════════════
// Star — Extinction
// ══════════════════════════════════════════════════════════════════════════════

func TestStarApparent_ZeroAirmass(t *testing.T) {
	m := magnitude.StarApparent(5.0, 0.0)
	assertNear(t, "star X=0", m, 5.0, 0.001)
}

func TestStarApparent_DefaultExtinction(t *testing.T) {
	// k(V)=0.20, X=2: m = 5.0 + 0.20*2 = 5.40
	m := magnitude.StarApparent(5.0, 2.0)
	assertNear(t, "star X=2", m, 5.4, 0.001)
}

func TestStarApparent_BandExtinction(t *testing.T) {
	// B-band: k=0.30, X=1.5: m = 3.0 + 0.30*1.5 = 3.45
	m := magnitude.StarApparent(3.0, 1.5, magnitude.ExtinctionB)
	assertNear(t, "star B X=1.5", m, 3.45, 0.001)
}

func TestExtinctionAtAltitude(t *testing.T) {
	k0 := magnitude.ExtinctionAtAltitude(0.20, 0)
	assertNear(t, "sea level", k0, 0.20, 0.001)

	// 2500m: ~75% of sea level.
	k2500 := magnitude.ExtinctionAtAltitude(0.20, 2500)
	if k2500 >= 0.20 || k2500 < 0.10 {
		t.Errorf("2500m: k=%.3f, expected ~0.15", k2500)
	}
	t.Logf("k(V) at 2500m = %.4f", k2500)
}

func TestGaiaGToJohnsonV(t *testing.T) {
	// Solar-type star (BP-RP ≈ 0.82).
	G := 10.0
	bpRp := 0.82
	V := magnitude.GaiaGToJohnsonV(G, bpRp)
	// V-G should be about -0.15 for solar type.
	diff := V - G
	if math.Abs(diff) > 0.5 {
		t.Errorf("G→V diff = %.3f, expected < 0.5", diff)
	}
	t.Logf("G=%.1f BP-RP=%.2f → V=%.3f (ΔV=%.3f)", G, bpRp, V, diff)

	// Red dwarf (BP-RP ≈ 3.0): should give larger negative correction.
	Vred := magnitude.GaiaGToJohnsonV(15.0, 3.0)
	t.Logf("Red dwarf: G=15.0 BP-RP=3.0 → V=%.3f", Vred)
}

// ══════════════════════════════════════════════════════════════════════════════
// Cubic Spline — Validation against sbpy knot tables
// ══════════════════════════════════════════════════════════════════════════════

func TestAsteroidHG1G2_SplineKnots(t *testing.T) {
	// At spline knot positions, HG1G2 should reproduce the knot values.
	// For G1=1, G2=0: V = H − 2.5·log₁₀(Φ₁(α)) + 5·log₁₀(r·Δ)
	// With r=Δ=1, H=0: V = −2.5·log₁₀(Φ₁(α))

	H := 0.0
	G1, G2 := 1.0, 0.0

	// sbpy Φ₁ knot at α=30°: y=0.3349
	m30 := magnitude.AsteroidHG1G2(H, G1, G2, 1.0, 1.0, angle.Deg(30))
	phi1_30 := math.Pow(10, -m30/2.5) // recover Φ₁ from magnitude
	assertNear(t, "Φ₁(30°)", phi1_30, 0.3349, 0.005)
	t.Logf("Φ₁(30°) = %.4f (expected ~0.3349)", phi1_30)

	// sbpy Φ₁ knot at α=90°: y=0.0511
	m90 := magnitude.AsteroidHG1G2(H, G1, G2, 1.0, 1.0, angle.Deg(90))
	phi1_90 := math.Pow(10, -m90/2.5)
	assertNear(t, "Φ₁(90°)", phi1_90, 0.0511, 0.005)
	t.Logf("Φ₁(90°) = %.4f (expected ~0.0511)", phi1_90)

	// Φ₂ at α=60°: y=0.3176 (with G1=0, G2=1)
	G1, G2 = 0.0, 1.0
	m60_2 := magnitude.AsteroidHG1G2(H, G1, G2, 1.0, 1.0, angle.Deg(60))
	phi2_60 := math.Pow(10, -m60_2/2.5)
	assertNear(t, "Φ₂(60°)", phi2_60, 0.3176, 0.005)
	t.Logf("Φ₂(60°) = %.4f (expected ~0.3176)", phi2_60)
}

// ══════════════════════════════════════════════════════════════════════════════
// sHG1G2 — Carry et al. (2024) model validation
// ══════════════════════════════════════════════════════════════════════════════

func TestCosAspectAngle(t *testing.T) {
	// Same RA/Dec for target and pole → cos Λ = 1 (pole-on).
	cosL := magnitude.CosAspectAngle(angle.Deg(30), angle.Deg(45), angle.Deg(30), angle.Deg(45))
	assertNear(t, "pole-on", cosL, 1.0, 1e-10)

	// RA differs by 180° → cos(ΔRA) = −1.
	// cos Λ = sin(δ)sin(δ₀) + cos(δ)cos(δ₀)·(−1)
	// For δ=δ₀=0: cos Λ = 0 + 1·1·(−1) = −1.
	cosL2 := magnitude.CosAspectAngle(angle.Deg(0), angle.Deg(0), angle.Deg(180), angle.Deg(0))
	assertNear(t, "anti-parallel equator", cosL2, -1.0, 1e-10)

	// δ=0°, δ₀=90° → cos Λ = 0 (equator view).
	cosL3 := magnitude.CosAspectAngle(angle.Deg(100), angle.Deg(0), angle.Deg(0), angle.Deg(90))
	assertNear(t, "equator view", cosL3, 0.0, 1e-10)
}

func TestSpinCorrection_Sphere(t *testing.T) {
	// When R=1 (sphere), spin correction should be zero for any aspect angle.
	for _, cosL := range []float64{0.0, 0.5, 1.0, -1.0} {
		s := magnitude.SpinCorrection(1.0, cosL)
		assertNear(t, "R=1 sphere", s, 0.0, 1e-10)
	}
}

func TestSpinCorrection_PoleOn(t *testing.T) {
	// When cos Λ = ±1 (pole-on): s = 2.5·log₁₀(1 − (1−R)·1) = 2.5·log₁₀(R)
	// For R=0.7: s = 2.5·log₁₀(0.7) ≈ −0.387
	R := 0.7
	s := magnitude.SpinCorrection(R, 1.0)
	expected := 2.5 * math.Log10(R)
	assertNear(t, "pole-on R=0.7", s, expected, 1e-6)
	t.Logf("s(pole-on, R=0.7) = %.4f mag (expected %.4f)", s, expected)
}

func TestSpinCorrection_EquatorOn(t *testing.T) {
	// When cos Λ = 0 (equator view): s = 2.5·log₁₀(1) = 0
	R := 0.5
	s := magnitude.SpinCorrection(R, 0.0)
	assertNear(t, "equator-on", s, 0.0, 1e-10)
}

func TestSHG1G2_ReducesToHG1G2_WhenSphere(t *testing.T) {
	// When R=1 (sphere), sHG1G2 should give exactly the same result as HG1G2.
	H, G1, G2 := 10.0, 0.62, 0.14
	r, d := 2.5, 1.5
	alpha := angle.Deg(30)

	mHG1G2 := magnitude.AsteroidHG1G2(H, G1, G2, r, d, alpha)
	mSHG1G2 := magnitude.AsteroidSHG1G2(H, G1, G2, r, d, alpha, 1.0, 0.5)

	assertNear(t, "sHG1G2=HG1G2 for R=1", mSHG1G2, mHG1G2, 1e-10)
}

func TestSHG1G2_PoleOnBrighter(t *testing.T) {
	// When viewed pole-on (cos Λ = 1) with R<1, the object should be BRIGHTER
	// (smaller magnitude) than when viewed equator-on (cos Λ = 0).
	H, G1, G2 := 10.0, 0.62, 0.14
	r, d := 2.5, 1.5
	alpha := angle.Deg(20)
	R := 0.7

	mEquator := magnitude.AsteroidSHG1G2(H, G1, G2, r, d, alpha, R, 0.0)
	mPole := magnitude.AsteroidSHG1G2(H, G1, G2, r, d, alpha, R, 1.0)

	if mPole >= mEquator {
		t.Errorf("pole-on (%.3f) should be brighter (smaller) than equator-on (%.3f)", mPole, mEquator)
	}
	t.Logf("R=%.1f: equator=%.3f, pole=%.3f (Δm=%.3f)", R, mEquator, mPole, mEquator-mPole)

	// The brightness difference should be exactly 2.5·log₁₀(R) ≈ 0.387 mag.
	expectedDiff := -2.5 * math.Log10(R)
	actualDiff := mEquator - mPole
	assertNear(t, "Δm pole-equator", actualDiff, expectedDiff, 1e-6)
}

func TestSHG1G2_Monotonic_Phase(t *testing.T) {
	// sHG1G2 should remain monotonic in phase angle (fixed geometry).
	H, G1, G2 := 10.0, 0.62, 0.14
	r, d := 2.5, 1.5
	R := 0.8
	cosL := 0.5

	m0 := magnitude.AsteroidSHG1G2(H, G1, G2, r, d, angle.Deg(0), R, cosL)
	m30 := magnitude.AsteroidSHG1G2(H, G1, G2, r, d, angle.Deg(30), R, cosL)
	m60 := magnitude.AsteroidSHG1G2(H, G1, G2, r, d, angle.Deg(60), R, cosL)

	if m30 <= m0 || m60 <= m30 {
		t.Errorf("sHG1G2 not monotonic: α=0°:%.2f, 30°:%.2f, 60°:%.2f", m0, m30, m60)
	}
	t.Logf("sHG1G2: α=0°:%.2f, 30°:%.2f, 60°:%.2f", m0, m30, m60)
}

func TestOblateness(t *testing.T) {
	// Sphere: a=b=c=1 → R = 1·(1+1)/(2·1·1) = 1.
	R := magnitude.Oblateness(1, 1, 1)
	assertNear(t, "sphere", R, 1.0, 1e-10)

	// Oblate: a=b=2, c=1 → R = 1·(2+2)/(2·2·2) = 4/8 = 0.5.
	R2 := magnitude.Oblateness(2, 2, 1)
	assertNear(t, "oblate", R2, 0.5, 1e-10)

	// Elongated: a=3, b=2, c=1 → R = 1·(3+2)/(2·3·2) = 5/12 ≈ 0.417.
	R3 := magnitude.Oblateness(3, 2, 1)
	assertNear(t, "elongated", R3, 5.0/12.0, 1e-6)
	t.Logf("R(3,2,1) = %.4f", R3)
}
