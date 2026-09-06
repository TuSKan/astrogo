package time_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// A negative leap second has never happened, and that is the reason to test it.
//
// The ITU-R has permitted one since 1972 — realised by skipping 23:59:59, so the
// day advances from 23:59:58 straight to 00:00:00 — and none has occurred in
// fifty-odd years of leap seconds, every one of which has been positive. Levine,
// Tavella & Milton (2023) say recent Earth-rotation data make one "no longer
// simply an academic possibility", project one "by about the year 2030", and
// warn specifically that because they "have never happened ... it is almost a
// certainty that there will be widespread errors in realizing the event".
//
// Fifty years of one-sided history is exactly what writes an assumption into
// code without anyone deciding to. One was already found here: the record's own
// well-formedness test asserted every step was +1 s, and would have rejected the
// first correct table containing a negative entry (#148).
//
// # What these can and cannot cover
//
// The ΔAT record and the scale conversions can be exercised now, and are, below.
// What cannot is the *representation*: on the day of a negative leap second
// 23:59:59 does not exist, and [time.Time] has no way to say that a civil second
// is absent — the mirror image of #144, where it has no way to say 23:59:60 is
// present. Both need the same decision, and the negative one arrives first if
// the 2030 projection holds. See #147.

// negativeAt2030 is the published record extended by a hypothetical negative
// leap second at the end of 2029: ΔAT drops from 37 to 36.
//
// Whether one is ever announced for that date is beside the point. What is
// pinned is that an announced one would be carried, and would convert.
func negativeAt2030() []time.LeapSecond {
	return extended(time.LeapSecond{Year: 2030, Month: 1, Day: 1, DeltaAT: 36})
}

// TestNegativeLeapSecondReachesTheScaleConversions is the half that matters.
//
// Registering a table is worth nothing if ΔAT does not reach the arithmetic, and
// a −1 step exercises a direction of the leap-second lookup that fifty years of
// positive steps never have.
//
// TestRegisterLeapSecondsAcceptsANegativeLeapSecond already covers registration
// and the TAI value. What is new here is the before/after contrast — ΔAT going
// down across a step, which nothing has ever asked it to do — and the TT branch,
// which reaches ΔAT by its own route and has been fixed one-sidedly before.
func TestNegativeLeapSecondReachesTheScaleConversions(t *testing.T) {
	defer time.ResetLeapSeconds()

	before := utc(2029, time.June, 1)
	after := utc(2030, time.June, 1)

	if got := taiMinusUTC(t, after); got != 37 {
		t.Fatalf("precondition: built-in table gives ΔAT = %g at 2030, want 37", got)
	}

	if err := time.RegisterLeapSeconds(negativeAt2030(), "hypothetical-negative"); err != nil {
		t.Fatalf("RegisterLeapSeconds: %v", err)
	}

	// ΔAT goes DOWN across the step, which has never happened before.
	if got := taiMinusUTC(t, before); got != 37 {
		t.Errorf("TAI−UTC before the step = %g s, want 37", got)
	}

	if got := taiMinusUTC(t, after); got != 36 {
		t.Errorf("TAI−UTC after a negative leap second = %g s, want 36.\n"+
			"  The registered table steps down and the conversion did not follow it.", got)
	}

	// TT reaches ΔAT by its own branch, so it is checked separately — the same
	// split that caught a one-sided fix before.
	if got, want := ttMinusUTC(t, after), 36+32.184; math.Abs(got-want) > 1e-6 {
		t.Errorf("TT−UTC after the step = %g s, want %g s.\n"+
			"  ΔAT reached the TAI conversion but not the TT one.", got, want)
	}
}

// TestNegativeLeapSecondKeepsUTCAndTAIInverse is the property a sign error most
// easily breaks, and the one a user notices as a wrong position.
//
// UTC→TAI→UTC must return the instant it started from whichever way the step
// went. A conversion that assumed steps are positive would come back two seconds
// out here — the size of the step, applied the wrong way.
func TestNegativeLeapSecondKeepsUTCAndTAIInverse(t *testing.T) {
	defer time.ResetLeapSeconds()

	if err := time.RegisterLeapSeconds(negativeAt2030(), "hypothetical-negative"); err != nil {
		t.Fatalf("RegisterLeapSeconds: %v", err)
	}

	for _, when := range []time.Time{
		utc(2029, time.December, 1),
		utc(2030, time.January, 2),
		utc(2031, time.June, 1),
	} {
		back := when.TAI().UTC()

		w1, w2 := when.JDParts()
		b1, b2 := back.JDParts()

		if d := math.Abs((b1-w1)+(b2-w2)) * 86400; d > 1e-6 {
			t.Errorf("at %s: UTC→TAI→UTC moved the instant by %.9f s",
				when.Format(time.DateOnly), d)
		}
	}
}

// TestRegistryStillRefusesAStepThatIsNotOneSecond keeps the widened rule from
// having widened into nothing.
//
// #148 changed the record check from "every step is +1" to "|step| is 1". The
// point of that is to admit −1, not to admit anything: a table stepping by 2, or
// by 0, is still a corrupt record and must still be refused. Without this, the
// fix for one-sidedness would have removed the check altogether.
func TestRegistryStillRefusesAStepThatIsNotOneSecond(t *testing.T) {
	defer time.ResetLeapSeconds()

	// An upward two-second step is already covered by
	// TestRegisterLeapSecondsRejectsAMalformedTable. These are the shapes the
	// widened rule could newly have let through: the same jump downward, a
	// step of zero, and a fractional one.
	for _, tc := range []struct {
		name    string
		deltaAT float64
	}{
		{"a two-second step downward", 35},
		{"no step at all", 37},
		{"a fractional step", 37.5},
	} {
		err := time.RegisterLeapSeconds(
			extended(time.LeapSecond{Year: 2030, Month: 1, Day: 1, DeltaAT: tc.deltaAT}),
			"malformed",
		)

		if !errors.Is(err, time.ErrLeapSecondOrder) {
			t.Errorf("%s (ΔAT %g) was accepted, err = %v.\n"+
				"  Admitting a negative leap second must not admit an arbitrary jump: a "+
				"leap second is ±1 s and nothing else.", tc.name, tc.deltaAT, err)
		}
	}
}
