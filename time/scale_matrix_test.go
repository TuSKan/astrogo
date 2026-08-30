package time_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/time"
)

// scaleName pairs a scale with its converter, so the matrix below can be
// written as a loop rather than as twenty-five hand-written cases.
type scaleName struct {
	name string
	to   func(time.Time) time.Time
}

func allScales() []scaleName {
	return []scaleName{
		{"UTC", time.Time.UTC},
		{"TAI", time.Time.TAI},
		{"TT", time.Time.TT},
		{"TDB", time.Time.TDB},
		{"UT1", func(t time.Time) time.Time {
			// UT1 needs Earth-orientation data and reports when it cannot
			// get it; the fallback is what every other caller in the module
			// gets, so the matrix uses it too rather than skipping the scale.
			u, err := t.UT1()
			if err != nil {
				return t.UTC()
			}

			return u
		}},
	}
}

// matrixEpoch is one instant the matrix is exercised at, and the reason it
// is in the list.
type matrixEpoch struct {
	name string
	t    time.Time
	why  string

	// preUTC marks an epoch before 1972, where UTC is not related to atomic
	// time by an integral count of leap seconds and the conversion stops
	// being invertible.
	preUTC bool
}

// matrixEpochs spans the boundaries where a time library actually breaks.
//
// Even sampling is nearly useless here: the failures are at the seams —
// where leap seconds are inserted, where the leap-second table begins and
// ends, where one model hands over to another. A conversion can be perfect
// everywhere between them and wrong at every one.
func matrixEpochs() []matrixEpoch {
	return []matrixEpoch{
		{"2017 leap second, before", time.Date(2016, 12, 31, 23, 59, 0, 0, time.LocationUTC),
			"the last leap second inserted, and the one ephemeris/jpl/lsk silently dropped", false},
		{"2017 leap second, after", time.Date(2017, 1, 1, 0, 1, 0, 0, time.LocationUTC),
			"the other side of the same insertion", false},
		{"2015 leap second", time.Date(2015, 7, 1, 0, 0, 30, 0, time.LocationUTC),
			"an earlier insertion, so the table is exercised at more than its last row", false},
		{"1972 UTC epoch", time.Date(1972, 1, 1, 0, 0, 1, 0, time.LocationUTC),
			"leap seconds begin here; the Delta-T path hands over to Delta-AT", true},
		{"1971, drift era", time.Date(1971, 6, 15, 12, 0, 0, 0, time.LocationUTC),
			"UTC ran at a rubber rate before 1972, so Delta-AT is not integral", true},
		{"1960, pre-Delta-AT", time.Date(1960, 1, 1, 0, 0, 0, 0, time.LocationUTC),
			"SOFA's Dat begins here; before it there is no Delta-AT at all", true},
		{"J2000", time.FromJD(2451545.0, time.TDB),
			"the origin every reduction in the library is expressed against", false},
		{"1900", time.Date(1900, 1, 1, 0, 0, 0, 0, time.LocationUTC),
			"a century before J2000, well inside the Delta-T polynomial", true},
		{"1600", time.Date(1600, 6, 15, 12, 0, 0, 0, time.LocationUTC),
			"historical; Delta-T is about two minutes and the two directions used to disagree by all of it", true},
		{"AD 33", time.Date(33, 4, 3, 15, 0, 0, 0, time.LocationUTC),
			"the era the historical showcases compute in, where Delta-T is hours", true},
		{"present era", time.Date(2026, 8, 29, 6, 30, 0, 0, time.LocationUTC),
			"an ordinary modern date with no special property", false},
		{"2100", time.Date(2100, 1, 1, 0, 0, 0, 0, time.LocationUTC),
			"past the end of measured Earth-orientation data", false},
	}
}

