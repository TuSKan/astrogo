package atmosphere

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// TestRefractionIsNeverNegative sweeps every empirical model over the whole
// range it is evaluated on and asserts the one property refraction cannot
// violate.
//
// # What went wrong
//
// The additive term that keeps each fit stable near the horizon carries its
// tangent argument past 90° near the zenith — Saemundsson's h + 10.3/(h+5.11)
// reaches 90.108° at h = 90°, Bennett's h + 7.31/(h+4.4) reaches 90.077°. tan
// is negative just beyond its asymptote, so both formulas returned negative
// refraction: −0.114″ and −0.080″ at the zenith, crossing zero at 89.8916° and
// 89.9225°.
//
// Small, but wrong in a way a magnitude check cannot see. Refraction raises an
// object or, at the zenith, does nothing; it never lowers one.
//
// # Why a sweep rather than a few points
//
// The defect lived in the last tenth of a degree of a 94° range. Picking
// altitudes by hand is how it survived: the existing table has a zenith_90
// case, and it passed. Stepping finely across the whole domain is what makes
// the guard independent of guessing where the next one will be.
func TestRefractionIsNeverNegative(t *testing.T) {
	env := StandardRefraction

	models := []struct {
		name string
		m    RefractionModel
	}{
		{"RefractionApproximate", RefractionApproximate{}},
		{"RefractionRigorous", RefractionRigorous{}},
		{"RefractionSOFA", RefractionSOFA{}},
		{"RefractionNone", RefractionNone{}},
	}

	directions := []struct {
		name string
		f    func(RefractionModel, angle.Angle) angle.Angle
	}{
		{"RefractFromTrue", func(m RefractionModel, a angle.Angle) angle.Angle {
			return m.RefractFromTrue(a, env)
		}},
		{"RefractFromApparent", func(m RefractionModel, a angle.Angle) angle.Angle {
			return m.RefractFromApparent(a, env)
		}},
	}

	for _, model := range models {
		for _, dir := range directions {
			t.Run(model.name+"/"+dir.name, func(t *testing.T) {
				// 0.001° steps through the region where the sign flipped, and
				// across the whole evaluated range besides.
				for h := lowAltitudeCutoffDeg - 1.0; h <= 90.0; h += 0.001 {
					got := dir.f(model.m, angle.Deg(h))
					if got < 0 {
						t.Fatalf("at %.4f deg altitude: %.6f arcsec.\n"+
							"  Refraction is never negative — check the tangent "+
							"argument against zenithArgumentLimit.",
							h, got.Arcseconds())
					}
				}
			})
		}
	}
}

// TestRefractionVanishesAtTheZenith pins the physical limit.
//
// Light arriving along the normal is not bent, so refraction at the zenith is
// zero. After clamping the tangent argument, both empirical models return a
// literal zero there; RefractionSOFA returns 5.7e-5 arcsec, because iauAtioq
// clamps cos(altitude) at celMin rather than letting it reach zero. Both are
// correct — the distinction is carried by the exact field below rather than
// papered over with one loose tolerance.
func TestRefractionVanishesAtTheZenith(t *testing.T) {
	env := StandardRefraction

	for _, model := range []struct {
		name string
		m    RefractionModel
		// exact is true for the models that return a literal zero. SOFA does
		// not: iauAtioq clamps cos(altitude) away from zero at celMin = 1e-6,
		// so the series evaluates to 5.7e-5 arcsec rather than nothing. That
		// is SOFA's own behaviour, reproduced on purpose, and 57 microarcsec
		// is eleven orders below anything this library claims.
		exact bool
	}{
		{"RefractionApproximate", RefractionApproximate{}, true},
		{"RefractionRigorous", RefractionRigorous{}, true},
		{"RefractionSOFA", RefractionSOFA{}, false},
	} {
		t.Run(model.name, func(t *testing.T) {
			const negligibleArcsec = 1e-3

			for _, dir := range []struct {
				name string
				got  angle.Angle
			}{
				{"RefractFromTrue", model.m.RefractFromTrue(angle.Deg(90), env)},
				{"RefractFromApparent", model.m.RefractFromApparent(angle.Deg(90), env)},
			} {
				switch {
				case model.exact && dir.got != 0:
					t.Errorf("%s(90 deg) = %.6f arcsec, want exactly 0",
						dir.name, dir.got.Arcseconds())
				case dir.got < 0:
					t.Errorf("%s(90 deg) = %.9f arcsec, which is negative",
						dir.name, dir.got.Arcseconds())
				case dir.got.Arcseconds() > negligibleArcsec:
					t.Errorf("%s(90 deg) = %.9f arcsec, want under %g",
						dir.name, dir.got.Arcseconds(), negligibleArcsec)
				}
			}
		})
	}
}

