package coord_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// The offset frame is a rotation, so nothing in it can be checked by comparing
// it against itself. Every test below pins it against something that already
// existed and was implemented a different way: coord.Separation, which works
// from the cross and dot products of two unit vectors, and coord.PositionAngle,
// which works from spherical trigonometry on the two declinations. Neither
// shares a line of code with the rotation under test.

// exactTolerance is a nanoradian, about 0.2 microarcseconds. The relations
// below are identities rather than approximations, so the only thing that
// should separate the two sides is float rounding.
const exactTolerance = 1e-9

// skyOffsetOrigins spans the declinations where the naive alternative to this
// frame goes wrong, plus the pole, where it has no answer at all.
var skyOffsetOrigins = []struct {
	name string
	ra   float64
	dec  float64
}{
	{"equator", 0, 0},
	{"mid-northern", 83.8221, -5.3911},
	{"high-northern", 120, 80},
	{"near the south pole", 200, -89.5},
	{"north pole", 45, 90},
	{"just past the RA wrap", 359.99, 12},
}

// TestSkyOffsetPlacesTheOriginAtZero is the defining property: the frame is
// centred on its origin, so the origin has no offset from itself.
func TestSkyOffsetPlacesTheOriginAtZero(t *testing.T) {
	t.Parallel()

	for _, tc := range skyOffsetOrigins {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			origin := coord.NewICRS(angle.Deg(tc.ra), angle.Deg(tc.dec))

			// A non-zero rotation must not move the origin either — it turns
			// the frame about that point, it does not shift it.
			lon, lat := coord.NewSkyOffset(origin, angle.Deg(37)).FromICRS(origin)

			testutil.AssertNear(t, "lon of the origin",
				lon.Radians(), 0, exactTolerance)
			testutil.AssertNear(t, "lat of the origin",
				lat.Radians(), 0, exactTolerance)
		})
	}
}

// TestSkyOffsetAgreesWithSeparationAndPositionAngle is the main check.
//
// Each case names a separation and a position angle, places a point there
// using nothing but the spherical destination formula from the frame's own
// origin, and then asks [Separation] and [PositionAngle] — neither of which
// knows this frame exists — whether the point really ended up there.
//
// It is exact rather than a small-angle check: the separations run from an
// arcsecond to 170°, because a frame that was only locally right would pass a
// test that only looked nearby. That is not hypothetical — the first draft of
// this test built its points as (sep·sin pa, sep·cos pa), the tangent-plane
// approximation, and was already wrong by 1e-8 radians at one arcminute.
func TestSkyOffsetAgreesWithSeparationAndPositionAngle(t *testing.T) {
	t.Parallel()

	separations := []float64{1.0 / 3600, 1.0 / 60, 1, 15, 90, 170}
	angles := []float64{0, 37, 90, 143, 180, 251, 315}
	rotations := []float64{0, 30, -75}

	for _, tc := range skyOffsetOrigins {
		for _, rot := range rotations {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				origin := coord.NewICRS(angle.Deg(tc.ra), angle.Deg(tc.dec))
				f := coord.NewSkyOffset(origin, angle.Deg(rot))

				for _, sepDeg := range separations {
					for _, paDeg := range angles {
						sep, pa := angle.Deg(sepDeg).Radians(), angle.Deg(paDeg).Radians()

						// Where a point at that separation and bearing lands,
						// measured from the frame's origin at (0, 0). This is
						// the great-circle destination formula, not a
						// projection onto a plane.
						lat := angle.Rad(math.Asin(math.Sin(sep) * math.Cos(pa)))
						lon := angle.Rad(math.Atan2(math.Sin(pa)*math.Sin(sep), math.Cos(sep)))

						where := fmt.Sprintf("%s, sep %g deg, pa %g deg, rotation %g deg",
							tc.name, sepDeg, paDeg, rot)

						c := f.ToICRS(lon, lat)

						testutil.AssertNear(t, "separation at "+where,
							coord.Separation(origin, c).Radians(), sep, exactTolerance)

						// The offsets have to survive the trip back, or the
						// frame is not a frame.
						gotLon, gotLat := f.FromICRS(c)

						testutil.AssertNear(t, "lat at "+where,
							gotLat.Radians(), lat.Radians(), exactTolerance)

						// Every longitude names the same point at the frame's
						// own poles, so there is nothing there to compare.
						if math.Abs(lat.Degrees()) < 90-1e-9 {
							assertAngle(t, gotLon, lon, exactTolerance, "lon at "+where)
						}

						// PositionAngle divides by the cosine of the origin's
						// declination, so it has nothing to say when the
						// origin is a pole, and nothing useful when the two
						// points coincide.
						if math.Abs(tc.dec) > 89 || sepDeg == 0 {
							continue
						}

						gotPA := coord.PositionAngle(origin, c)
						wantPA := angle.Deg(paDeg).Add(f.Rotation()).Wrap360()

						if diff := math.Abs(gotPA.Sub(wantPA).Wrap180().Radians()); diff > 1e-8 {
							t.Errorf("position angle at %s, sep %g deg, pa %g deg, rotation %g: "+
								"got %v, want %v (differ by %g rad)",
								tc.name, sepDeg, paDeg, rot, gotPA, wantPA, diff)
						}
					}
				}
			})
		}
	}
}

