package gofaext

// This file is in package gofaext rather than gofaext_test because the
// interesting claim is about the *unexported* distance-free path: that it
// agrees with SOFA in the regime where SOFA can answer, which is the only way
// to show it is a correct limit rather than a plausible substitute.

import (
	"math"
	"testing"

	"github.com/hebl/gofa"
)

// milliarcsecPerYear converts radians per year to mas/yr, the unit every
// assertion below is stated in.
const milliarcsecPerYear = 1000 * 3600 * 180 / math.Pi

// TestTheDistanceFreePathAgreesWithSOFAWhereSOFACanAnswer is the test that
// makes the rest of this file credible.
//
// The distance-free formulation exists for stars whose parallax SOFA cannot
// invert, where there is nothing to compare against. So it is checked in the
// opposite regime instead: for ordinary stars, where SOFA's pv route works
// perfectly well, the two must give the same answer.
//
// They are not identical, and should not be. SOFA's route is relativistic and
// accounts for the changing light-time that distorts a moving star's apparent
// proper motion; both terms are of order v/c and neither survives the division
// by distance. So the residual is expected to be small and to grow with the
// star's radial velocity — which is what the table below asserts, rather than a
// single tolerance that would hide the structure.
//
// Measured, and the tolerances are set a small multiple above these:
//
//	case                     separation   proper motion         radial velocity
//	at rest                  0"           8.9e-17 mas/yr        4.5e-08 km/s
//	no radial velocity       0"           4.4e-09 mas/yr        2.2e-07 km/s
//	nearby, rv = -110 km/s   2.3e-11"     3.5e-04 mas/yr        8.3e-07 km/s
//	distant, rv = +20 km/s   0"           5.6e-05 mas/yr        7.9e-04 km/s
//
// The bottom two rows are the physics; the top two are SOFA's own iauStarpv ↔
// iauPvstar round trip failing to close, which is why a star that is not
// moving still shows 4.5e-08 km/s.
//
// None of this is a reason to prefer one path for a star without a parallax:
// a light-time correction needs a distance, so for the stars this path serves
// those terms are not merely dropped, they are unobtainable.
func TestTheDistanceFreePathAgreesWithSOFAWhereSOFACanAnswer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name             string
		ra, dec          float64 // degrees
		pmRA, pmDec      float64 // mas/yr, on-sky and in declination
		parallax         float64 // arcsec
		rv               float64 // km/s
		tolMasPerYear    float64
		tolArcsecOnSky   float64
		tolKilometrePerS float64
	}{
		{
			name: "a nearby star with a large proper motion",
			ra:   123.4, dec: -35.6,
			pmRA: 3000, pmDec: -1500, // Barnard's-star scale
			parallax: 0.5, rv: -110,
			tolMasPerYear: 2e-3, tolArcsecOnSky: 1e-9, tolKilometrePerS: 5e-6,
		},
		{
			name: "a distant star with a modest proper motion",
			ra:   270, dec: 65,
			pmRA: 15, pmDec: -8,
			parallax: 1e-3, rv: 20,
			tolMasPerYear: 3e-4, tolArcsecOnSky: 1e-9, tolKilometrePerS: 5e-3,
		},
		{
			name: "no radial velocity, so no light-time term at all",
			ra:   10, dec: 0,
			pmRA: 500, pmDec: 500,
			parallax: 0.1, rv: 0,
			tolMasPerYear: 1e-7, tolArcsecOnSky: 1e-9, tolKilometrePerS: 1e-6,
		},
		{
			name: "at rest, where the two must agree to rounding",
			ra:   200, dec: -80,
			pmRA: 0, pmDec: 0,
			parallax: 0.02, rv: 0,
			tolMasPerYear: 1e-9, tolArcsecOnSky: 1e-9, tolKilometrePerS: 1e-6,
		},
	} {
		ra := tc.ra * math.Pi / 180
		dec := tc.dec * math.Pi / 180

		// The package's convention is dRA/dt, so the on-sky rate divides out
		// the cosine before going in.
		dr := (tc.pmRA / milliarcsecPerYear) / math.Cos(dec)
		dd := tc.pmDec / milliarcsecPerYear

		if !starpvIsExact(ra, dec, dr, dd, tc.parallax, tc.rv) {
			t.Fatalf("%s: SOFA cannot answer for this star, so it is the wrong case for this test", tc.name)
		}

		for _, dir := range []struct {
			name  string
			sofa  func() (float64, float64, float64, float64, float64, float64)
			angle func() (float64, float64, float64, float64, float64, float64)
		}{
			{
				name: "H2fk5",
				sofa: func() (r, d, pr, pd, px, rv float64) {
					gofa.H2fk5(ra, dec, dr, dd, tc.parallax, tc.rv, &r, &d, &pr, &pd, &px, &rv)
					return r, d, pr, pd, px, rv
				},
				angle: func() (float64, float64, float64, float64, float64, float64) {
					return h2fk5Angular(ra, dec, dr, dd, tc.parallax, tc.rv)
				},
			},
			{
				name: "Fk52h",
				sofa: func() (r, d, pr, pd, px, rv float64) {
					gofa.Fk52h(ra, dec, dr, dd, tc.parallax, tc.rv, &r, &d, &pr, &pd, &px, &rv)
					return r, d, pr, pd, px, rv
				},
				angle: func() (float64, float64, float64, float64, float64, float64) {
					return fk52hAngular(ra, dec, dr, dd, tc.parallax, tc.rv)
				},
			},
		} {
			sr, sd, spr, spd, _, srv := dir.sofa()
			ar, ad, apr, apd, _, arv := dir.angle()

			// Position, as a true on-sky separation so the cosine is handled.
			sep := math.Hypot((sr-ar)*math.Cos(sd), sd-ad) * 180 / math.Pi * 3600
			if sep > tc.tolArcsecOnSky {
				t.Errorf("%s/%s: positions %.3g arcsec apart, tolerance %.3g",
					tc.name, dir.name, sep, tc.tolArcsecOnSky)
			}

			// Proper motion, converted back to the on-sky rate.
			dPmRA := (spr - apr) * math.Cos(sd) * milliarcsecPerYear
			dPmDec := (spd - apd) * milliarcsecPerYear

			if math.Abs(dPmRA) > tc.tolMasPerYear || math.Abs(dPmDec) > tc.tolMasPerYear {
				t.Errorf("%s/%s: proper motion differs by (%.3g, %.3g) mas/yr, tolerance %.3g",
					tc.name, dir.name, dPmRA, dPmDec, tc.tolMasPerYear)
			}

			if d := math.Abs(srv - arv); d > tc.tolKilometrePerS {
				t.Errorf("%s/%s: radial velocity differs by %.3g km/s, tolerance %.3g",
					tc.name, dir.name, d, tc.tolKilometrePerS)
			}
		}
	}
}

