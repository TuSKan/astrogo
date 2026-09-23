package kepler_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// cometElements are perihelion-form elements as a table row: perihelion time
// (JD TDB), q (AU), e, and the three angles in degrees.
type cometElements struct {
	name                string
	tp, q, e            float64
	incl, node, argPeri float64
}

func (c cometElements) build(t *testing.T) kepler.Elements {
	t.Helper()

	el, err := kepler.FromPerihelion(time.FromJDParts(c.tp, 0, time.TDB), unit.AU(c.q), c.e,
		angle.Deg(c.incl), angle.Deg(c.node), angle.Deg(c.argPeri))
	if err != nil {
		t.Fatalf("%s: FromPerihelion: %v", c.name, err)
	}

	return el
}

// Elements as the MPC published them, from #374. They are orbit shapes to
// propagate, not positions to trust: the tests that use them compare two
// formulations of the same two-body motion.
var (
	tsuchinshanATLAS = cometElements{"C/2023 A3", 2460581.3251, 0.391335, 1.000178, 139.1002, 21.6693, 308.5797}
	ztf              = cometElements{"C/2022 E3", 2459956.7376, 1.117885, 1.000066, 109.1808, 302.4889, 145.8919}
	encke            = cometElements{"2P/Encke", 2461446.7271, 0.338614, 0.847315, 11.3479, 334.0194, 187.2869}
	ponsBrooks       = cometElements{"12P/Pons-Brooks", 2460421.7009, 0.781879, 0.954746, 74.1498, 255.8672, 199.0343}
	oumuamua         = cometElements{"1I/'Oumuamua", 2458006.0073, 0.255912, 1.201134, 122.7417, 24.5969, 241.8105}
	borisov          = cometElements{"2I/Borisov", 2458826.0528, 2.006521, 3.356476, 44.0526, 308.1477, 209.1237}
)

// muAUDay is the Sun's mass parameter in AU³/day², from the constants
// StateAt itself uses.
func muAUDay() float64 {
	au := unit.AU(1).Meters()
	day := constants.Derived.JulianDaySeconds.Value

	return constants.IAU.SunGravitationalParameter.Value * day * day / (au * au * au)
}

// inPlane returns the distance and the true anomaly of pos, measured in the
// orbit's own plane from peri, the perihelion direction, about the angular
// momentum h. Both are unchanged by the orientation angles, so a comparison
// through them tests the propagation and nothing else.
func inPlane(peri, pos, h vector.Vec3) (r, nu float64) {
	return pos.Norm(), math.Atan2(peri.Cross(pos).Dot(h.Unit()), peri.Dot(pos))
}

