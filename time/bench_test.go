package time

import (
	"testing"
)

// ── Scale Conversion Benchmarks ──────────────────────────────────────────────
// These measure the cost of the time-scale conversion graph.

func BenchmarkUTCToTAI(b *testing.B) {
	t := FromJD(2460000.5, UTC)

	for b.Loop() {
		_ = t.TAI()
	}
}

func BenchmarkUTCToTT(b *testing.B) {
	t := FromJD(2460000.5, UTC)

	for b.Loop() {
		_ = t.TT()
	}
}

func BenchmarkUTCToTDB(b *testing.B) {
	t := FromJD(2460000.5, UTC)

	for b.Loop() {
		_ = t.TDB()
	}
}

func BenchmarkUTCToUT1(b *testing.B) {
	t := FromJD(2460000.5, UTC)

	for b.Loop() {
		_, _ = t.UT1()
	}
}

func BenchmarkTTToUTC(b *testing.B) {
	t := FromJD(2460000.5, UTC).TT()

	for b.Loop() {
		_ = t.UTC()
	}
}

func BenchmarkTDBToTT(b *testing.B) {
	t := FromJD(2460000.5, UTC).TDB()

	for b.Loop() {
		_ = t.TT()
	}
}

// ── Round-Trip Conversion ────────────────────────────────────────────────────
// Measures the full chain cost: UTC → TAI → TT → TDB → TT → TAI → UTC

func BenchmarkFullRoundTrip(b *testing.B) {
	t := FromJD(2460000.5, UTC)

	for b.Loop() {
		_ = t.TAI().TT().TDB().TT().TAI().UTC()
	}
}

// ── Cross-Scale Comparison ───────────────────────────────────────────────────
// Measures the overhead of auto-converting comparisons.

func BenchmarkEqual_SameScale(b *testing.B) {
	t1 := FromJD(2460000.5, UTC)
	t2 := FromJD(2460000.5, UTC)

	for b.Loop() {
		_ = t1.Equal(t2)
	}
}

func BenchmarkEqual_CrossScale(b *testing.B) {
	t1 := FromJD(2460000.5, UTC)
	t2 := t1.TT()

	for b.Loop() {
		_ = t1.Equal(t2)
	}
}

func BenchmarkSub_SameScale(b *testing.B) {
	t1 := FromJD(2460001.0, UTC)
	t2 := FromJD(2460000.5, UTC)

	for b.Loop() {
		_ = t1.Sub(t2)
	}
}

func BenchmarkSub_CrossScale(b *testing.B) {
	t1 := FromJD(2460001.0, UTC)
	t2 := FromJD(2460000.5, UTC).TDB()

	for b.Loop() {
		_ = t1.Sub(t2)
	}
}

// BenchmarkSub_SameScale_Uniform is the counterpart to BenchmarkSub_SameScale,
// which uses UTC.
//
// Sub converts to TT whenever it is handed a non-uniform scale, so two UTC
// epochs now cost two conversions (measured: 2.0 -> 91.5 ns/op) in exchange for
// an answer that counts the leap seconds between them. A uniform scale needs no
// conversion and must stay on the cheap path — this is what pins that.
func BenchmarkSub_SameScale_Uniform(b *testing.B) {
	t1 := FromJD(2460001.0, TT)
	t2 := FromJD(2460000.5, TT)

	for b.Loop() {
		_ = t1.Sub(t2)
	}
}
