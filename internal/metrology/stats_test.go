package metrology_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
)

// testContract is a valid contract for tests that are not about contracts.
func testContract(maximum float64) metrology.Contract {
	return metrology.MustContract(maximum, "arcsec",
		"an arbitrary bound for exercising the statistics",
		"internal/metrology stats_test.go")
}

func testSuite(maximum float64, errs ...float64) *metrology.Suite {
	s := metrology.NewSuite("test.suite", metrology.Reference{
		Kind: metrology.KindInvariant, Name: "none",
	}, testContract(maximum))

	for i, e := range errs {
		s.Add(metrology.Sample{
			Error:   e,
			Label:   string(rune('a' + i%26)),
			Context: "sample",
		})
	}

	return s
}

// Percentiles are interpolated between order statistics — numpy's default,
// R's type 7 — and the definition is pinned here because the alternatives
// disagree on small samples and a percentile whose definition is unstated
// cannot be compared against anyone else's.
func TestStatsPercentiles(t *testing.T) {
	t.Parallel()

	t.Run("one sample", func(t *testing.T) {
		t.Parallel()

		got := testSuite(100, 7).Stats()
		for name, v := range map[string]float64{
			"P50": got.P50, "P90": got.P90, "P95": got.P95, "P99": got.P99, "Max": got.Max,
		} {
			if v != 7 {
				t.Errorf("%s = %v, want 7 — every percentile of one sample is that sample", name, v)
			}
		}
	})

	t.Run("two samples interpolate", func(t *testing.T) {
		t.Parallel()

		// With n=2 the p-th quantile sits at index p*(n-1) = p, so the
		// median is halfway between the two and p90 is nine tenths along.
		got := testSuite(100, 0, 10).Stats()

		if math.Abs(got.P50-5) > 1e-12 {
			t.Errorf("P50 = %v, want 5", got.P50)
		}

		if math.Abs(got.P90-9) > 1e-12 {
			t.Errorf("P90 = %v, want 9", got.P90)
		}
	})

	t.Run("one thousand samples", func(t *testing.T) {
		t.Parallel()

		// 1..1000, so the p-th quantile is 1 + p*999.
		errs := make([]float64, 1000)
		for i := range errs {
			errs[i] = float64(i + 1)
		}

		got := testSuite(1e9, errs...).Stats()

		for _, tc := range []struct {
			name string
			got  float64
			want float64
		}{
			{"P50", got.P50, 1 + 0.50*999},
			{"P95", got.P95, 1 + 0.95*999},
			{"P99", got.P99, 1 + 0.99*999},
			{"Max", got.Max, 1000},
		} {
			if math.Abs(tc.got-tc.want) > 1e-9 {
				t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
			}
		}
	})
}

// Percentiles are taken on magnitudes; bias is kept separately.
//
// A set of residuals scattered about zero and a set all pointing one way can
// have identical magnitudes and mean entirely different things, and only the
// second is a systematic error worth chasing.
func TestStatsSeparatesMagnitudeFromBias(t *testing.T) {
	t.Parallel()

	scattered := testSuite(100, -3, 3, -3, 3).Stats()
	biased := testSuite(100, 3, 3, 3, 3).Stats()

	if scattered.Mean != biased.Mean || scattered.Max != biased.Max {
		t.Fatalf("magnitudes should be identical: scattered %v/%v vs biased %v/%v",
			scattered.Mean, scattered.Max, biased.Mean, biased.Max)
	}

	if scattered.MeanSigned != 0 {
		t.Errorf("scattered signed mean = %v, want 0", scattered.MeanSigned)
	}

	if biased.MeanSigned != 3 {
		t.Errorf("biased signed mean = %v, want 3", biased.MeanSigned)
	}

	if scattered.MinSigned != -3 || scattered.MaxSigned != 3 {
		t.Errorf("scattered signed range = [%v, %v], want [-3, 3]",
			scattered.MinSigned, scattered.MaxSigned)
	}
}