// TestFromPerihelionReproducesHorizonsOwnConversion pins the propagation to
// JPL. Beside the osculating elements it reports, Horizons prints its own
// two-body conversion of them to an ICRF heliocentric state at the elements'
// epoch ("Equivalent ICRF heliocentric cartesian coordinates"). Propagating
// the same elements from the perihelion time to that epoch has to reproduce
// it.
//
// The four bodies cover the conics: two hyperbolas, one of them far from
// parabolic; an ellipse; and a comet 1e-4 above e = 1, 133 days from
// perihelion, which is where the closed-form Stumpff functions lose digits.
//
// Two conventions differ, and both are set to Horizons' here so that what is
// left is the propagation. Horizons converts with GM☉ = 1.3271244004127939e20
// m³/s², DE440's value, where kepler uses the IAU 2015 nominal one; the
// Keplerian GM is printed with the elements. And Horizons refers the elements
// to the J2000 ecliptic of the IAU 1976 obliquity, 84381.448″, where kepler
// rotates by IAU 2006's 84381.406″, a 0.042″ tilt that is #391. The comparison
// is made in each side's own ecliptic, so that difference cancels.
//
// Values are from Horizons ELEMENTS queries of 2026-09-23 (CENTER='@10',
// OBJ_DATA='YES'); the solution dates are those Horizons reported.
func TestFromPerihelionReproducesHorizonsOwnConversion(t *testing.T) {
	const (
		jplGM = 1.3271244004127939e20 // m³/s², Horizons' "Keplerian GM"

		// Measured: 8.9e-13 AU and 4e-16 AU/day for C/2023 A3, which is
		// where the sixteen printed digits run out.
		tolPos = 1e-11 // AU
		tolVel = 1e-13 // AU/day
	)

	eps2006 := constants.IAU.ObliquityJ2000.Value
	eps1976 := 84381.448 * math.Pi / 648000

	cases := []struct {
		el       cometElements
		epoch    float64 // JD TDB
		pos, vel vector.Vec3
	}{
		{
			// Solution 2026-Jul-28.
			cometElements{"C/2023 A3", 2460581.2408451759, 0.3914300748355564, 1.000095368540586,
				139.112109080566, 21.55947897244586, 308.4917649633916},
			2460448.5,
			vector.V3(-2.279899826191497e+00, -1.056123837697506e+00, -2.950480954489832e-01),
			vector.V3(1.050527699046718e-02, 1.111360900816833e-02, -3.617796927992962e-04),
		},
		{
			// Solution 2018-Jun-26.
			cometElements{"1I/'Oumuamua", 2458006.0073213754, 0.2559115812959116, 1.201133796102373,
				122.7417062847286, 24.59690955523242, 241.8105360304898},
			2458080.5,
			vector.V3(1.889136186533480e+00, 5.222899434623109e-01, 5.088057830311862e-01),
			vector.V3(2.106502285864550e-02, 3.535022471254437e-04, 8.998631872968255e-03),
		},
		{
			// Solution 2024-Jun-24.
			cometElements{"2I/Borisov", 2458826.052845906, 2.006520878500843, 3.356475782676596,
				44.05264247909138, 308.1477292269942, 209.1236864378081},
			2458853.5,
			vector.V3(-1.746422156220264e+00, 7.994315507603348e-01, -8.420898120636099e-01),
			vector.V3(-3.266387554200373e-03, -1.273246218308122e-02, -2.137606337596023e-02),
		},
		{
			// Solution 2026-Sep-21.
			cometElements{"2P/Encke", 2460240.0090567791, 0.3394821819631164, 0.8470279034259183,
				11.34227003837, 334.0454694778515, 187.2630430521377},
			2460062.5,
			vector.V3(2.527287865060516e+00, 1.988959670995109e-01, 3.947960681092019e-01),
			vector.V3(-8.983430485049088e-03, 3.762348840839456e-03, 1.573182324411714e-03),
		},
	}

	for _, tc := range cases {
		t.Run(tc.el.name, func(t *testing.T) {
			el := tc.el.build(t).WithCentralBody(kepler.CentralBody{ID: core.Sun, GM: jplGM})

			pos, vel, err := el.StateAt(time.FromJDParts(tc.epoch, 0, time.TDB))
			testutil.AssertNoError(t, err)

			dr := pos.RotateX(-eps2006).Sub(tc.pos.RotateX(-eps1976)).Norm()
			dv := vel.RotateX(-eps2006).Sub(tc.vel.RotateX(-eps1976)).Norm()

			if dr > tolPos || dv > tolVel {
				t.Errorf("|Δr| = %.3g AU, |Δv| = %.3g AU/day from Horizons' conversion; want below %g and %g",
					dr, dv, tolPos, tolVel)
			}
		})
	}
}

// TestFromPerihelionAgreesWithNewElementsOnAnEllipse checks the universal
// path against the eccentric-anomaly one on the orbit both can describe: an
// ellipse given as q, e and T is the one NewElements takes as a = q/(1−e) and
// M = 0 at T.
//
// The offsets reach several revolutions of Encke, which is what folds the
// interval by the period, and the two most eccentric orbits are also sampled
// just short of aphelion, where the solver's first step from below the root
// overshoots its bracket and it bisects instead.
//
// Agreement is relative, since aphelion at e = 0.999 is a thousand AU out.
// Near perihelion those two orbits are left out, because there the
// eccentric-anomaly path is the less accurate side: SolveKepler stops at
// 1e-12 rad, which at a = 500 AU is 5e-10 AU, and just before perihelion E
// sits near 2π, where it carries that angle's rounding rather than a small
// one's. The invariants, the continuity check and the two closed forms cover
// that region instead.
func TestFromPerihelionAgreesWithNewElementsOnAnEllipse(t *testing.T) {
	const tol = 1e-12

	shapes := []struct {
		cometElements

		nearPerihelion bool
	}{
		{encke, true},
		{ponsBrooks, true},
		{cometElements{"a near-circular orbit", 2460000.5, 2.7, 0.01, 10, 80, 73}, true},
		{cometElements{"a circular orbit", 2460000.5, 1.3, 0, 5, 40, 0}, true},
		{cometElements{"e = 0.99", 2460000.5, 0.5, 0.99, 30, 120, 250}, false},
		{cometElements{"e = 0.999", 2460000.5, 0.5, 0.999, 150, 200, 10}, false},
	}

	mu := muAUDay()

	for _, c := range shapes {
		t.Run(c.name, func(t *testing.T) {
			universal := c.build(t)

			a := c.q / (1 - c.e)
			classical, err := kepler.NewElements(universal.Epoch(), unit.AU(a), c.e,
				angle.Deg(c.incl), angle.Deg(c.node), angle.Deg(c.argPeri), angle.Zero())
			testutil.AssertNoError(t, err)

			period := 2 * math.Pi * math.Sqrt(a*a*a/mu)

			offsets := make([]float64, 0, 17)
			if c.nearPerihelion {
				offsets = append(offsets, -12000, -3000, -300, -30, -1, 0, 1, 30, 300, 3000, 12000)
			}

			for _, f := range []float64{0.45, 0.49, 0.499} {
				offsets = append(offsets, f*period, -f*period)
			}

			for _, d := range offsets {
				at := universal.Epoch().Add(unit.Days(d))

				p1, v1, err := universal.StateAt(at)
				testutil.AssertNoError(t, err)

				p2, v2, err := classical.StateAt(at)
				testutil.AssertNoError(t, err)

				dr, dv := p1.Sub(p2).Norm()/p2.Norm(), v1.Sub(v2).Norm()/v2.Norm()
				if dr > tol || dv > tol {
					t.Errorf("T%+g d: the two paths differ by %.3g of |r| and %.3g of |v|", d, dr, dv)
				}
			}
		})
	}
}