// TestZenithClampCostsLessThanTheModelsOwnAccuracy bounds what the clamp
// throws away, so "return zero up there" is a measured decision rather than an
// assumption.
//
// The clamp takes effect exactly where each formula crosses zero, so the
// largest value it discards is the one the formula gives immediately below the
// crossing. Both fits quote about 0.1 arcmin (6 arcsec) of accuracy, so the
// discarded amount has to be far under that to be free.
func TestZenithClampCostsLessThanTheModelsOwnAccuracy(t *testing.T) {
	env := StandardRefraction

	// The quoted accuracy of the empirical fits, in arcseconds.
	const quotedAccuracy = 0.1 * 60.0

	for _, tc := range []struct {
		name       string
		f          func(angle.Angle) angle.Angle
		crossingAt float64 // measured by bisection before the clamp existed
	}{
		{"Saemundsson (RefractFromTrue)", func(a angle.Angle) angle.Angle {
			return RefractionRigorous{}.RefractFromTrue(a, env)
		}, 89.8916},
		{"Bennett (RefractFromApparent)", func(a angle.Angle) angle.Angle {
			return RefractionRigorous{}.RefractFromApparent(a, env)
		}, 89.9225},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Just inside the fitted range: the most this clamp can discard.
			discarded := tc.f(angle.Deg(tc.crossingAt - 0.001)).Arcseconds()

			if discarded < 0 {
				t.Fatalf("the formula is already negative %.3f deg below the "+
					"recorded crossing; the crossing has moved", 0.001)
			}

			if discarded > quotedAccuracy/10 {
				t.Errorf("clamping discards %.4f arcsec, which is not negligible "+
					"against the model's own %.1f arcsec accuracy", discarded, quotedAccuracy)
			}

			// And immediately above the crossing it is clamped, not evaluated.
			if got := tc.f(angle.Deg(tc.crossingAt + 0.001)); got != 0 {
				t.Errorf("above the crossing: %.6f arcsec, want exactly 0",
					got.Arcseconds())
			}

			t.Logf("clamp discards at most %.4f arcsec, against %.1f arcsec quoted",
				discarded, quotedAccuracy)
		})
	}
}

// TestZenithClampLeavesTheUsefulRangeUntouched checks the fix is local: every
// altitude a real observation cares about must return what it did before.
func TestZenithClampLeavesTheUsefulRangeUntouched(t *testing.T) {
	env := StandardRefraction

	// Values recorded on main before the clamp, in arcseconds.
	for _, tc := range []struct {
		altDeg float64
		want   float64
	}{
		{89, 0.937318},
		{88, 1.989153},
		{85, 5.154335},
		{80, 10.501172},
		{60, 34.592363},
		{45, 59.868502},
		{30, 103.217851},
		{10, 319.687275},
	} {
		got := RefractionRigorous{}.RefractFromTrue(angle.Deg(tc.altDeg), env).Arcseconds()
		if math.Abs(got-tc.want) > 1e-5 {
			t.Errorf("at %.0f deg: %.4f arcsec, want %.4f — the clamp reached "+
				"below the zenith region", tc.altDeg, got, tc.want)
		}
	}
}