// TestTheDistanceFreePathIsItsOwnInverse checks that the two directions undo
// each other exactly in the regime they exist for — a parallax of zero, where
// SOFA's route cannot go at all.
//
// This is the property a caller actually depends on: #331's second table was a
// star that went out and came back different, and a transformation pair that
// is not an inverse is how that happens.
func TestTheDistanceFreePathIsItsOwnInverse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		ra, dec     float64 // degrees
		pmRA, pmDec float64 // mas/yr
	}{
		{"an ordinary direction", 123.4, -35.6, 150, 220},
		{"at the RA wrap", 359.999, 12, -80, 40},
		{"on the equator", 45, 0, 500, -500},
		{"far south", 200, -85, 30, 30},
		{"at rest", 77, 33, 0, 0},
	} {
		ra := tc.ra * math.Pi / 180
		dec := tc.dec * math.Pi / 180
		dr := (tc.pmRA / milliarcsecPerYear) / math.Cos(dec)
		dd := tc.pmDec / milliarcsecPerYear

		// Out through one direction and straight back through the other, with
		// no parallax at any point.
		r5, d5, dr5, dd5, px5, rv5 := h2fk5Angular(ra, dec, dr, dd, 0, 0)
		rh, dh, drh, ddh, pxh, rvh := fk52hAngular(r5, d5, dr5, dd5, px5, rv5)

		sep := math.Hypot((rh-ra)*math.Cos(dec), dh-dec) * 180 / math.Pi * 3600
		if sep > 1e-9 {
			t.Errorf("%s: round trip moved the position by %.3g arcsec", tc.name, sep)
		}

		dPmRA := (drh - dr) * math.Cos(dec) * milliarcsecPerYear
		dPmDec := (ddh - dd) * milliarcsecPerYear

		if math.Abs(dPmRA) > 1e-9 || math.Abs(dPmDec) > 1e-9 {
			t.Errorf("%s: round trip moved the proper motion by (%.3g, %.3g) mas/yr",
				tc.name, dPmRA, dPmDec)
		}

		// A rotation moves nothing nearer or further and adds no radial
		// velocity, so both must come back untouched rather than merely close.
		if pxh != 0 || rvh != 0 {
			t.Errorf("%s: round trip produced parallax %g and radial velocity %g, want both zero",
				tc.name, pxh, rvh)
		}
	}
}