// TestFromPerihelionMatchesBarkersEquationOnAParabola compares e = 1 against
// Barker's equation, the parabola's closed-form solution: with
// W = 3·√(μ/2q³)·Δt, tan(ν/2) = y − 1/y where y = ∛(W/2 + √(W²/4 + 1)).
func TestFromPerihelionMatchesBarkersEquationOnAParabola(t *testing.T) {
	c := tsuchinshanATLAS
	c.name, c.e = "a parabola", 1

	el := c.build(t)
	mu := muAUDay()

	peri, v0, err := el.StateAt(el.Epoch())
	testutil.AssertNoError(t, err)

	h := peri.Cross(v0)

	for _, d := range []float64{-3000, -300, -100, -30, -1, 1, 30, 100, 300, 3000} {
		pos, _, err := el.StateAt(el.Epoch().Add(unit.Days(d)))
		testutil.AssertNoError(t, err)

		r, nu := inPlane(peri, pos, h)

		// Solved for |Δt| and the sign restored: for large negative W the
		// sum under the cube root cancels, and the reference, not the code
		// under test, would lose the digits.
		w := 3 * math.Sqrt(mu/(2*c.q*c.q*c.q)) * math.Abs(d)
		y := math.Cbrt(w/2 + math.Sqrt(w*w/4+1))
		s := math.Copysign(y-1/y, d)
		wantR, wantNu := c.q*(1+s*s), 2*math.Atan(s)

		if dr, dnu := math.Abs(r-wantR)/wantR, math.Abs(nu-wantNu); dr > 1e-13 || dnu > 1e-13 {
			t.Errorf("T%+g d: r = %.15g AU, ν = %.15g rad; Barker gives %.15g AU, %.15g rad", d, r, nu, wantR, wantNu)
		}
	}
}

// TestFromPerihelionMatchesTheHyperbolicKeplerEquation compares e > 1
// against the hyperbolic Kepler equation e·sinh H − H = M, solved on its own
// here, for two near-parabolic comets and the two interstellar objects.
//
// The reference is itself ill conditioned near e = 1: it forms
// a = q/(1−e), −16,900 AU for C/2022 E3, and subtracts to get r back. The
// tolerance is relative, and set by that side.
func TestFromPerihelionMatchesTheHyperbolicKeplerEquation(t *testing.T) {
	mu := muAUDay()

	for _, c := range []cometElements{tsuchinshanATLAS, ztf, oumuamua, borisov} {
		t.Run(c.name, func(t *testing.T) {
			el := c.build(t)

			peri, v0, err := el.StateAt(el.Epoch())
			testutil.AssertNoError(t, err)

			h := peri.Cross(v0)
			a := c.q / (1 - c.e)

			for _, d := range []float64{-3000, -300, -100, -30, -1, 1, 30, 100, 300, 3000} {
				pos, _, err := el.StateAt(el.Epoch().Add(unit.Days(d)))
				testutil.AssertNoError(t, err)

				r, nu := inPlane(peri, pos, h)

				m := math.Sqrt(mu/(-a*-a*-a)) * d
				hyp := math.Asinh(m / c.e)

				for range 200 {
					step := (c.e*math.Sinh(hyp) - hyp - m) / (c.e*math.Cosh(hyp) - 1)

					hyp -= step
					if math.Abs(step) < 1e-15*math.Max(1, math.Abs(hyp)) {
						break
					}
				}

				wantR := a * (1 - c.e*math.Cosh(hyp))
				wantNu := 2 * math.Atan(math.Sqrt((c.e+1)/(c.e-1))*math.Tanh(hyp/2))

				if dr, dnu := math.Abs(r-wantR)/wantR, math.Abs(nu-wantNu); dr > 1e-10 || dnu > 1e-10 {
					t.Errorf("T%+g d: r = %.15g AU, ν = %.15g rad; e·sinh H − H = M gives %.15g AU, %.15g rad",
						d, r, nu, wantR, wantNu)
				}
			}
		})
	}
}

