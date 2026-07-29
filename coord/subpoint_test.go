package coord_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// TestSubPoint_LatitudeInvariantUnderGAST verifies the core geometric fact
// SubPoint's doc comment relies on: rotating a geocentric direction about
// the Z axis by GAST (converting GCRS → ECEF) leaves its Z component, and
// therefore its declination/geodetic latitude, unchanged — only longitude
// shifts. This must hold at any epoch, so it's checked at two very
// different Julian dates rather than derived from SubPoint's own code.
func TestSubPoint_LatitudeInvariantUnderGAST(t *testing.T) {
	vec := vector.V3(0.6, 0.3, 0.5) // arbitrary, non-axis-aligned direction
	wantLat := math.Asin(vec.Z / vec.Norm())

	epochs := []time.Time{
		time.FromJD(2451545.0, time.UTC), // J2000.0
		time.FromJD(2460310.5, time.UTC), // an unrelated later date
	}

	for _, tm := range epochs {
		geo, err := coord.SubPoint(vec, tm)
		if err != nil {
			t.Fatalf("SubPoint at JD %v: %v", tm, err)
		}

		if got := geo.Lat().Radians(); math.Abs(got-wantLat) > 1e-9 {
			t.Errorf("SubPoint at JD %v: Lat = %v rad, want %v rad (declination is GAST-invariant)", tm, got, wantLat)
		}
	}
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
// the pole (Z-axis) yields a geodetic latitude of exactly ±90°, matching
// the fact that longitude is undefined at a pole (the underlying
// FromUnitVector's atan2(0,0)=0 convention is acceptable there).
func TestSubPoint_PoleIsLongitudeIndependent(t *testing.T) {
	tm := time.FromJD(2451545.0, time.UTC)

	north, err := coord.SubPoint(vector.V3(0, 0, 1), tm)
	if err != nil {
		t.Fatalf("SubPoint(north): %v", err)
	}

	if math.Abs(north.Lat().Degrees()-90) > 1e-9 {
		t.Errorf("north pole Lat = %v, want 90°", north.Lat().Degrees())
	}

	south, err := coord.SubPoint(vector.V3(0, 0, -1), tm)
	if err != nil {
		t.Fatalf("SubPoint(south): %v", err)
	}

	if math.Abs(south.Lat().Degrees()-(-90)) > 1e-9 {
		t.Errorf("south pole Lat = %v, want -90°", south.Lat().Degrees())
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
