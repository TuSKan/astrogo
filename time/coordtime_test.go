package time_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// TCG and TCB are each one rate scaling away from the scale they belong to, and
// a round trip cannot see a wrong rate constant: scaling out by the wrong
// factor and back by the same wrong factor closes perfectly. So the constants
// are checked against SOFA's own published values for iauTttcg, iauTcgtt,
// iauTdbtcb and iauTcbtdb, at the epoch SOFA's own test suite uses.
//
// The values below are SOFA's, to the digits SOFA states them to. They are
// quoted rather than derived: a value computed from the same formula under test
// would agree with it however wrong the formula was.

// sofaEpoch is JD 2453750.5, the date SOFA's t_sofa_c.c exercises every time
// scale conversion at.
const sofaEpoch = 2453750.5

// dayTolerance is a day fraction that bounds a nanosecond, which is finer than
// any of these scales is realised to and far finer than the millisecond-to-
// second offsets under test.
const dayTolerance = 1e-12

func TestTTToTCGMatchesSOFA(t *testing.T) {
	t.Parallel()

	// iauTttcg(2453750.5, 0.892482639) -> 0.8924900312508587113
	got := time.FromJDParts(sofaEpoch, 0.892482639, time.TT).TCG()

	assertJDParts(t, got, 0.8924900312508587113, "TT->TCG")

	if got.Scale() != time.TCG {
		t.Errorf("scale = %v, want TCG", got.Scale())
	}
}

func TestTCGToTTMatchesSOFA(t *testing.T) {
	t.Parallel()

	// iauTcgtt(2453750.5, 0.892862531) -> 0.8928551387488816828
	got := time.FromJDParts(sofaEpoch, 0.892862531, time.TCG).TT()

	assertJDParts(t, got, 0.8928551387488816828, "TCG->TT")
}

func TestTDBToTCBMatchesSOFA(t *testing.T) {
	t.Parallel()

	// iauTdbtcb(2453750.5, 0.892855137) -> 0.8930195997253656716
	got := time.FromJDParts(sofaEpoch, 0.892855137, time.TDB).TCB()

	assertJDParts(t, got, 0.8930195997253656716, "TDB->TCB")

	if got.Scale() != time.TCB {
		t.Errorf("scale = %v, want TCB", got.Scale())
	}
}

func TestTCBToTDBMatchesSOFA(t *testing.T) {
	t.Parallel()

	// iauTcbtdb(2453750.5, 0.893019599) -> 0.8928551362746343397
	got := time.FromJDParts(sofaEpoch, 0.893019599, time.TCB).TDB()

	assertJDParts(t, got, 0.8928551362746343397, "TCB->TDB")
}

// assertJDParts compares a converted time against SOFA's two-part result.
//
// wantFraction is the day fraction SOFA returns; the integer part is always
// [sofaEpoch], since every one of these conversions is a sub-second scaling.
//
// The comparison is on the sum rather than part by part: the split between jd1
// and jd2 is an internal precision device, and SOFA itself only promises the
// first part to 1e-6.
func assertJDParts(t *testing.T, got time.Time, wantFraction float64, what string) {
	t.Helper()

	g1, g2 := got.JDParts()

	if diff := math.Abs((g1 + g2) - (sofaEpoch + wantFraction)); diff > dayTolerance {
		t.Errorf("%s = %.15f + %.15f, want %.15f + %.15f (differ by %g days, %.3f ns)",
			what, g1, g2, sofaEpoch, wantFraction, diff, diff*86400e9)
	}
}

