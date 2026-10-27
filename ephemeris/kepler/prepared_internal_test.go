package kepler

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// rotated is the orientation as StateAt did it until docs/kepler.md phase 3:
// each perifocal vector carried through the three angles, with the apsis
// advanced by the precession, and then the frame.
func rotated(el Elements, v vector.Vec3, dtDays float64) vector.Vec3 {
	argp := angle.Rad(el.argPeriapsis.Radians() + el.precession.rate()*dtDays)
	ref := rotatePerifocalToEcliptic(v, el.inclination, el.ascendingNode, argp)

	if lp := el.plane; lp != nil {
		return lp.rotateToICRF(ref)
	}

	return rotateEclipticToEquatorialJ2000(ref)
}

// TestPreparedOrientationIsTheRotation: carrying a state along the prepared P
// and Q is the same map as rotating it through the angles, to rounding — on
// the J2000 ecliptic and on a Laplace plane, with the apsis fixed and turning,
// over a decade. A bound, not bit identity: the arithmetic differs.
//
// A With method that forgot to prepare would leave P and Q at another
// orientation, and this is the comparison that sees it.
func TestPreparedOrientationIsTheRotation(t *testing.T) {
	t.Parallel()

	base, err := NewElements(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		unit.AU(0.0028), 0.0041, angle.Deg(2.2), angle.Deg(43.1), angle.Deg(85.7), angle.Deg(171.3))
	if err != nil {
		t.Fatal(err)
	}

	for name, el := range map[string]Elements{
		"ecliptic":                     base,
		"Laplace plane":                base.WithLaplacePlane(LaplacePlane{RA: angle.Deg(268.05), Dec: angle.Deg(64.49)}),
		"turning apsis":                base.WithSecularPrecession(SecularPrecession{ApsisPeriod: 1.3}),
		"turning apsis, Laplace plane": base.WithLaplacePlane(LaplacePlane{RA: angle.Deg(268.05), Dec: angle.Deg(64.49)}).WithSecularPrecession(SecularPrecession{ApsisPeriod: 1.3}),
	} {
		worst := 0.0

		for d := -1800.0; d <= 1800; d += 17.3 {
			// Eight ulps of a unit vector, each path rounding about three
			// times per component, and the turning angle's own rounding,
			// which neither path can beat and each pays once: over a decade
			// the angle reaches tens of radians.
			turn := math.Abs(el.precession.rate()*d) + math.Abs(el.argPeriapsis.Radians())
			tol := 8*0x1p-53 + 2*turn*0x1p-52

			for _, v := range []vector.Vec3{vector.V3(0.0021, -0.0017, 0), vector.V3(-3.1e-4, 4.4e-4, 0)} {
				got, _ := el.orient(v, v, d)
				want := rotated(el, v, d)
				worst = math.Max(worst, got.Sub(want).Norm()/want.Norm()/tol)
			}
		}

		if worst > 1 {
			t.Errorf("%s: the prepared orientation is %.2g times its bound from the rotation", name, worst)
		}
	}
}

// TestABareLiteralIsPreparedWhenAsked: a set built without the constructors,
// as this package's tests do, carries no prepared quantities, and StateAt
// prepares it then, to the same answer, bit for bit, as the constructor's.
func TestABareLiteralIsPreparedWhenAsked(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	built, err := NewElements(epoch, unit.AU(2.77), 0.0785, angle.Deg(10.6), angle.Deg(80.3), angle.Deg(73.6), angle.Deg(291.4))
	if err != nil {
		t.Fatal(err)
	}

	literal := Elements{
		epoch: epoch, semiMajorAxis: unit.AU(2.77), eccentricity: 0.0785,
		inclination: angle.Deg(10.6), ascendingNode: angle.Deg(80.3),
		argPeriapsis: angle.Deg(73.6), meanAnomaly: angle.Deg(291.4),
	}

	if literal.prep.ok {
		t.Fatal("a bare literal claims to be prepared")
	}

	at := epoch.Add(unit.Days(123.4))

	wantPos, wantVel, err := built.StateAt(at)
	if err != nil {
		t.Fatal(err)
	}

	gotPos, gotVel, err := literal.StateAt(at)
	if err != nil {
		t.Fatal(err)
	}

	if gotPos != wantPos || gotVel != wantVel {
		t.Errorf("a bare literal answers %v %v, the constructor's set %v %v", gotPos, gotVel, wantPos, wantVel)
	}
}

// TestAnInvalidatedSetIsReported: a With method that makes a set invalid, a
// period on an orbit that does not close, leaves it unprepared, and StateAt
// says why rather than propagating stale quantities.
func TestAnInvalidatedSetIsReported(t *testing.T) {
	t.Parallel()

	tp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	el, err := FromPerihelion(tp, tp,
		unit.AU(1.1), 1.2, angle.Deg(40), angle.Deg(10), angle.Deg(20))
	if err != nil {
		t.Fatal(err)
	}

	bad := el.WithPeriod(400)

	if _, _, err := bad.StateAt(el.Epoch()); err == nil {
		t.Error("a hyperbola given a period propagated")
	}
}
