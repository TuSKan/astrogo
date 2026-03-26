package constants_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/constants"
)

// tol is the relative tolerance used throughout these tests.
// Float64 has ~15–16 significant decimal digits, so 1e-14 is a tight but
// achievable threshold for derived relationships between compiled constants.
const tol = 1e-14

func near(a, b float64) bool {
	if b == 0 {
		return math.Abs(a) <= tol
	}
	return math.Abs(a-b)/math.Abs(b) <= tol
}

// ── Physical constants ────────────────────────────────────────────────────────

func TestSpeedOfLight_exact(t *testing.T) {
	// The SI definition is exact: c = 299 792 458 m/s.
	const want = 299_792_458.0
	if constants.SpeedOfLight != want {
		t.Errorf("SpeedOfLight = %v, want exact %v", constants.SpeedOfLight, want)
	}
}

// ── Astronomical constants ────────────────────────────────────────────────────

func TestAstronomicalUnit_range(t *testing.T) {
	// IAU 2012 value is 1.495978707 × 10¹¹ m (exact by definition).
	const want = 1.495_978_707e11
	if constants.AstronomicalUnit != want {
		t.Errorf("AstronomicalUnit = %v, want %v", constants.AstronomicalUnit, want)
	}
}

func TestJulianDaySeconds_exact(t *testing.T) {
	// 1 Julian day = 86400 seconds, exact by definition.
	const want = 86400.0
	if constants.JulianDaySeconds != want {
		t.Errorf("JulianDaySeconds = %v, want exact %v", constants.JulianDaySeconds, want)
	}
}

func TestMeanEarthRadius_range(t *testing.T) {
	// IAU 2015 nominal volumetric mean radius: 6 371 000 m exactly.
	const want = 6_371_000.0
	if constants.MeanEarthRadius != want {
		t.Errorf("MeanEarthRadius = %v, want %v", constants.MeanEarthRadius, want)
	}
	// Sanity: must be between 6350 km and 6400 km.
	if constants.MeanEarthRadius < 6.35e6 || constants.MeanEarthRadius > 6.40e6 {
		t.Errorf("MeanEarthRadius = %v m is outside the expected [6.35e6, 6.40e6] range",
			constants.MeanEarthRadius)
	}
}

// ── WGS84 ─────────────────────────────────────────────────────────────────────

func TestWGS84SemiMajorAxis_exact(t *testing.T) {
	const want = 6_378_137.0
	if constants.WGS84SemiMajorAxis != want {
		t.Errorf("WGS84SemiMajorAxis = %v, want exact %v", constants.WGS84SemiMajorAxis, want)
	}
}

func TestWGS84Flattening_range(t *testing.T) {
	f := constants.WGS84Flattening
	// Must be 1/298.257... ≈ 0.003352810664
	if f < 0.003352 || f > 0.003353 {
		t.Errorf("WGS84Flattening = %.10f, expected ~1/298.257", f)
	}
}

func TestWGS84Flattening_inverse(t *testing.T) {
	// WGS84Flattening == 1 / WGS84InverseFlattening
	got := constants.WGS84Flattening
	want := 1.0 / constants.WGS84InverseFlattening
	if got != want {
		t.Errorf("WGS84Flattening = %v, want 1/WGS84InverseFlattening = %v", got, want)
	}
}

// The WGS84 polar radius b = a(1-f) must be smaller than a.
func TestWGS84_PolarRadius_SmallerThan_SemiMajor(t *testing.T) {
	a := constants.WGS84SemiMajorAxis
	f := constants.WGS84Flattening
	b := a * (1 - f)
	if b >= a {
		t.Errorf("WGS84 polar radius b = %v must be < a = %v", b, a)
	}
	// Known value: b ≈ 6 356 752.3142 m
	if b < 6.356e6 || b > 6.357e6 {
		t.Errorf("WGS84 polar radius b = %v m, expected ~6.3568e6", b)
	}
}

// ── Angular conversion constants ──────────────────────────────────────────────

func TestRadiansPerDegree_DegreesPerRadian_inverse(t *testing.T) {
	// Rad/deg and deg/rad must be multiplicative inverses.
	product := constants.RadiansPerDegree * constants.DegreesPerRadian
	if !near(product, 1.0) {
		t.Errorf("RadiansPerDegree × DegreesPerRadian = %.16g, want 1 (tol %g)", product, tol)
	}
}

func TestRadiansPerDegree_value(t *testing.T) {
	// π/180 ≈ 0.017453292519943295
	want := math.Pi / 180
	if !near(constants.RadiansPerDegree, want) {
		t.Errorf("RadiansPerDegree = %.16g, want %.16g", constants.RadiansPerDegree, want)
	}
}

func TestDegreesPerRadian_value(t *testing.T) {
	want := 180 / math.Pi
	if !near(constants.DegreesPerRadian, want) {
		t.Errorf("DegreesPerRadian = %.16g, want %.16g", constants.DegreesPerRadian, want)
	}
}

func TestArcSecondsPerRadian_is_3600xDegreesPerRadian(t *testing.T) {
	// ArcSecondsPerRadian == 3600 × DegreesPerRadian — exact within the package.
	want := 3600 * constants.DegreesPerRadian
	if constants.ArcSecondsPerRadian != want {
		t.Errorf("ArcSecondsPerRadian = %v, want 3600×DegreesPerRadian = %v",
			constants.ArcSecondsPerRadian, want)
	}
}

func TestArcSecondsPerRadian_knownValue(t *testing.T) {
	// 1 radian ≈ 206 264.806 arcsec (standard reference value).
	const knownLo, knownHi = 206264.0, 206265.5
	v := constants.ArcSecondsPerRadian
	if v < knownLo || v > knownHi {
		t.Errorf("ArcSecondsPerRadian = %v, expected in [%v, %v]", v, knownLo, knownHi)
	}
}

func TestFullCircle_radians(t *testing.T) {
	// 360 degrees must equal 2π radians under the conversion factor.
	got := 360 * constants.RadiansPerDegree
	want := 2 * math.Pi
	if !near(got, want) {
		t.Errorf("360 × RadiansPerDegree = %.16g, want 2π = %.16g", got, want)
	}
}

func TestRightAngle_radians(t *testing.T) {
	// 90 degrees must equal π/2 radians.
	got := 90 * constants.RadiansPerDegree
	want := math.Pi / 2
	if !near(got, want) {
		t.Errorf("90 × RadiansPerDegree = %.16g, want π/2 = %.16g", got, want)
	}
}
