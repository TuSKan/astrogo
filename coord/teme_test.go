package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// TEME vectors the tests rotate, in km: a low orbit inclined like the ISS's,
// and a geostationary one.
var temeVectors = []vector.Vec3{
	{X: -4400.6, Y: 3000.2, Z: 4210.8},
	{X: 30214.1, Y: -29411.7, Z: 12.3},
}

// equinoxRoute is TEME → GCRS the way ephemeris/satellite does it without a
// Context: R3(−EqEq) to the true equinox of date, then the transpose of the
// IAU 2006/2000A bias-precession-nutation matrix, both evaluated at t.
func equinoxRoute(t time.Time, v vector.Vec3) vector.Vec3 {
	tt1, tt2 := t.TT().JDParts()
	bpn := gofaext.Pnm06a(tt1, tt2)
	s, c := math.Sincos(gofaext.Ee06aFromBPN(tt1, tt2, bpn))

	x, y := c*v.X-s*v.Y, s*v.X+c*v.Y

	return vector.Vec3{
		X: bpn[0][0]*x + bpn[1][0]*y + bpn[2][0]*v.Z,
		Y: bpn[0][1]*x + bpn[1][1]*y + bpn[2][1]*v.Z,
		Z: bpn[0][2]*x + bpn[1][2]*y + bpn[2][2]*v.Z,
	}
}

// turnedBy is the angle between two vectors of about the same length, in
// radians.
func turnedBy(a, b vector.Vec3) float64 { return a.Sub(b).Norm() / a.Norm() }

// TestTEMEToGCRSIsTheEquinoxRoute: at the instant a Context was built for,
// TEME → CIRS → GCRS through its celestial-to-intermediate matrix is the same
// rotation as the equinox route through the bias-precession-nutation matrix.
// The two are one rotation factored two ways — SOFA's equation of the origins
// is what separates the matrices, and GMST − ERA is that plus the equation of
// the equinoxes — so they agree to rounding, across two centuries of
// precession.
func TestTEMEToGCRSIsTheEquinoxRoute(t *testing.T) {
	site, atm, _, _ := setTimeFixture(t)

	// Rounding in a 3×3 rotation and two sidereal-time polynomials, with room:
	// 1e-12 rad is 0.2 µas, against the 0.1″ the Context's own window allows.
	const tol = 1e-12

	for _, jd := range []float64{2447892.5, 2461000.0, 2470000.25} { // 1990, 2025, 2050
		at := time.FromJD(jd, time.UTC)
		ctx := coord.NewContext(at, site, atm)

		for _, v := range temeVectors {
			if d := turnedBy(ctx.TEMEToGCRS(v), equinoxRoute(at, v)); d > tol {
				t.Errorf("JD %.2f, %v: the two routes differ by %.3g rad, want ≤ %g", jd, v, d, tol)
			}
		}
	}
}

// TestTEMELookAngleIgnoresTheContextWindow is why the Context's matrix is the
// right one to use (#812). A moved Context interpolates its precession-
// nutation across a day (#790), and GCRS through it carries that
// interpolation's error; but GeocentricToObserved turns GCRS back through the
// same matrix, so a TEME vector's look angle is what a Context built at the
// instant gives — Earth rotation and GMST are both taken at the instant.
//
// The probe is 12 hours from the build, mid-bracket, where that error peaks.
// The equinox route, exact in GCRS, meets the moved Context's interpolated
// matrix there: measured, 0.91 mas off. That is the control; it shows the
// window is wide enough to be seen. It used to probe 55 minutes in, where the
// hourly rebuild's stale matrix put the control at 2.7 to 4.0 mas, and #790's
// interpolation left 0.17 (#825).
func TestTEMELookAngleIgnoresTheContextWindow(t *testing.T) {
	site, atm, base, _ := setTimeFixture(t)

	moved := coord.NewContext(base, site, atm)
	at := base.Add(unit.Hours(12)) // mid-bracket
	moved.SetTime(at)

	fresh := coord.NewContext(at, site, atm)

	const kmPerAU = 149597870.7

	sep := func(a, b coord.AltAz) float64 {
		da := a.Alt().Radians() - b.Alt().Radians()
		dz := math.Remainder(a.Az().Radians()-b.Az().Radians(), 2*math.Pi) * math.Cos(a.Alt().Radians())

		return math.Hypot(da, dz)
	}

	// What is left is rounding: this test binary loads no EOP, so polar
	// motion, the one factor not taken at the instant, is zero at both.
	// Measured, the two agree to under a microarcsecond; 0.01 mas is ten
	// thousand times that, and leaves the control at ninety times the bound.
	const tol = 0.01 * math.Pi / (180 * 3600 * 1000)

	var worstControl float64

	for _, v := range temeVectors {
		want := fresh.GeocentricToObserved(fresh.TEMEToGCRS(v).MulScalar(1 / kmPerAU))
		got := moved.GeocentricToObserved(moved.TEMEToGCRS(v).MulScalar(1 / kmPerAU))

		if d := sep(got, want); d > tol {
			t.Errorf("%v: a moved Context's look angle is %.3g mas from a fresh one's, want ≤ 0.01 mas",
				v, d*180*3600*1000/math.Pi)
		}

		control := moved.GeocentricToObserved(equinoxRoute(at, v).MulScalar(1 / kmPerAU))
		worstControl = max(worstControl, sep(control, want))
	}

	if worstControl < 10*tol {
		t.Errorf("the equinox route through the moved Context is only %.3g mas off; "+
			"the window this test relies on is not visible", worstControl*180*3600*1000/math.Pi)
	}
}