// TestTheAngularDecompositionIsExactlyInvertible covers the two helpers on
// their own, including the pole — where right ascension has no meaning and the
// rate of change of it therefore has none either.
func TestTheAngularDecompositionIsExactlyInvertible(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		ra, dec     float64 // degrees
		pmRA, pmDec float64 // mas/yr, on-sky
		atPole      bool
	}{
		{name: "mid sky", ra: 123.4, dec: -35.6, pmRA: 150, pmDec: 220},
		{name: "equator", ra: 0, dec: 0, pmRA: -40, pmDec: 90},
		{name: "high declination", ra: 88, dec: 89.9, pmRA: 10, pmDec: -10},
		{name: "north pole", ra: 0, dec: 90, pmRA: 0, pmDec: 55, atPole: true},
	} {
		ra := tc.ra * math.Pi / 180
		dec := tc.dec * math.Pi / 180
		dr := (tc.pmRA / milliarcsecPerYear) / math.Cos(dec)
		dd := tc.pmDec / milliarcsecPerYear

		if tc.atPole {
			// cos(dec) is not exactly zero at the float64 nearest to 90°, so
			// the input rate is set directly rather than divided.
			dr = 0
		}

		p, mu := starToAngular(ra, dec, dr, dd)

		// The direction is a unit vector and the rate is tangential to it:
		// both are what makes the distance cancel, and neither is asserted
		// anywhere else.
		if n := math.Hypot(math.Hypot(p[0], p[1]), p[2]); math.Abs(n-1) > 1e-15 {
			t.Errorf("%s: direction has norm %.17g, want 1", tc.name, n)
		}

		var radial float64
		for i := range mu {
			radial += mu[i] * p[i]
		}

		if math.Abs(radial) > 1e-18 {
			t.Errorf("%s: rate has a radial component of %.3g, want none", tc.name, radial)
		}

		gotRA, gotDec, gotDr, gotDd := angularToStar(p, mu)

		sep := math.Hypot((gotRA-ra)*math.Cos(dec), gotDec-dec) * 180 / math.Pi * 3600
		if sep > 1e-9 {
			t.Errorf("%s: position round trip moved %.3g arcsec", tc.name, sep)
		}

		if math.Abs((gotDd-dd)*milliarcsecPerYear) > 1e-9 {
			t.Errorf("%s: declination rate round trip moved %.3g mas/yr",
				tc.name, (gotDd-dd)*milliarcsecPerYear)
		}

		if math.Abs((gotDr-dr)*math.Cos(dec)*milliarcsecPerYear) > 1e-9 {
			t.Errorf("%s: right-ascension rate round trip moved %.3g mas/yr",
				tc.name, (gotDr-dr)*math.Cos(dec)*milliarcsecPerYear)
		}
	}
}