// TestFromPerihelionConservesEnergyAndAngularMomentum holds every conic to its
// two integrals, v²/2 − μ/r = −μ(1−e)/2q and |r × v| = √(μq(1+e)), out to a
// thousand years either side of perihelion, where a hyperbola's universal
// anomaly is large enough to test the solver's bracket.
//
// The two synthetic hyperbolas are there for the solver's starting point. On
// e = 2 with q = 0.01 AU, a thousand years out, the equation grows like
// sinh, and Newton's method started from the upper bound of the bracket
// needed more than 200 steps; from the lower bound it takes three.
//
// Each integral is a small difference of large terms far from perihelion —
// the energy of a near-parabolic orbit is nearly zero, and r and v of an
// escaping body are nearly parallel — so each is held to a tolerance scaled
// by the size of the terms it cancels: v²/2 + μ/r for the energy, |r|·|v|
// for the angular momentum. Scaled that way, 1e-13 is tight enough to catch
// a universal anomaly solved to 1e-13 instead of to the last digit, which is
// how a bisection that discarded a converged Newton step was found.
func TestFromPerihelionConservesEnergyAndAngularMomentum(t *testing.T) {
	const tol = 1e-13

	mu := muAUDay()

	parabola := tsuchinshanATLAS
	parabola.name, parabola.e = "a parabola", 1

	shapes := []cometElements{
		tsuchinshanATLAS, ztf, oumuamua, borisov, encke, ponsBrooks, parabola,
		{"e = 2, q = 0.01 AU", 2460000.5, 0.01, 2, 60, 10, 300},
		{"e = 50", 2460000.5, 1, 50, 20, 250, 45},
	}

	for _, c := range shapes {
		t.Run(c.name, func(t *testing.T) {
			el := c.build(t)

			wantEnergy := -mu * (1 - c.e) / (2 * c.q)
			wantH := math.Sqrt(mu * c.q * (1 + c.e))

			for _, years := range []float64{-1000, -50, -1, -0.01, 0, 0.01, 1, 50, 1000} {
				pos, vel, err := el.StateAt(el.Epoch().Add(unit.Days(years * 365.25)))
				if err != nil {
					t.Fatalf("%+g years: %v", years, err)
				}

				kinetic, potential := vel.Dot(vel)/2, mu/pos.Norm()
				if d := math.Abs(kinetic-potential-wantEnergy) / (kinetic + potential); d > tol {
					t.Errorf("%+g years: energy %.15g, want %.15g (off by %.2g of v²/2 + μ/r)",
						years, kinetic-potential, wantEnergy, d)
				}

				if d := math.Abs(pos.Cross(vel).Norm()-wantH) / (pos.Norm() * vel.Norm()); d > tol {
					t.Errorf("%+g years: |r × v| off by %.2g of |r|·|v|", years, d)
				}
			}
		})
	}
}

// TestFromPerihelionIsContinuousAcrossTheParabola checks that nothing blows up
// as e crosses 1. An orbit a hair either side of the parabola is, over a
// hundred days, the parabola to within a distance proportional to the hair;
// the eccentric-anomaly formulation cannot do this, since it divides by 1−e.
func TestFromPerihelionIsContinuousAcrossTheParabola(t *testing.T) {
	c := tsuchinshanATLAS
	c.e = 1

	at := c.build(t).Epoch().Add(unit.Days(100))

	parabolaPos, _, err := c.build(t).StateAt(at)
	testutil.AssertNoError(t, err)

	for _, delta := range []float64{1e-3, 1e-6, 1e-9, 1e-12} {
		for _, e := range []float64{1 - delta, 1 + delta} {
			c.e = e

			pos, _, err := c.build(t).StateAt(at)
			testutil.AssertNoError(t, err)

			// Measured: about 4 AU of displacement per unit of e here.
			if d := pos.Sub(parabolaPos).Norm(); d > 10*delta+1e-13 {
				t.Errorf("e = 1%+g: %.3g AU from the parabola, want under %.3g", e-1, d, 10*delta+1e-13)
			}
		}
	}
}

