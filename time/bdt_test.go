package time_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// The BeiDou tests share epoch(), labelSeconds() and secondsPerDay with the
// GPS ones in gpst_test.go: the two scales are the same construction with a
// different constant, and testing them apart from the same helpers is what
// makes the constant the only thing that differs.

// TestBDTOffsetFromTAIIsExactlyThirtyThreeSeconds pins the definition.
//
// BeiDou was set to UTC on 2006-01-01, when TAI−UTC was 33 s, and has run at
// the TAI rate ignoring leap seconds ever since. So TAI − BDT is 33 s at every
// epoch, not a table lookup.
func TestBDTOffsetFromTAIIsExactlyThirtyThreeSeconds(t *testing.T) {
	t.Parallel()

	for _, when := range []time.Time{
		epoch(),
		time.Date(2006, time.January, 1, 0, 0, 0, 0, time.LocationUTC), // the synchronisation
		time.Date(1999, time.June, 1, 0, 0, 0, 0, time.LocationUTC),    // before it
		time.Date(2040, time.June, 1, 0, 0, 0, 0, time.LocationUTC),    // past the table
	} {
		got := labelSeconds(when.TAI(), when.BDT())
		if math.Abs(got-33) > 1e-9 {
			t.Errorf("at %v: TAI − BDT = %.9f s, want exactly 33", when, got)
		}
	}
}

// TestBDTIsFourSecondsAheadOfUTCToday: unlike the TAI offset, this one moves,
// and it moves at every leap second. ΔAT is 37 s today, so BDT − UTC is 4 s.
func TestBDTIsFourSecondsAheadOfUTCToday(t *testing.T) {
	t.Parallel()

	utc := epoch()

	got := labelSeconds(utc.BDT(), utc)
	if math.Abs(got-4) > 1e-9 {
		t.Errorf("BDT − UTC = %.9f s, want 4 (ΔAT 37 − 33)", got)
	}
}

// TestBDTAndGPSTDifferByFourteenSecondsForEver is the property that makes two
// scales necessary rather than one.
//
// Both are TAI minus a frozen constant, so the gap between them is the
// difference of those constants and cannot move — not at a leap second, not
// ever. A caller handed a timestamp from a dual-constellation receiver has two
// readings fourteen seconds apart and nothing in either number saying which is
// which.
func TestBDTAndGPSTDifferByFourteenSecondsForEver(t *testing.T) {
	t.Parallel()

	for _, when := range []time.Time{
		epoch(),
		time.Date(2006, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2016, time.December, 31, 12, 0, 0, 0, time.LocationUTC), // a leap-second day
		time.Date(2040, time.June, 1, 0, 0, 0, 0, time.LocationUTC),
	} {
		got := labelSeconds(when.GPST(), when.BDT())
		if math.Abs(got-14) > 1e-9 {
			t.Errorf("at %v: GPST − BDT = %.9f s, want exactly 14", when, got)
		}
	}
}

// TestBDTNamesTheSameInstantAsItsUTC is the contract that matters to a caller.
//
// The label differs by four seconds; the instant does not. Sub converts to a
// common scale before subtracting, so a zero here means the conversion put the
// BeiDou timestamp where the UTC one already was — which is the whole reason
// to express the scale rather than leave the caller to subtract.
func TestBDTNamesTheSameInstantAsItsUTC(t *testing.T) {
	t.Parallel()

	utc := epoch()

	if got := utc.BDT().Sub(utc); got != 0 {
		t.Errorf("BDT(t).Sub(t) = %v, want 0 — the same instant, differently labelled", got)
	}

	// And the other direction, which is the one a receiver forces: a caller
	// builds an instant declaring BDT and asks for UTC.
	//
	// Through FromJDParts rather than FromJD. Collapsing the two-part JD into
	// one float64 costs about 40 microseconds at a modern epoch, which is
	// nothing physically and is enough to make an exact assertion fail — the
	// same reason labelSeconds subtracts componentwise.
	jd1, jd2 := utc.BDT().JDParts()
	bdt := time.FromJDParts(jd1, jd2, time.BDT)

	if got := bdt.UTC().Sub(utc); got != 0 {
		t.Errorf("a BDT-declared instant converted to UTC is %v from the UTC it denotes, want 0", got)
	}
}

// TestBDTMislabelledAsUTCIsFourSecondsWrong measures the defect the scale
// exists to prevent, in the units a satellite user cares about.
//
// Four seconds is the offset most easily read as a rounding difference, and at
// the ISS's 7.66 km/s it is 30 km of ground track.
func TestBDTMislabelledAsUTCIsFourSecondsWrong(t *testing.T) {
	t.Parallel()

	utc := epoch()
	receiver := utc.BDT()

	// What a caller does today with no BDT scale: take the receiver's numbers
	// and call them UTC. Deliberately through FromJD, since a single JD is the
	// shape that number usually arrives in — which costs about 40 microseconds
	// of resolution and is why the tolerance is a millisecond rather than
	// exact.
	mislabelled := time.FromJD(receiver.JD(), time.UTC)

	got := mislabelled.Sub(utc).Seconds()
	if math.Abs(got-4) > 1e-3 {
		t.Errorf("a BDT timestamp passed as UTC lands %.6f s away, want 4", got)
	}
}

// TestBDTRoundTripsThroughEveryScale: BDT is uniform, so every conversion out
// of it and back has to return the instant it started from. The matrix test
// covers this across epochs; this is the same property stated where a reader
// looking for BDT will find it.
func TestBDTRoundTripsThroughEveryScale(t *testing.T) {
	t.Parallel()

	start := epoch().BDT()

	for _, tc := range []struct {
		name string
		trip func(time.Time) time.Time
	}{
		{"UTC", func(x time.Time) time.Time { return x.UTC().BDT() }},
		{"TAI", func(x time.Time) time.Time { return x.TAI().BDT() }},
		{"TT", func(x time.Time) time.Time { return x.TT().BDT() }},
		{"TDB", func(x time.Time) time.Time { return x.TDB().BDT() }},
		{"GPST", func(x time.Time) time.Time { return x.GPST().BDT() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			back := tc.trip(start)
			if back.Scale() != time.BDT {
				t.Fatalf("round trip through %s came back as %v", tc.name, back.Scale())
			}

			if got := math.Abs(labelSeconds(back, start)); got > 1e-6 {
				t.Errorf("round trip through %s moved the label by %.9f s", tc.name, got)
			}
		})
	}
}

// One mutation survives on purpose, and it is the same one gpst_test.go
// records: dropping BDT from Scale.uniform() leaves every test here passing.
//
// uniform() is consulted in exactly one place — SubDays, to decide whether to
// convert to TT before subtracting — and for BDT both routes give the same
// answer, because BDT-to-TT is a constant that cancels in a difference. The
// marking is still correct: UTC and UT1 are non-uniform because their labels do
// not track SI seconds and BDT's do, and saying so keeps the predicate meaning
// one thing rather than becoming a list of scales that happen to want the fast
// path.

// TestBDTStringIsItsName: the scale is printed in error messages and test
// output, and "UNKNOWN" there would be worse than useless — it would suggest
// the value is corrupt rather than newly added.
func TestBDTStringIsItsName(t *testing.T) {
	t.Parallel()

	if got := time.BDT.String(); got != "BDT" {
		t.Errorf("BDT.String() = %q, want %q", got, "BDT")
	}
}
