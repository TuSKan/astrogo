package gofaext

import (
	"sync/atomic"

	"github.com/hebl/gofa"
)

// seriesEvaluations counts evaluations of the IAU 2006/2000A
// precession-nutation series made through this package. See
// [SeriesEvaluations].
var seriesEvaluations atomic.Int64

// SeriesEvaluations reports how many times the IAU 2006/2000A
// precession-nutation series has been evaluated through this package since
// the process started.
//
// The series, Nut00a's 1,365 terms, is the dearest thing astrogo asks SOFA
// for: about 75 µs, against nanoseconds for nearly everything else here. It
// is also the easiest to evaluate twice without noticing, because several
// SOFA routines run it inside themselves. coord.NewContext did exactly that
// from May 2026 on: Apco13 built the celestial-to-intermediate matrix and
// returned it, and C2t06a, later C2i06a, built it again from the same series
// for the same instant. Every output was correct, so no value test could see
// it, and it cost a third of every NewContext (#473).
//
// This counter is how a test can see it. Counts are deterministic across
// machines, as allocation counts are and timings are not, so a test can hold
// a path to a number of evaluations and fail the build when it changes: a
// work contract. coord's TestNewContextEvaluatesTheSeriesOnce is the first.
//
// It costs one atomic add per evaluation of a series that takes about 75 µs.
func SeriesEvaluations() int64 {
	return seriesEvaluations.Load()
}

// series records one evaluation of the precession-nutation series. Every
// wrapper whose SOFA routine reaches Nut00a calls it exactly once: directly
// (Nut06a), through Pnm06a (Pnm06a, C2i06a, C2t06a, Gst06a, Ee06a), through
// Apco13 (Apco13, Atco13, Atoc13) or through Apci13 (Atci13, Atic13).
// TestEverySeriesWrapperCountsOnce holds that list to the wrappers.
func series() {
	seriesEvaluations.Add(1)
}

// UTCToTT returns the TT two-part Julian date that Apco13 derives internally
// from a UTC two-part Julian date: UTC to TAI by the leap-second table, then
// TAI to TT.
//
// It differs from astrogo's own time.Time.TT before 1972, where that follows
// ΔT and this follows SOFA's table, by up to 34 s at 1900. From 1972 on the
// two agree, which is what lets coord.NewContext reuse the matrix Apco13 has
// already built instead of evaluating the series again.
func UTCToTT(utc1, utc2 float64) (tt1, tt2 float64) {
	var tai1, tai2 float64

	gofa.Utctai(utc1, utc2, &tai1, &tai2)
	gofa.Taitt(tai1, tai2, &tt1, &tt2)

	return tt1, tt2
}