// TestSkyOffsetRoundTrips checks the inverse, including at separations where a
// tangent-plane approximation would have diverged long ago.
func TestSkyOffsetRoundTrips(t *testing.T) {
	t.Parallel()

	f := coord.NewSkyOffset(coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911)), angle.Deg(42))

	for _, tc := range []struct {
		name     string
		lon, lat float64
	}{
		{"the origin", 0, 0},
		{"an arcsecond away", 1.0 / 3600, -1.0 / 3600},
		{"a detector away", 0.25, 0.18},
		{"a quadrant away", 60, -30},
		{"the frame's own pole", 0, 90},
		{"the antipode", 180, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lon, lat := angle.Deg(tc.lon), angle.Deg(tc.lat)

			gotLon, gotLat := f.FromICRS(f.ToICRS(lon, lat))

			// At the frame's own pole every longitude names the same point, so
			// only the latitude is meaningful there.
			if math.Abs(tc.lat) < 90 {
				assertAngle(t, gotLon, lon, exactTolerance, "lon")
			}

			testutil.AssertNear(t, "lat",
				gotLat.Radians(), lat.Radians(), exactTolerance)
		})
	}
}

// TestSkyOffsetSignsPointEastAndNorth pins the two things a caller reads off
// an offset without thinking about them: which way is positive, and that a
// point to the west reports a small negative longitude rather than a number
// just under 360.
//
// The wrap matters more than it sounds. A dither pattern that averages its
// offsets, or a plot that scales to them, turns a 359.99° into nonsense while
// a -0.01° behaves.
func TestSkyOffsetSignsPointEastAndNorth(t *testing.T) {
	t.Parallel()

	origin := coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911))
	f := coord.NewSkyOffset(origin, 0)

	for _, tc := range []struct {
		name    string
		pa      float64
		wantLon func(float64) bool
		wantLat func(float64) bool
	}{
		{"east", 90, func(l float64) bool { return l > 0 }, func(b float64) bool { return math.Abs(b) < 1e-9 }},
		{"west", 270, func(l float64) bool { return l < 0 && l > -1 }, func(b float64) bool { return math.Abs(b) < 1e-9 }},
		{"north", 0, func(l float64) bool { return math.Abs(l) < 1e-9 }, func(b float64) bool { return b > 0 }},
		{"south", 180, func(l float64) bool { return math.Abs(l) < 1e-9 }, func(b float64) bool { return b < 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pa := angle.Deg(tc.pa).Radians()
			sep := angle.Deg(0.5).Radians()

			c := f.ToICRS(
				angle.Rad(math.Atan2(math.Sin(pa)*math.Sin(sep), math.Cos(sep))),
				angle.Rad(math.Asin(math.Sin(sep)*math.Cos(pa))),
			)

			lon, lat := f.FromICRS(c)

			if !tc.wantLon(lon.Degrees()) || !tc.wantLat(lat.Degrees()) {
				t.Errorf("half a degree %s of the origin reads as lon %v, lat %v",
					tc.name, lon, lat)
			}
		})
	}
}

// TestSkyOffsetRotationAimsLatAtThePositionAngle pins the orientation the doc
// comment promises, which is the part a caller setting an instrument angle
// depends on being right.
func TestSkyOffsetRotationAimsLatAtThePositionAngle(t *testing.T) {
	t.Parallel()

	origin := coord.NewICRS(angle.Deg(210.8), angle.Deg(54.35))

	for _, rot := range []float64{0, 15, 90, 180, 275, -60} {
		f := coord.NewSkyOffset(origin, angle.Deg(rot))

		// A point one degree along the frame's +lat axis.
		c := f.ToICRS(0, angle.Deg(1))

		got := coord.PositionAngle(origin, c)
		want := angle.Deg(rot).Wrap360()

		if diff := math.Abs(got.Sub(want).Wrap180().Radians()); diff > 1e-9 {
			t.Errorf("with rotation %g deg the +lat axis points at position angle %v, want %v",
				rot, got, want)
		}
	}
}

