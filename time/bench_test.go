package time

import (
	"testing"
)

// ── Scale Conversion Benchmarks ──────────────────────────────────────────────
// These measure the cost of the time-scale conversion graph.

func BenchmarkUTCToTAI(b *testing.B) {
	t := FromJD(2460000.5, UTC)

	b.ResetTimer()

	for range b.N {
		_ = t.TAI()
	}
}

func BenchmarkUTCToTT(b *testing.B) {
	t := FromJD(2460000.5, UTC)

	b.ResetTimer()

	for range b.N {
		_ = t.TT()
	}
}

func BenchmarkUTCToTDB(b *testing.B) {
	t := FromJD(2460000.5, UTC)

	b.ResetTimer()

	for range b.N {
		_ = t.TDB()
	}
}

func BenchmarkUTCToUT1(b *testing.B) {
	t := FromJD(2460000.5, UTC)

	b.ResetTimer()

	for range b.N {
		_, _ = t.UT1()
	}
}

func BenchmarkTTToUTC(b *testing.B) {
	t := FromJD(2460000.5, UTC).TT()

	b.ResetTimer()

	for range b.N {
		_ = t.UTC()
	}
}

func BenchmarkTDBToTT(b *testing.B) {
	t := FromJD(2460000.5, UTC).TDB()

	b.ResetTimer()

	for range b.N {
		_ = t.TT()
	}
}

// ── Round-Trip Conversion ────────────────────────────────────────────────────
// Measures the full chain cost: UTC → TAI → TT → TDB → TT → TAI → UTC

func BenchmarkFullRoundTrip(b *testing.B) {
	t := FromJD(2460000.5, UTC)

	b.ResetTimer()

	for range b.N {
		_ = t.TAI().TT().TDB().TT().TAI().UTC()
	}
}

// ── Cross-Scale Comparison ───────────────────────────────────────────────────
// Measures the overhead of auto-converting comparisons.

func BenchmarkEqual_SameScale(b *testing.B) {
	t1 := FromJD(2460000.5, UTC)
	t2 := FromJD(2460000.5, UTC)

	b.ResetTimer()

	for range b.N {
		_ = t1.Equal(t2)
	}
}

func BenchmarkEqual_CrossScale(b *testing.B) {
	t1 := FromJD(2460000.5, UTC)
	t2 := t1.TT()

	b.ResetTimer()

	for range b.N {
		_ = t1.Equal(t2)
	}
}

func BenchmarkSub_SameScale(b *testing.B) {
	t1 := FromJD(2460001.0, UTC)
	t2 := FromJD(2460000.5, UTC)

	b.ResetTimer()

	for range b.N {
		_ = t1.Sub(t2)
	}
}

func BenchmarkSub_CrossScale(b *testing.B) {
	t1 := FromJD(2460001.0, UTC)
	t2 := FromJD(2460000.5, UTC).TDB()

	b.ResetTimer()

	for range b.N {
		_ = t1.Sub(t2)
	}
}