// TestAPolesEnormousRateIsFiniteAndComposesBack covers the one place the
// decomposition looks alarming.
//
// At the unit vector (0, 0, ±1) the divisor cos(dec) is 6.1e-17 — the float64
// nearest to +90° has no exact cosine — so dRA/dt comes back around 10⁸ radians
// a year for a star barely moving. The first instinct is to guard it to zero.
// That would be wrong twice over: the value is correct (meridians converge, so
// the coordinate really does sweep), and every consumer multiplies by the same
// cosine again, which restores the on-sky rate exactly.
//
// So what is asserted is what actually matters: the result is finite, and the
// composition is lossless. An earlier revision of this file did guard it, and
// the guard was dead code — math.Cos never returns exactly zero.
func TestAPolesEnormousRateIsFiniteAndComposesBack(t *testing.T) {
	t.Parallel()

	for _, z := range []float64{1, -1} {
		// A tangential rate at the pole, of order a few mas/yr in radians.
		mu := [3]float64{3e-8, -2e-8, 0}

		ra, dec, dr, dd := angularToStar([3]float64{0, 0, z}, mu)

		if math.IsInf(dr, 0) || math.IsNaN(dr) {
			t.Fatalf("z=%+g: dRA/dt = %v at a pole, want a finite number", z, dr)
		}

		if math.Abs(dr) < 1e6 {
			t.Errorf("z=%+g: dRA/dt = %g, expected the ~1e8 the converging meridians imply "+
				"— a small value here means something rounded the rate away", z, dr)
		}

		// C2s puts right ascension at zero when x and y are, so east is
		// (0, 1, 0) and the on-sky eastward rate the vector carries is mu[1].
		if ra != 0 {
			t.Fatalf("z=%+g: right ascension at a pole came back %g, want 0", z, ra)
		}

		// The composition every consumer performs: multiplying by the same
		// cosine restores the rate that went in, so nothing was lost by the
		// large intermediate value.
		if onSky := dr * math.Cos(dec); math.Abs(onSky-mu[1]) > 1e-24 {
			t.Errorf("z=%+g: dRA/dt·cos(dec) = %g, want the on-sky rate %g", z, onSky, mu[1])
		}

		// North at a pole is (-sin(dec), 0, ~0) with ra = 0, so it is (-1, 0, 0)
		// at the north pole and (+1, 0, 0) at the south. The declination rate
		// flips sign with the pole for that reason, and asserting one sign for
		// both is how this test failed on its first run.
		if wantDd := -z * mu[0]; math.Abs(dd-wantDd) > 1e-24 {
			t.Errorf("z=%+g: dDec/dt = %g, want %g", z, dd, wantDd)
		}
	}
}

// TestStarpvIsExactSeesWhatSOFAThrowsAway checks the predicate the dispatch
// turns on, against the two conditions #331 named.
func TestStarpvIsExactSeesWhatSOFAThrowsAway(t *testing.T) {
	t.Parallel()

	const (
		ra  = 2.1
		dec = -0.4
	)

	// 150 mas/yr in each component, in radians per year.
	pm := 150 / milliarcsecPerYear

	for _, tc := range []struct {
		name  string
		px    float64
		pm    float64
		exact bool
	}{
		{"an ordinary star", 0.05, pm, true},
		{"a distant star, still representable", 1e-3, pm, true},
		{"no parallax at all: the distance is overridden", 0, pm, false},
		{"below PXMIN: the distance is overridden", 1e-9, pm, false},
		{"at PXMIN with motion: the speed exceeds VMAX", 1e-7, pm, false},
		{"at PXMIN at rest: nothing to clamp", 1e-7, 0, true},

		// #339's band: above PXMIN, under VMAX, and iauStarpv reports complete
		// success — for a star that would be crossing the sky at 0.43c. None
		// of the conditions #331 named fires here, which is why the speed test
		// had to be added beside them. 2.4 mas/yr is what FK4's own fictitious
		// motion amounts to, so this is not a contrived rate.
		{"above PXMIN, status clean, star at 0.43c", 1e-7, 2.4 / milliarcsecPerYear, false},
		{"the same motion at a real distance", 1e-2, 2.4 / milliarcsecPerYear, true},
	} {
		if got := starpvIsExact(ra, dec, tc.pm, tc.pm, tc.px, 0); got != tc.exact {
			t.Errorf("%s (px=%g, pm=%g rad/yr): starpvIsExact = %v, want %v",
				tc.name, tc.px, tc.pm, got, tc.exact)
		}
	}
}