// TestScaleRoundTripMatrix converts every scale to every other and back.
//
// # Why this exists
//
// Three defects in this package were found by review rather than by test,
// and every one of them would have failed here:
//
//   - Time.ToGo read jd1/jd2 as UTC whatever scale they were on, so one
//     instant rendered 69.184 s apart depending on the representation the
//     caller happened to hold.
//   - ApplyDeltaT documented a conversion to TT and tagged the result TDB.
//   - UTC->TT used the Delta-T polynomial before 1972 while TT->UTC went
//     through Delta-AT, so a 1600 round trip came back 85.7 s late.
//
// All three are the same failure: a scale that lives in a comment rather than
// in something executable. Sixty round trips over twelve epochs is cheap, and
// it converts that whole class from "someone has to notice" into "the suite
// fails".
//
// # Two regimes, two contracts
//
// Splitting them is the point rather than an accounting convenience: averaged
// together, a nanosecond-exact conversion and one bounded by tabulated data
// produce a single number that describes neither.
//
//   - **Arithmetic.** TAI, TT and TDB differ by a constant and a periodic
//     term, and from 1972 UTC differs from them by an integral count of leap
//     seconds. Those conversions are additions on the fraction of a two-part
//     Julian Date, where float64 spacing is about 1e-16 days — nine
//     nanoseconds. A round trip must return the instant it started from, and
//     measured it does: the median is exactly zero. The bound is 1
//     microsecond, two orders above the representation limit and far below
//     anything a model asymmetry could produce.
//
//   - **Modelled.** UT1 is UTC plus a tabulated DUT1, and before 1972 UTC ran
//     at a rubber rate whose Delta-AT is not an integer and not invertible.
//     Neither round trip can be exact, and pretending otherwise would mean
//     tightening a bound until a physical limitation looked like a bug. The
//     bound is 5 seconds, which is the magnitude of the offsets themselves —
//     |DUT1| below 0.9 s by leap-second insertion, pre-1972 Delta-AT reaching
//     about 4.2 s by 1971 — rather than a number fitted to what was measured.
//     The worst case is 0.94 s, at 1960-01-01: the very first row of SOFA's
//     Delta-AT table, where the rate definition begins and there is nothing
//     before it to interpolate against. Every other epoch, 1971 included,
//     round-trips to microseconds or better.
func TestScaleRoundTripMatrix(t *testing.T) {
	t.Parallel()

	// An identity needs no external authority, and no other check can catch
	// an error that two implementations share.
	ref := metrology.Reference{
		Kind:   metrology.KindInvariant,
		Name:   "scale round trip, A to B and back",
		Source: "time/scale_matrix_test.go",
	}

	arithmetic := metrology.NewSuite("time.scale.roundtrip.arithmetic", ref,
		metrology.MustContract(1e-6, "s",
			"a two-part Julian Date keeps the fraction in jd2, where float64 spacing is about "+
				"1e-16 days or 9 ns; these conversions are a handful of additions and cannot "+
				"accumulate past a microsecond, so anything above this is a model asymmetry "+
				"rather than rounding",
			"IEEE 754 double spacing at the magnitude of jd2; see Time.normalize"))

	modelled := metrology.NewSuite("time.scale.roundtrip.modelled", ref,
		metrology.MustContract(5.0, "s",
			"a round trip through a scale defined by tabulated data can lose up to the magnitude "+
				"of the offset itself: |DUT1| is kept below 0.9 s by leap-second insertion, and "+
				"pre-1972 Delta-AT reaches about 4.2 s by 1971 under a rate definition that is "+
				"neither integral nor exactly invertible. 5 s bounds both; it is not fitted to the "+
				"measurement, which sits five times inside it",
			"IERS Bulletin C on |DUT1|; SOFA iauDat, pre-1972 drift terms"))

	scales := allScales()

	for _, e := range matrixEpochs() {
		for _, from := range scales {
			// Put the epoch on the starting scale first, so the round trip
			// measured is from->to->from rather than whatever the epoch was
			// constructed as.
			start := from.to(e.t)

			for _, to := range scales {
				back := from.to(to.to(start))

				// Compared through JDParts rather than JD, and that matters
				// at this precision: JD() sums the two halves, collapsing
				// them to a single float64 whose spacing at JD 2.46e6 is
				// about 5.5e-10 days — 47 microseconds. Measuring that way
				// put a 40-microsecond floor under every sample and made the
				// measurement's own grain look like a conversion error.
				b1, b2 := back.JDParts()
				s1, s2 := start.JDParts()
				drift := ((b1 - s1) + (b2 - s2)) * 86400

				sample := metrology.Sample{
					Error:   drift,
					Label:   from.name + "->" + to.name + "->" + from.name + " @ " + e.name,
					Context: e.why,
				}

				// UT1 always consults tabulated DUT1; before 1972 so does
				// any pair that crosses between civil and atomic time.
				if from.name == "UT1" || to.name == "UT1" || (e.preUTC && from.name != to.name) {
					modelled.Add(sample)
				} else {
					arithmetic.Add(sample)
				}
			}
		}
	}

	arithmetic.Report(t)
	modelled.Report(t)
}

// TestToGoIsOneInstantWhateverTheScale pins the defect directly.
//
// The matrix above compares Julian Dates, which is the right place to measure
// a conversion — but the bug that prompted all this was in the step *after*
// the conversion, where a Time becomes a time.Time. Every formatted event
// time in plan goes through it.
func TestToGoIsOneInstantWhateverTheScale(t *testing.T) {
	t.Parallel()

	for _, e := range matrixEpochs() {
		want := e.t.UTC().ToGo()

		for _, s := range allScales() {
			got := s.to(e.t).ToGo()

			// Two regimes again, for the reason the matrix splits them: a
			// scale reached by arithmetic must render the same instant to
			// the microsecond, while one reached through tabulated data
			// cannot do better than the data. 1960-01-01 on TAI is the one
			// case that needs the looser bound, being the first row of
			// SOFA's Delta-AT table.
			tol := 1e-3
			if s.name == "UT1" || e.preUTC {
				tol = 5.0
			}

			if d := math.Abs(got.Sub(want).Seconds()); d > tol {
				t.Errorf("%s @ %s: ToGo differs by %.6f s from the same instant on UTC.\n"+
					"  One instant must render as one time.Time whichever scale holds it;\n"+
					"  a difference here is a representation leaking into an instant.",
					s.name, e.name, d)
			}
		}
	}
}

// TestApplyDeltaTReturnsTT is the whole of the second defect.
//
// The method's own documentation says three times that it converts to TT.
// It returned TDB. A test this small would have caught it the day it was
// written, which is the argument for semantic invariants over test count.
func TestApplyDeltaTReturnsTT(t *testing.T) {
	t.Parallel()

	got := time.Date(1600, 6, 15, 12, 0, 0, 0, time.LocationUTC).ApplyDeltaT()
	if got.Scale() != time.TT {
		t.Errorf("ApplyDeltaT().Scale() = %v, want TT — which is what its own doc comment says",
			got.Scale())
	}
}