func TestStatsRMSAndMean(t *testing.T) {
	t.Parallel()

	// |errors| 1, 2, 3: mean 2, RMS sqrt(14/3).
	got := testSuite(100, 1, -2, 3).Stats()

	if math.Abs(got.Mean-2) > 1e-12 {
		t.Errorf("Mean = %v, want 2", got.Mean)
	}

	if want := math.Sqrt(14.0 / 3.0); math.Abs(got.RMS-want) > 1e-12 {
		t.Errorf("RMS = %v, want %v", got.RMS, want)
	}

	// RMS exceeding the mean is what a tail looks like; equal would mean
	// every sample is identical.
	if got.RMS <= got.Mean {
		t.Errorf("RMS %v should exceed mean %v for a spread distribution", got.RMS, got.Mean)
	}
}

// The worst sample must carry its own identity, or a report can say how bad
// things got but not where.
func TestStatsWorstNamesItsScenario(t *testing.T) {
	t.Parallel()

	s := metrology.NewSuite("test.suite", metrology.Reference{}, testContract(100))
	s.Add(metrology.Sample{Error: 1, Label: "Mars @ Paranal", Context: "2026-01-05"})
	s.Add(metrology.Sample{Error: -9, Label: "Moon @ Polar", Context: "2026-06-21 el=2.1"})
	s.Add(metrology.Sample{Error: 4, Label: "Jupiter @ Greenwich", Context: "2026-03-01"})

	got := s.Stats()

	if got.Max != 9 {
		t.Errorf("Max = %v, want 9 (magnitude of the -9 sample)", got.Max)
	}

	if got.Worst.Label != "Moon @ Polar" {
		t.Errorf("Worst.Label = %q, want the sample with the largest magnitude", got.Worst.Label)
	}

	if got.Worst.Context != "2026-06-21 el=2.1" {
		t.Errorf("Worst.Context = %q, want the context that reproduces it", got.Worst.Context)
	}
}

// Non-finite samples are dropped, counted, and reported as an error.
//
// Propagating them would turn every aggregate into NaN and destroy the
// evidence about the samples that were fine; ignoring them would let a suite
// discard most of its input and still publish excellent statistics over
// whatever survived.
func TestStatsRejectsNonFiniteSamples(t *testing.T) {
	t.Parallel()

	s := testSuite(100, 1, math.NaN(), 3, math.Inf(1), math.Inf(-1))
	got := s.Stats()

	if got.Rejected != 3 {
		t.Errorf("Rejected = %d, want 3", got.Rejected)
	}

	if got.N != 2 {
		t.Errorf("N = %d, want 2 — only the finite samples count", got.N)
	}

	if math.IsNaN(got.Mean) || math.Abs(got.Mean-2) > 1e-12 {
		t.Errorf("Mean = %v, want 2 — a NaN must not poison the aggregate", got.Mean)
	}

	rec := &recorder{}
	s.Report(rec)

	if len(rec.errors) == 0 {
		t.Fatal("dropping 3 of 5 samples was not reported as an error")
	}

	if !strings.Contains(rec.output(), "NaN or infinite") {
		t.Errorf("the failure does not say what was dropped: %s", rec.output())
	}
}

// An empty suite reports nothing rather than dividing by nothing.
func TestStatsWithNoSamples(t *testing.T) {
	t.Parallel()

	got := testSuite(100).Stats()

	for name, v := range map[string]float64{
		"Mean": got.Mean, "RMS": got.RMS, "P50": got.P50, "Max": got.Max,
		"MeanSigned": got.MeanSigned, "MinSigned": got.MinSigned, "MaxSigned": got.MaxSigned,
	} {
		if v != 0 {
			t.Errorf("%s = %v on an empty suite, want 0", name, v)
		}
	}

	if got.N != 0 {
		t.Errorf("N = %d, want 0", got.N)
	}
}

func TestStatsCountsSamplesOutsideTheContract(t *testing.T) {
	t.Parallel()

	got := testSuite(2.5, 1, 2, 3, 4).Stats()

	if got.Exceeding != 2 {
		t.Errorf("Exceeding = %d, want 2 (3 and 4 are over a 2.5 bound)", got.Exceeding)
	}
}
