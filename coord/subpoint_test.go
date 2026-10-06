package coord_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// TestSubPointIsOverhead holds SubPoint to its definition through the
// transform core: an observer standing at the point returned sees the
// direction at the zenith.
//
// # Why this and not the latitude
//
// This test used to assert that the sub-point's latitude is the GCRS
// declination of the direction, at any epoch — "declination is
// GAST-invariant". That was the defect, written down as the property: it
// holds only if the rotation into the Earth-fixed frame is Earth rotation
// alone, which is what SubPoint did, and is wrong for a GCRS vector by the
// precession and nutation since J2000. The sub-point latitude is the
// declination referred to the true equator of date, and the only way to state
// that without restating SubPoint is to observe from the point it returns.
//
// [coord.Context.GeocentricToObserved] is that observation: the ICRS→ITRS
// matrix NewContext builds, the rotation into the observer's horizon, and
// diurnal aberration, with refraction off. Diurnal aberration is the floor:
// 0.32 arcsec times the cosine of latitude, eastward, which a point directly
// beneath the direction cannot remove. Measured, the worst case here is 0.319
// arcsec from the zenith. With the rotation GAST alone it was 10 to 13 arcsec
// at J2000, and up to 1236 in 2026 and 2315 in 2050.
func TestSubPointIsOverhead(t *testing.T) {
	t.Parallel()

	const tolArcsec = 0.5 // diurnal aberration's 0.32 arcsec at the equator, with headroom

	var worst float64

	directions := []vector.Vec3{
		vector.V3(0.6, 0.3, 0.5),
		vector.V3(-0.2, 0.9, -0.4),
		vector.V3(0.1, -0.7, 0.05),
	}

	for _, jd := range []float64{
		2451545.0, // J2000.0, where precession is zero and nutation is not
		2455197.5, // 2010
		2461318.0, // 2026
		2469807.5, // 2050
	} {
		tm := time.FromJD(jd, time.UTC)

		for _, dir := range directions {
			geo, err := coord.SubPoint(dir, tm)
			if err != nil {
				t.Fatalf("SubPoint at JD %.1f: %v", jd, err)
			}

			// Far enough that the observer's 6378 km offset from the
			// geocenter subtends nothing: the direction is all that matters.
			far := dir.MulScalar(1e9 / dir.Norm())

			ctx := coord.NewContext(tm, geo, atmosphere.Refraction{Pressure: 0})
			zd := 90*3600 - ctx.GeocentricToObserved(far).Alt().Degrees()*3600
			worst = max(worst, zd)

			if zd > tolArcsec {
				t.Errorf("JD %.1f, direction %v: seen from the sub-point (lat %.6f°, lon %.6f°) it is "+
					"%.3f arcsec from the zenith, want under %.1f",
					jd, dir, geo.Lat().Degrees(), geo.Lon().Degrees(), zd, tolArcsec)
			}
		}
	}

	t.Logf("worst zenith distance at the sub-point: %.3f arcsec", worst)
}

// TestSubPoint_ZeroVector confirms the degenerate zero-length input is
// rejected rather than silently returning (0,0).
func TestSubPoint_ZeroVector(t *testing.T) {
	_, err := coord.SubPoint(vector.Vec3{}, time.FromJD(2451545.0, time.UTC))
	if !errors.Is(err, coord.ErrZeroVector) {
		t.Fatalf("SubPoint(zero vector) error = %v, want ErrZeroVector", err)
	}
}

// TestSubPoint_PoleIsLongitudeIndependent confirms a body directly over
// the pole yields a geodetic latitude of exactly ±90°, matching the fact
// that longitude is undefined at a pole (the underlying FromUnitVector's
// atan2(0,0)=0 convention is acceptable there).
//
// The pole is Earth's, carried into GCRS by the Context's own ITRS→ICRS
// rotation. It used to be the GCRS z-axis, which is not where Earth's pole
// points: frame bias and nutation put it 8 arcsec away at J2000, and
// precession moves it 20 arcsec a year from there. That test passed because
// SubPoint ignored both.
func TestSubPoint_PoleIsLongitudeIndependent(t *testing.T) {
	tm := time.FromJD(2451545.0, time.UTC)

	site, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	ctx := coord.NewContext(tm, site, atmosphere.Refraction{Pressure: 0})

	for _, tc := range []struct {
		name    string
		itrsZ   float64
		wantLat float64
	}{
		{"north", 1, 90},
		{"south", -1, -90},
	} {
		pole := ctx.ITRSToICRS(vector.V3(0, 0, tc.itrsZ))

		geo, err := coord.SubPoint(pole, tm)
		if err != nil {
			t.Fatalf("SubPoint(%s): %v", tc.name, err)
		}

		if math.Abs(geo.Lat().Degrees()-tc.wantLat) > 1e-9 {
			t.Errorf("%s pole Lat = %v, want %v°", tc.name, geo.Lat().Degrees(), tc.wantLat)
		}
	}
}

