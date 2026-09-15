//go:build validation

package satellite

import (
	"math"
	"slices"
	"testing"
)

// Satellite.Verified makes a claim about which element sets astrogo can be
// trusted on, and the claim is only worth anything if it is measured against
// the data that established the regimes in the first place. These run against
// the checked-in Vallado suite for exactly that reason.

// divergence is the maximum position error measured per satellite, in km, for
// the seven cases astrogo could not reproduce.
//
// Re-measured after the propagator was corrected to WGS-72, which is what
// dropped the count from eight: satellite 29141's 0.62 km was the gravity-model
// mismatch and not a propagator defect at all, and it now agrees to 0.2 m.
// 23333 fell from 3.11 km to 0.22 km for the same reason. The five large ones
// moved by less than a part in a thousand, which is itself informative — those
// are the s4 and deep-space defects, and they do not care which Earth radius
// they are computed with.
var divergence = map[string]float64{
	"28350": 3438.51,
	"22312": 1828.92,
	"16925": 1328.37,
	"11801": 781.71,
	"28623": 485.94,
	"28872": 7.78,
	"23333": 0.2175,
}

// TestPerigeeReproducesValladosOwnFigures is the check that the arithmetic is
// SGP4's rather than merely near it.
//
// Vallado wrote perigee altitudes into the comments of his own test file for
// three cases. A two-body semi-major axis gets within about 1.4 km of them; the
// Kozai-to-Brouwer correction in initl is what closes the gap. Matching his
// numbers to a tenth of a kilometre is how this says the correction is right,
// rather than asserting the code against itself.
func TestPerigeeReproducesValladosOwnFigures(t *testing.T) {
	t.Parallel()

	tles := loadValladoTLEs(t)

	// The tolerance is the precision Vallado quoted, not one number for all
	// three: two of his figures carry two decimals and the third is written
	// "-51km", so holding that one to a tenth would be asserting precision the
	// source does not have.
	for _, tc := range []struct {
		satnum string
		want   float64 // km, from Vallado's own comment in SGP4-VER.TLE
		tol    float64
		note   string
	}{
		{"28350", 127.20, 0.05, `"Near Earth, perigee = 127.20 (< 156) s4 mod"`},
		{"28623", 135.75, 0.05, `"Deep space, perigee = 135.75 (<156) s4 mod"`},
		{"28872", -51.00, 1.00, `"(perigee = -51km), lost in 50 minutes" — quoted to the km`},
	} {
		sets, ok := tles[tc.satnum]
		if !ok || len(sets) == 0 {
			t.Fatalf("fixture no longer carries satellite %s", tc.satnum)
		}

		el, err := parseTLENumerics(sets[0][0][:tleLineLength], sets[0][1][:tleLineLength])
		if err != nil {
			t.Fatalf("%s: %v", tc.satnum, err)
		}

		got := perigeeAltitudeKM(el.meanMotion, el.ecc, el.inclRad)

		if math.Abs(got-tc.want) > tc.tol {
			t.Errorf("satellite %s: perigee %.2f km, Vallado says %.2f — %s.\n"+
				"  A gap near 1.4 km means the Kozai correction is missing and this is the "+
				"two-body value, which is not the number SGP4 branches on.",
				tc.satnum, got, tc.want, tc.note)
		}
	}
}

// TestVerifiedMatchesTheMeasuredDivergence asserts every figure in
// Satellite.Verified's doc comment against the fixtures.
//
// A predicate about trustworthiness that nobody checks is worse than none, and
// the numbers in that comment — flags ten, seven of them divergent, misses one
// — are exactly the kind of claim that rots. This recomputes them.
func TestVerifiedMatchesTheMeasuredDivergence(t *testing.T) {
	t.Parallel()

	tles := loadValladoTLEs(t)

	var (
		flagged      []string
		flaggedBad   int
		missed       []string
		verifiedGood int
	)

	for satnum, sets := range tles {
		// The three checksum-invalid cases never reach a Satellite at all.
		if checksumInvalid[satnum] {
			continue
		}

		sat, err := NewFromTLE(satnum, sets[0][0][:tleLineLength], sets[0][1][:tleLineLength])
		if err != nil {
			t.Errorf("%s: NewFromTLE: %v", satnum, err)

			continue
		}

		ok, reason := sat.Verified()
		_, diverges := divergence[satnum]

		switch {
		case !ok && diverges:
			flagged = append(flagged, satnum)
			flaggedBad++
		case !ok && !diverges:
			flagged = append(flagged, satnum)
		case ok && diverges:
			missed = append(missed, satnum)
		default:
			verifiedGood++
		}

		if ok && reason != "" {
			t.Errorf("%s: Verified reported true with a reason attached: %q", satnum, reason)
		}

		if !ok && reason == "" {
			t.Errorf("%s: Verified reported false with no reason", satnum)
		}
	}

	slices.Sort(flagged)
	slices.Sort(missed)

	t.Logf("flagged %d (%d of them divergent); missed %d; cleared %d",
		len(flagged), flaggedBad, len(missed), verifiedGood)

	if len(flagged) != 10 {
		t.Errorf("flagged %d cases %v, want 10 — the doc comment says ten", len(flagged), flagged)
	}

	if flaggedBad != 7 {
		t.Errorf("%d of the flagged cases actually diverge, want 7", flaggedBad)
	}

	// It misses nothing, and that is asserted rather than left implied: this
	// used to be `want exactly [29141]`, and 29141 stopped diverging the moment
	// the propagator was given the WGS-72 constants TLEs are fitted with. A
	// blind spot that disappears when an unrelated bug is fixed was never a
	// blind spot, and if one reappears the predicate needs revisiting.
	if len(missed) != 0 {
		t.Errorf("Verified cleared %v, which the suite says diverge — every divergent case "+
			"should be flagged", missed)
	}
}