// TestTheExportedWrappersDispatchOnWhatSOFACanAnswer covers the front door
// itself, which every astrogo caller reaches and which the tests above bypass
// by calling the two paths directly.
//
// The property is that the dispatch is invisible: a caller passes star data and
// gets the right answer, whichever route it took. So each case asserts both
// which path ran — by comparing against that path computed directly — and that
// the other path was *not* the one used.
func TestTheExportedWrappersDispatchOnWhatSOFACanAnswer(t *testing.T) {
	t.Parallel()

	const (
		ra  = 2.1
		dec = -0.4
	)

	pm := 150 / milliarcsecPerYear

	for _, tc := range []struct {
		name     string
		px, rv   float64
		wantSOFA bool
	}{
		{"an ordinary star takes SOFA's route", 0.05, -30, true},
		{"no parallax at all takes the distance-free route", 0, 0, false},
		{"a parallax below PXMIN takes the distance-free route", 1e-9, 0, false},
		{"at PXMIN with real motion, the speed cap sends it the same way", 1e-7, 0, false},
	} {
		for _, dir := range []struct {
			name     string
			exported func() (float64, float64, float64, float64, float64, float64)
			angular  func() (float64, float64, float64, float64, float64, float64)
			sofa     func() (float64, float64, float64, float64, float64, float64)
		}{
			{
				name: "H2fk5",
				exported: func() (float64, float64, float64, float64, float64, float64) {
					return H2fk5(ra, dec, pm, pm, tc.px, tc.rv)
				},
				angular: func() (float64, float64, float64, float64, float64, float64) {
					return h2fk5Angular(ra, dec, pm, pm, tc.px, tc.rv)
				},
				sofa: func() (r, d, pr, pd, px, rv float64) {
					gofa.H2fk5(ra, dec, pm, pm, tc.px, tc.rv, &r, &d, &pr, &pd, &px, &rv)
					return r, d, pr, pd, px, rv
				},
			},
			{
				name: "Fk52h",
				exported: func() (float64, float64, float64, float64, float64, float64) {
					return Fk52h(ra, dec, pm, pm, tc.px, tc.rv)
				},
				angular: func() (float64, float64, float64, float64, float64, float64) {
					return fk52hAngular(ra, dec, pm, pm, tc.px, tc.rv)
				},
				sofa: func() (r, d, pr, pd, px, rv float64) {
					gofa.Fk52h(ra, dec, pm, pm, tc.px, tc.rv, &r, &d, &pr, &pd, &px, &rv)
					return r, d, pr, pd, px, rv
				},
			},
		} {
			gr, gd, gpr, gpd, _, _ := dir.exported()

			wr, wd, wpr, wpd, _, _ := dir.angular()
			if tc.wantSOFA {
				wr, wd, wpr, wpd, _, _ = dir.sofa()
			}

			if gr != wr || gd != wd || gpr != wpr || gpd != wpd {
				route := "the distance-free path"
				if tc.wantSOFA {
					route = "SOFA"
				}

				t.Errorf("%s/%s: result does not match %s computed directly", tc.name, dir.name, route)
			}

			// And the proper motion survives either way, which is the whole
			// point of #331 and the one thing a caller would notice.
			if math.Abs(gpr*milliarcsecPerYear-pm*milliarcsecPerYear) > 1 {
				t.Errorf("%s/%s: proper motion in RA came back %.4f mas/yr, want near %.4f",
					tc.name, dir.name, gpr*milliarcsecPerYear, pm*milliarcsecPerYear)
			}
		}
	}
}
