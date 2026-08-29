package metrology_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/vector"
)

// The wrap boundary is the whole reason AngleDifference exists.
//
// Two directions two arcseconds apart either side of zero must measure two
// arcseconds. A plain subtraction measures 359.999... degrees, which is not
// merely imprecise — it is large enough to swamp every real residual in a
// dataset and to look like a catastrophic failure rather than a metric bug.
func TestAngleDifferenceWrapsTheShortWay(t *testing.T) {
	t.Parallel()

	const arcsec = 1.0 / 3600

	for _, tc := range []struct {
		name       string
		got, want  angle.Angle
		wantDegree float64
	}{
		{"either side of zero", angle.Deg(360 - arcsec), angle.Deg(arcsec), -2 * arcsec},
		{"either side of zero, reversed", angle.Deg(arcsec), angle.Deg(360 - arcsec), 2 * arcsec},
		{"identical", angle.Deg(123.456), angle.Deg(123.456), 0},
		{"whole turn apart", angle.Deg(400), angle.Deg(40), 0},
		{"ordinary difference", angle.Deg(31), angle.Deg(30), 1},
		{"ordinary difference, negative", angle.Deg(30), angle.Deg(31), -1},
		// Exactly half a turn is the one genuinely ambiguous case: +180 and
		// -180 are the same direction. The convention is (-180, +180], so
		// it resolves to +180 and never to -180, which keeps the sign of a
		// bias stable instead of flipping on rounding noise.
		{"exactly half a turn", angle.Deg(180), angle.Deg(0), 180},
		{"exactly half a turn, reversed", angle.Deg(0), angle.Deg(180), 180},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := metrology.AngleDifference(tc.got, tc.want).Degrees()
			if math.Abs(got-tc.wantDegree) > 1e-9 {
				t.Errorf("AngleDifference = %.9f deg, want %.9f deg", got, tc.wantDegree)
			}
		})
	}
}

// AngularSeparation has to be right at the poles and accurate for the small
// separations a validation suite actually measures.
func TestAngularSeparation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                   string
		lon1, lat1, lon2, lat2 float64
		wantDeg                float64
	}{
		{"identical", 10, 20, 10, 20, 0},
		{"antipodal", 0, 0, 180, 0, 180},
		{"pole to pole", 0, 90, 0, -90, 180},
		// Longitude is meaningless at a pole: every meridian meets there,
		// so two points with the same latitude 90 are the same point. A
		// formula that mishandles this reports the longitude difference.
		{"same pole, different longitudes", 0, 90, 175, 90, 0},
		{"one degree along the equator", 0, 0, 1, 0, 1},
		{"one degree in latitude", 45, 10, 45, 11, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := metrology.AngularSeparation(
				angle.Deg(tc.lon1), angle.Deg(tc.lat1),
				angle.Deg(tc.lon2), angle.Deg(tc.lat2),
			).Degrees()

			if math.Abs(got-tc.wantDeg) > 1e-9 {
				t.Errorf("AngularSeparation = %.9f deg, want %.9f deg", got, tc.wantDeg)
			}
		})
	}
}

// A tiny separation must survive as a tiny separation.
//
// The cosine rule loses about half its significant digits below a
// milliarcsecond, which is exactly the regime an astrometric comparison lives
// in, so the implementation uses atan2 of the cross product over the dot. At
// one microarcsecond the cosine rule would return zero or noise; this asserts
// the value comes back with its magnitude intact.
func TestAngularSeparationKeepsSmallAnglesPrecise(t *testing.T) {
	t.Parallel()

	const wantMicroarcsec = 1.0

	sep := metrology.AngularSeparation(
		angle.Zero(), angle.Zero(),
		angle.Deg(wantMicroarcsec/3.6e9), angle.Zero(),
	)

	got := sep.Arcseconds() * 1e6
	if math.Abs(got-wantMicroarcsec) > 1e-3 {
		t.Errorf("separation of 1 microarcsec measured as %.6f microarcsec", got)
	}
}

// RelativeError must not quietly become an absolute error at zero.
//
// Substituting |got| when want is zero is the common shortcut, and it changes
// the quantity mid-dataset: a column labelled "relative error" then holds
// relative errors for most rows and absolute ones for the rest, in whatever
// units the underlying value happened to have. NaN is the honest answer, and
// Stats counts it rather than averaging it.
func TestRelativeErrorAtZero(t *testing.T) {
	t.Parallel()

	if got := metrology.RelativeError(1e-30, 0); !math.IsNaN(got) {
		t.Errorf("RelativeError(1e-30, 0) = %v, want NaN", got)
	}

	if got := metrology.RelativeError(0, 0); !math.IsNaN(got) {
		t.Errorf("RelativeError(0, 0) = %v, want NaN — zero over zero is not zero error", got)
	}

	if got := metrology.RelativeError(1.01, 1.0); math.Abs(got-0.01) > 1e-12 {
		t.Errorf("RelativeError(1.01, 1) = %v, want 0.01", got)
	}

	// Sign of want must not leak into the magnitude.
	if got := metrology.RelativeError(-1.01, -1.0); math.Abs(got-0.01) > 1e-12 {
		t.Errorf("RelativeError(-1.01, -1) = %v, want 0.01", got)
	}
}

func TestVectorDistance(t *testing.T) {
	t.Parallel()

	a := vector.Vec3{X: 1, Y: 2, Z: 2}
	b := vector.Vec3{X: 1, Y: 2, Z: 2}

	if got := metrology.VectorDistance(a, b); got != 0 {
		t.Errorf("distance to itself = %v, want 0", got)
	}

	// 3-4-5 in two components, so the expected norm is exact in binary.
	if got := metrology.VectorDistance(
		vector.Vec3{X: 3, Y: 4, Z: 0}, vector.Vec3{},
	); got != 5 {
		t.Errorf("distance = %v, want 5", got)
	}
}