// TestFromPerihelionStartsAtPerihelion checks the state at the perihelion
// time: at distance q, moving perpendicular to the radius at the vis-viva
// speed √(μ(1+e)/q).
func TestFromPerihelionStartsAtPerihelion(t *testing.T) {
	mu := muAUDay()

	for _, c := range []cometElements{tsuchinshanATLAS, oumuamua, borisov, encke} {
		el := c.build(t)

		pos, vel, err := el.StateAt(el.Epoch())
		testutil.AssertNoError(t, err)

		testutil.AssertRelNear(t, c.name+" |r(T)|", pos.Norm(), c.q, 1e-15)
		testutil.AssertRelNear(t, c.name+" |v(T)|", vel.Norm(), math.Sqrt(mu*(1+c.e)/c.q), 1e-15)
		testutil.AssertNear(t, c.name+" r·v at T", pos.Unit().Dot(vel.Unit()), 0, 1e-15)
	}
}

// TestNewElementsReportsAnOpenOrbitAsUnsupported is the side note in #374.
// The only way to hand MPC comet elements to NewElements was a = q/(1−e),
// which is negative for a hyperbola and infinite for a parabola, and Validate
// used to check the semi-major axis first: every real open orbit came back as
// ErrInvalidElements, indistinguishable from bad data.
func TestNewElementsReportsAnOpenOrbitAsUnsupported(t *testing.T) {
	c := tsuchinshanATLAS
	tp := time.FromJDParts(c.tp, 0, time.TDB)

	cases := []struct {
		name string
		a    unit.Length
		e    float64
	}{
		{"a hyperbola, a = q/(1−e) < 0", unit.AU(c.q / (1 - c.e)), c.e},
		{"a parabola, a = +Inf", unit.Length(math.Inf(1)), 1},
		{"a hyperbola with a placeholder a", unit.AU(1), c.e},
	}

	for _, tc := range cases {
		_, err := kepler.NewElements(tp, tc.a, tc.e,
			angle.Deg(c.incl), angle.Deg(c.node), angle.Deg(c.argPeri), angle.Zero())
		if !errors.Is(err, kepler.ErrUnsupportedOrbit) {
			t.Errorf("%s: NewElements = %v, want ErrUnsupportedOrbit", tc.name, err)
		}
	}
}

// TestFromPerihelionRejectsWhatNoOrbitHas checks the constructor's own
// validation.
func TestFromPerihelionRejectsWhatNoOrbitHas(t *testing.T) {
	tp := time.FromJDParts(2460000.5, 0, time.TDB)

	cases := []struct {
		name string
		q    unit.Length
		e    float64
		incl angle.Angle
		want error
	}{
		{"q = 0", 0, 1, angle.Zero(), kepler.ErrInvalidElements},
		{"q < 0", unit.AU(-0.5), 1, angle.Zero(), kepler.ErrInvalidElements},
		{"q = NaN", unit.Length(math.NaN()), 1, angle.Zero(), kepler.ErrInvalidElements},
		{"q = +Inf", unit.Length(math.Inf(1)), 1, angle.Zero(), kepler.ErrInvalidElements},
		{"e < 0", unit.AU(1), -0.1, angle.Zero(), kepler.ErrUnsupportedOrbit},
		{"e = NaN", unit.AU(1), math.NaN(), angle.Zero(), kepler.ErrUnsupportedOrbit},
		{"e = +Inf", unit.AU(1), math.Inf(1), angle.Zero(), kepler.ErrUnsupportedOrbit},
		{"inclination = NaN", unit.AU(1), 1.5, angle.Rad(math.NaN()), kepler.ErrInvalidElements},
	}

	for _, tc := range cases {
		_, err := kepler.FromPerihelion(tp, tc.q, tc.e, tc.incl, angle.Zero(), angle.Zero())
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: FromPerihelion = %v, want %v", tc.name, err, tc.want)
		}
	}
}