// TestEveryLargeDivergenceIsFlagged is the property that actually matters, and
// it is stronger than the counts above.
//
// A caller can live with a conservative flag on an orbit that turns out fine.
// What they cannot live with is a 3440 km error reported as trustworthy. Every
// case in the suite that misses by more than 100 km must be flagged, and that
// is asserted separately so it cannot be traded away by tuning the threshold.
func TestEveryLargeDivergenceIsFlagged(t *testing.T) {
	t.Parallel()

	tles := loadValladoTLEs(t)

	const seriousKM = 100

	serious := 0

	for satnum, maxErr := range divergence {
		if maxErr < seriousKM {
			continue
		}

		serious++

		sets := tles[satnum]

		sat, err := NewFromTLE(satnum, sets[0][0][:tleLineLength], sets[0][1][:tleLineLength])
		if err != nil {
			t.Fatalf("%s: %v", satnum, err)
		}

		if ok, _ := sat.Verified(); ok {
			t.Errorf("satellite %s misses Vallado by %.0f km and Verified reports it as trustworthy.\n"+
				"  A silent kilometre-scale error is the whole reason this predicate exists.",
				satnum, maxErr)
		}
	}

	if serious != 5 {
		t.Errorf("the suite has %d divergences over %d km, want 5", serious, seriousKM)
	}
}

// TestDeepSpaceIsDeliberatelyNotACondition makes the negative decision checkable.
//
// Flagging deep space looks obviously right and the measurement refutes it: the
// deep-space cases in this suite that agree outnumber the ones that do not by
// about three to one, and every divergent one is already caught by its perigee.
// If somebody adds the condition later, this fails and says why.
func TestDeepSpaceIsDeliberatelyNotACondition(t *testing.T) {
	t.Parallel()

	tles := loadValladoTLEs(t)

	// SGP4's own deep-space branch: a period of 225 minutes or more.
	const deepSpaceMinutes = 225

	deepAndFine := 0

	for satnum, sets := range tles {
		if checksumInvalid[satnum] {
			continue
		}

		sat, err := NewFromTLE(satnum, sets[0][0][:tleLineLength], sets[0][1][:tleLineLength])
		if err != nil {
			continue
		}

		if sat.OrbitalPeriod() < deepSpaceMinutes {
			continue
		}

		if _, diverges := divergence[satnum]; diverges {
			continue
		}

		// A deep-space case can still be flagged, for its perigee — 23599 is
		// deep space with a 180 km perigee and is one of the three known
		// conservative flags. What must not happen is a case being flagged for
		// being deep space, so only those clear of the perigee band are
		// asserted on.
		if perigeeAltitudeKM(sat.MeanMotion, sat.ecc, sat.inclRad) < simplifiedDragPerigeeKM {
			continue
		}

		if ok, reason := sat.Verified(); !ok {
			t.Errorf("satellite %s is deep space, has a perigee above the band, agrees with "+
				"Vallado, and is flagged anyway: %s", satnum, reason)
		}

		deepAndFine++
	}

	if deepAndFine < 10 {
		t.Errorf("only %d deep-space cases both agree and are cleared; the argument for leaving "+
			"deep space out of the predicate rests on there being many", deepAndFine)
	}

	t.Logf("%d deep-space cases agree with Vallado and are correctly not flagged", deepAndFine)
}