// geodeticToICRS reinterprets a Geodetic's (lon, lat) as an (RA, Dec) pair
// — legitimate here since both are just angular coordinates on a sphere,
// letting coord.Separation measure the angular distance between two
// Geodetic points for SmallCircle's own tests below.
func geodeticToICRS(g *coord.Geodetic) coord.ICRS {
	return coord.NewICRS(g.Lon(), g.Lat())
}

// TestSmallCircle_PointsAtExactSeparation confirms every returned point is
// exactly radius away from center, independently verified via
// coord.Separation (a wholly separate code path from SmallCircle's own
// vector construction).
func TestSmallCircle_PointsAtExactSeparation(t *testing.T) {
	center, err := coord.NewGeodetic(angle.Deg(40), angle.Deg(-15), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	for _, radiusDeg := range []float64{5, 30, 90, 150} {
		radius := angle.Deg(radiusDeg)

		pts, err := coord.SmallCircle(center, radius, 24)
		if err != nil {
			t.Fatalf("SmallCircle(radius=%v): %v", radiusDeg, err)
		}

		centerICRS := geodeticToICRS(center)

		for i, p := range pts {
			sep := coord.Separation(centerICRS, geodeticToICRS(p))
			if math.Abs(sep.Degrees()-radiusDeg) > 1e-6 {
				t.Errorf("radius=%v point %d: separation = %v°, want %v°", radiusDeg, i, sep.Degrees(), radiusDeg)
			}
		}
	}
}

// TestSmallCircle_EquatorialNinetyReachesPoles confirms a 90°-radius
// circle centered on the equator (the geometric terminator at equinox)
// passes through both poles — a direct geometric consequence of the
// small-circle construction, not tautological with SmallCircle's own math
// since it's checking a specific, independently-derivable special case.
func TestSmallCircle_EquatorialNinetyReachesPoles(t *testing.T) {
	center, err := coord.NewGeodetic(angle.Deg(10), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	// n=36 (10° steps) guarantees az=90° and az=270° are hit exactly,
	// which is exactly where the pole-reaching points fall for an
	// equatorial center — see coord/subpoint.go's basis construction.
	pts, err := coord.SmallCircle(center, angle.Deg(90), 36)
	if err != nil {
		t.Fatalf("SmallCircle: %v", err)
	}

	var maxAbsLat float64

	for _, p := range pts {
		if abs := math.Abs(p.Lat().Degrees()); abs > maxAbsLat {
			maxAbsLat = abs
		}
	}

	if maxAbsLat < 89.99 {
		t.Errorf("max |lat| among equinox-terminator points = %v°, want ~90° (a pole)", maxAbsLat)
	}
}

// TestSmallCircle_RejectsTooFewPoints and TestSmallCircle_RejectsNilCenter
// cover SmallCircle's two documented error paths.
func TestSmallCircle_RejectsTooFewPoints(t *testing.T) {
	center, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	for _, n := range []int{-1, 0, 1, 2} {
		if _, err := coord.SmallCircle(center, angle.Deg(10), n); !errors.Is(err, coord.ErrTooFewPoints) {
			t.Errorf("SmallCircle(n=%d) error = %v, want ErrTooFewPoints", n, err)
		}
	}
}

func TestSmallCircle_RejectsNilCenter(t *testing.T) {
	if _, err := coord.SmallCircle(nil, angle.Deg(10), 10); !errors.Is(err, coord.ErrNilCenter) {
		t.Fatalf("SmallCircle(nil center) error = %v, want ErrNilCenter", err)
	}
}

// TestSmallCircle_HandlesLongitudeWrap centers the circle exactly on the
// ±180° longitude discontinuity and confirms every point still separates
// from center by exactly radius (via coord.Separation, which handles the
// wrap correctly by construction) and that all n points are distinct —
// guarding against a naive lon-arithmetic bug that only shows up when the
// circle straddles the antimeridian.
func TestSmallCircle_HandlesLongitudeWrap(t *testing.T) {
	center, err := coord.NewGeodetic(angle.Deg(180), angle.Deg(20), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	for _, n := range []int{3, 4, 5, 12, 36} {
		pts, err := coord.SmallCircle(center, angle.Deg(25), n)
		if err != nil {
			t.Fatalf("SmallCircle(n=%d): %v", n, err)
		}

		if len(pts) != n {
			t.Fatalf("SmallCircle(n=%d) returned %d points", n, len(pts))
		}

		centerICRS := geodeticToICRS(center)
		seen := make(map[[2]int]bool, n)

		for i, p := range pts {
			if math.IsNaN(p.Lat().Degrees()) || math.IsNaN(p.Lon().Degrees()) {
				t.Fatalf("n=%d point %d: NaN coordinate", n, i)
			}

			sep := coord.Separation(centerICRS, geodeticToICRS(p))
			if math.Abs(sep.Degrees()-25) > 1e-6 {
				t.Errorf("n=%d point %d: separation = %v°, want 25°", n, i, sep.Degrees())
			}

			// Round to a coarse grid to detect exact duplicates without
			// float-equality fragility.
			key := [2]int{int(p.Lat().Degrees() * 1e6), int(p.Lon().Degrees() * 1e6)}
			if seen[key] {
				t.Errorf("n=%d point %d: duplicate of an earlier point", n, i)
			}

			seen[key] = true
		}
	}
}