// TestTCGDriftFromTTIsTheDefinedRate checks the scaling against its definition
// rather than against a library, because the rate is the whole content of the
// scale.
//
// TCG − TT grows at L_G/(1−L_G) per unit of TT elapsed since 1977-01-01 TAI,
// where L_G = 6.969290134e-10 exactly (IAU 2000 Resolution B1.9). Over the
// 48 years to 2025 that is about 1.06 s.
func TestTCGDriftFromTTIsTheDefinedRate(t *testing.T) {
	t.Parallel()

	// The zero point: 1977-01-01T00:00:32.184 TAI, where TCG = TT by
	// definition. JD 2443144.5003725 in TT.
	const (
		zeroPoint = 2443144.5003725
		lg        = 6.969290134e-10
	)

	for _, years := range []float64{1, 10, 48} {
		jd := zeroPoint + years*365.25

		tt := time.FromJD(jd, time.TT)
		tcg := tt.TCG()

		g1, g2 := tcg.JDParts()
		t1, t2 := tt.JDParts()

		gotSeconds := ((g1 - t1) + (g2 - t2)) * 86400

		// TCG − TT = L_G/(1−L_G) × (TT elapsed since the zero point).
		wantSeconds := lg / (1 - lg) * (jd - zeroPoint) * 86400

		if diff := math.Abs(gotSeconds - wantSeconds); diff > 1e-6 {
			t.Errorf("after %v years TCG−TT = %.9f s, want %.9f s (differ by %g s)",
				years, gotSeconds, wantSeconds, diff)
		}
	}
}

// TestTCBDriftFromTDBIsFarLargerThanTCGs is the reason an ephemeris is argued
// in TDB and not in TCB, stated as a measurement.
//
// L_B is 22 times L_G, so the barycentric rate difference is half a second a
// year against 22 milliseconds. A caller who confuses the two scales is wrong
// by the larger number.
func TestTCBDriftFromTDBIsFarLargerThanTCGs(t *testing.T) {
	t.Parallel()

	const zeroPoint = 2443144.5003725

	jd := zeroPoint + 48*365.25 // roughly 2025

	tdb := time.FromJD(jd, time.TDB)
	tcb := tdb.TCB()

	b1, b2 := tcb.JDParts()
	d1, d2 := tdb.JDParts()

	drift := ((b1 - d1) + (b2 - d2)) * 86400

	// L_B = 1.550519768e-8 over 48 years is about 23.5 s.
	if drift < 20 || drift > 27 {
		t.Errorf("TCB−TDB after 48 years = %.3f s, want roughly 23.5 s.\n"+
			"  This is the offset that makes TCB unusable as an ephemeris "+
			"argument and TDB usable.", drift)
	}
}

// TestCoordinateTimesConvertToEveryOtherScale keeps the new scales from being
// reachable only from the one they are defined against.
//
// Every conversion method carries a switch whose default returns the time
// unchanged, so a scale nobody added a case for is silently passed through with
// the wrong label — which is exactly what happened to TAI here, and showed up
// as BDT->TCG->BDT losing 34.95 s at 1900.
func TestCoordinateTimesConvertToEveryOtherScale(t *testing.T) {
	t.Parallel()

	const jd = 2460000.5 // 2023-02-25, well inside the leap-second table

	for _, from := range []struct {
		scale time.Scale
		name  string
	}{
		{time.TCG, "TCG"},
		{time.TCB, "TCB"},
	} {
		t.Run(from.name, func(t *testing.T) {
			t.Parallel()

			src := time.FromJD(jd, from.scale)

			for _, to := range []struct {
				convert func(time.Time) time.Time
				name    string
				want    time.Scale
			}{
				{name: "UTC", want: time.UTC, convert: time.Time.UTC},
				{name: "TAI", want: time.TAI, convert: time.Time.TAI},
				{name: "TT", want: time.TT, convert: time.Time.TT},
				{name: "TDB", want: time.TDB, convert: time.Time.TDB},
				{name: "GPST", want: time.GPST, convert: time.Time.GPST},
				{name: "BDT", want: time.BDT, convert: time.Time.BDT},
			} {
				got := to.convert(src)
				if got.Scale() != to.want {
					t.Errorf("%s->%s produced scale %v", from.name, to.name, got.Scale())
				}

				// The conversion has to move the instant. A scale that fell
				// through an unhandled case comes back byte-identical, which
				// is the failure this test exists for.
				s1, s2 := src.JDParts()
				g1, g2 := got.JDParts()

				if (g1 + g2) == (s1 + s2) {
					t.Errorf("%s->%s left the date unchanged, so the conversion "+
						"fell through an unhandled case", from.name, to.name)
				}
			}
		})
	}
}