// TestSubtractingCoordinatesIsNotAnOffset is the reason this frame exists,
// stated as numbers.
//
// Each case takes a point exactly one degree due east of its origin — due
// east, on the sphere, at a position angle of 90° — and asks what the usual
// shortcuts make of it. Both fail, and the second fails much worse than the
// first:
//
//   - Δα·cos δ is offered as the "cos-δ correction" for the scale of right
//     ascension. It is wrong by 12 arcseconds at δ = 80° and by 13 arcminutes
//     at δ = 89°.
//   - Δδ is expected to be zero, because the offset was purely eastward. It is
//     3 arcminutes at δ = 80° and 25 arcminutes at δ = 89°. That is the shear
//     no scale factor can remove, and it is the larger error of the two.
func TestSubtractingCoordinatesIsNotAnOffset(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		dec float64
		// Lower bounds on how wrong the two shortcuts are, in arcminutes.
		minScaleError float64
		minShear      float64
	}{
		{dec: 0, minScaleError: 0, minShear: 0},
		{dec: 60, minScaleError: 0.01, minShear: 0.8},
		{dec: 80, minScaleError: 0.15, minShear: 2.5},
		{dec: 89, minScaleError: 10, minShear: 20},
	} {
		origin := coord.NewICRS(angle.Deg(120), angle.Deg(tc.dec))
		f := coord.NewSkyOffset(origin, 0)

		c := f.ToICRS(angle.Deg(1), 0)

		// The frame's own answer is exactly right, which is the control: the
		// numbers below are the shortcuts being wrong, not this being wrong.
		testutil.AssertNear(t, "separation of the constructed point",
			coord.Separation(origin, c).Degrees(), 1, 1e-9)

		scaleError := (c.RA().Sub(origin.RA()).Wrap180().Degrees()*
			math.Cos(origin.Dec().Radians()) - 1) * 60
		shear := c.Dec().Sub(origin.Dec()).Degrees() * 60

		t.Logf("dec %+5.1f: a 1 deg eastward offset has dRA %.4f deg; "+
			"dRA*cos(dec) is off by %+.4f arcmin and dDec is %+.4f arcmin",
			tc.dec, c.RA().Sub(origin.RA()).Wrap180().Degrees(), scaleError, shear)

		if math.Abs(scaleError) < tc.minScaleError {
			t.Errorf("dec %g: dRA*cos(dec) is off by only %g arcmin, want at least %g",
				tc.dec, math.Abs(scaleError), tc.minScaleError)
		}

		if math.Abs(shear) < tc.minShear {
			t.Errorf("dec %g: a purely eastward offset changed the declination by only "+
				"%g arcmin, want at least %g", tc.dec, math.Abs(shear), tc.minShear)
		}
	}
}

// TestSkyOffsetAccessorsReportWhatItWasBuiltWith keeps the frame's parameters
// readable, since a caller handed a frame has no other way to ask what it is.
func TestSkyOffsetAccessorsReportWhatItWasBuiltWith(t *testing.T) {
	t.Parallel()

	origin := coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911))
	f := coord.NewSkyOffset(origin, angle.Deg(42))

	if f.Origin() != origin {
		t.Errorf("Origin() = %v, want %v", f.Origin(), origin)
	}

	if f.Rotation() != angle.Deg(42) {
		t.Errorf("Rotation() = %v, want 42 deg", f.Rotation())
	}

	if s := f.String(); !strings.Contains(s, "SkyOffset") {
		t.Errorf("String() = %q, want it to name the frame", s)
	}
}

func BenchmarkSkyOffsetFromICRS(b *testing.B) {
	f := coord.NewSkyOffset(coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911)), angle.Deg(42))
	c := coord.NewICRS(angle.Deg(83.9), angle.Deg(-5.3))

	b.ReportAllocs()

	for range b.N {
		lon, lat := f.FromICRS(c)
		_, _ = lon, lat
	}
}

// assertAngle compares two angles as directions rather than as numbers, so
// that +180 degrees and -180 degrees count as the same longitude. They are the
// same place, and a plain subtraction makes them look a full turn apart.
func assertAngle(t *testing.T, got, want angle.Angle, tol float64, what string) {
	t.Helper()

	if diff := math.Abs(got.Sub(want).Wrap180().Radians()); diff > tol {
		t.Errorf("%s = %v, want %v (differ by %g rad)", what, got, want, diff)
	}
}