// TestAPeriodAppliesOnlyToAClosedOrbit checks WithPeriod on the perihelion
// form. On an ellipse it means what it means for NewElements, and the two
// paths agree; an orbit that does not close has no period, and StateAt says
// so rather than inventing a mean motion.
func TestAPeriodAppliesOnlyToAClosedOrbit(t *testing.T) {
	const periodDays = 1500.0

	universal := encke.build(t).WithPeriod(periodDays)

	classical, err := kepler.NewElements(universal.Epoch(), unit.AU(encke.q/(1-encke.e)), encke.e,
		angle.Deg(encke.incl), angle.Deg(encke.node), angle.Deg(encke.argPeri), angle.Zero())
	testutil.AssertNoError(t, err)

	classical = classical.WithPeriod(periodDays)

	for _, d := range []float64{-5000, -100, 0, 100, 5000} {
		at := universal.Epoch().Add(unit.Days(d))

		p1, _, err := universal.StateAt(at)
		testutil.AssertNoError(t, err)

		p2, _, err := classical.StateAt(at)
		testutil.AssertNoError(t, err)

		if dr := p1.Sub(p2).Norm(); dr > 1e-11 {
			t.Errorf("T%+g d with a %g-day period: the two paths differ by %.3g AU", d, periodDays, dr)
		}
	}

	_, _, err = oumuamua.build(t).WithPeriod(periodDays).StateAt(time.FromJDParts(oumuamua.tp, 0, time.TDB))
	testutil.AssertErrorIs(t, err, kepler.ErrInvalidElements)
}

// TestPerihelionFormAccessors checks what an element set built from
// perihelion reports about itself, for each conic, and PerihelionDistance on
// the mean-anomaly form too.
func TestPerihelionFormAccessors(t *testing.T) {
	hyperbola := oumuamua.build(t)
	testutil.AssertRelNear(t, "hyperbola a", hyperbola.SemiMajorAxis().AU(), oumuamua.q/(1-oumuamua.e), 1e-15)
	testutil.AssertRelNear(t, "hyperbola q", hyperbola.PerihelionDistance().AU(), oumuamua.q, 1e-15)
	testutil.AssertExact(t, "hyperbola M", hyperbola.MeanAnomaly().Radians(), 0)
	testutil.AssertExact(t, "hyperbola epoch", hyperbola.Epoch().JD(), time.FromJDParts(oumuamua.tp, 0, time.TDB).JD())

	if hyperbola.SemiMajorAxis() >= 0 {
		t.Errorf("a hyperbola's semi-major axis is %v; by convention it is negative", hyperbola.SemiMajorAxis())
	}

	c := tsuchinshanATLAS
	c.e = 1

	parabola := c.build(t)
	if a := parabola.SemiMajorAxis().AU(); !math.IsInf(a, 1) {
		t.Errorf("a parabola's semi-major axis is %v AU, want +Inf", a)
	}

	testutil.AssertRelNear(t, "parabola q", parabola.PerihelionDistance().AU(), c.q, 1e-15)

	ellipse := encke.build(t)
	testutil.AssertRelNear(t, "ellipse a", ellipse.SemiMajorAxis().AU(), encke.q/(1-encke.e), 1e-15)

	classical, err := kepler.NewElements(ellipse.Epoch(), unit.AU(2.5), 0.2,
		angle.Zero(), angle.Zero(), angle.Zero(), angle.Zero())
	testutil.AssertNoError(t, err)
	testutil.AssertRelNear(t, "NewElements q = a(1−e)", classical.PerihelionDistance().AU(), 2.0, 1e-15)
}

// TestProviderPropagatesAHyperbolicComet registers an open orbit with a
// Provider, which validates it, and reads a state back through the same
// interface every other body uses.
func TestProviderPropagatesAHyperbolicComet(t *testing.T) {
	const id core.ID = 1003639

	el := borisov.build(t)

	p := kepler.New()
	testutil.AssertNoError(t, p.Register(id, el))

	at := el.Epoch().Add(unit.Days(30))

	st, err := p.State(id, at)
	testutil.AssertNoError(t, err)

	helio, _, err := el.StateAt(at)
	testutil.AssertNoError(t, err)

	// The provider's state is geocentric; the comet is 2 AU from the Sun, so
	// the two positions differ by the Earth's heliocentric distance.
	if d := st.Pos.Sub(helio).Norm(); d < 0.97 || d > 1.03 {
		t.Errorf("geocentric and heliocentric positions differ by %.4f AU, want about 1", d)
	}
}
