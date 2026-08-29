package metrology_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
)

// baselineResult builds a Result directly, since a baseline comparison is
// about two sets of statistics and not about how they were produced.
func baselineResult(suite string, status metrology.Status, p50, p95, p99, maximum float64) metrology.Result {
	return metrology.Result{
		SchemaVersion: metrology.SchemaVersion,
		Suite:         suite,
		Status:        status,
		Contract:      testContract(1.0),
		Stats: metrology.Stats{
			N: 100, P50: p50, P95: p95, P99: p99, Max: maximum,
		},
	}
}

// The case the whole file exists for: accuracy getting materially worse while
// the contract still holds.
//
// A p99 going from 0.15 to 0.62 under a 1.0 bound is four times worse and
// passes every assertion a contract can make, because a contract does not
// remember what the number used to be.
func TestBaselineFlagsARegressionInsideTheContract(t *testing.T) {
	t.Parallel()

	base := metrology.NewBaseline(baselineResult("s", metrology.StatusVerified, 0.05, 0.10, 0.15, 0.20))
	now := baselineResult("s", metrology.StatusVerified, 0.05, 0.10, 0.62, 0.70)

	regressions := base.Compare(now, metrology.DefaultRegressionFactor)
	if len(regressions) == 0 {
		t.Fatal("a 4x p99 regression inside the contract went unreported")
	}

	// Worst first, so the most alarming number is the one a reader sees.
	if regressions[0].Statistic != "p99" {
		t.Errorf("worst regression is %q, want p99 (4.13x beats max at 3.5x)", regressions[0].Statistic)
	}

	if regressions[0].Was != 0.15 || regressions[0].Now != 0.62 {
		t.Errorf("regression reports %v -> %v, want 0.15 -> 0.62", regressions[0].Was, regressions[0].Now)
	}

	// p50 and p95 did not move and must not be reported; a check that cries
	// about everything gets ignored about everything.
	for _, r := range regressions {
		if r.Statistic == "p50" || r.Statistic == "p95" {
			t.Errorf("%s was unchanged but reported as a regression", r.Statistic)
		}
	}

	// The failure has to say the contract still holds, or it reads as a
	// correctness failure and the next person reaches for the contract.
	rec := &recorder{}
	base.Check(rec, now, metrology.DefaultRegressionFactor)

	if len(rec.errors) == 0 {
		t.Fatal("Check did not fail on a regression")
	}

	if !strings.Contains(rec.output(), "still holds") {
		t.Errorf("the failure does not distinguish a regression from a violation:\n%s", rec.output())
	}

	if !strings.Contains(rec.output(), "Update the baseline only once") {
		t.Errorf("the failure does not say what to do about it:\n%s", rec.output())
	}
}

func TestBaselineIgnoresImprovementAndSmallDrift(t *testing.T) {
	t.Parallel()

	base := metrology.NewBaseline(baselineResult("s", metrology.StatusVerified, 0.05, 0.10, 0.15, 0.20))

	for _, tc := range []struct {
		name                   string
		p50, p95, p99, maximum float64
	}{
		{"identical", 0.05, 0.10, 0.15, 0.20},
		{"improved", 0.01, 0.02, 0.03, 0.04},
		// Drifting by half again is ordinary for a suite measured against
		// live services and reference data that moves.
		{"drifted 1.5x", 0.075, 0.15, 0.225, 0.30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			now := baselineResult("s", metrology.StatusVerified, tc.p50, tc.p95, tc.p99, tc.maximum)
			if got := base.Compare(now, metrology.DefaultRegressionFactor); len(got) != 0 {
				t.Errorf("reported %d regressions, want none: %+v", len(got), got)
			}
		})
	}
}

// A suite with no baseline has no evidence of getting worse.
//
// Comparing against a missing entry as though it were zero would make every
// new suite report an infinite regression on its first run, which trains
// people to ignore the check.
func TestBaselineSaysNothingAboutAnUnknownSuite(t *testing.T) {
	t.Parallel()

	base := metrology.NewBaseline(baselineResult("known", metrology.StatusVerified, 1, 1, 1, 1))
	now := baselineResult("brand-new", metrology.StatusVerified, 99, 99, 99, 99)

	if got := base.Compare(now, metrology.DefaultRegressionFactor); got != nil {
		t.Errorf("compared an unknown suite against nothing: %+v", got)
	}
}

// A suite that did not run neither contributes to a baseline nor is compared
// against one.
//
// Recording its zeroes would turn "we did not measure" into "we measured zero
// error" — the strongest possible claim, made by accident — and every later
// run would then regress against it infinitely.
func TestBaselineExcludesSuitesThatDidNotRun(t *testing.T) {
	t.Parallel()

	base := metrology.NewBaseline(
		baselineResult("ran", metrology.StatusVerified, 1, 1, 1, 1),
		baselineResult("did-not", metrology.StatusNotVerified, 0, 0, 0, 0),
	)

	if _, ok := base.Suites["did-not"]; ok {
		t.Error("a NOT_VERIFIED suite was recorded in the baseline as though it had measured zero")
	}

	if _, ok := base.Suites["ran"]; !ok {
		t.Error("a verified suite was left out of the baseline")
	}

	// And in the other direction: a run that could not happen is not a
	// regression against a baseline that could.
	skipped := baselineResult("ran", metrology.StatusNotVerified, 0, 0, 0, 0)
	if got := base.Compare(skipped, metrology.DefaultRegressionFactor); got != nil {
		t.Errorf("a skipped run was compared against its baseline: %+v", got)
	}
}

// A percentile that was zero has no factor to grow by.
func TestBaselineSkipsZeroValuedStatistics(t *testing.T) {
	t.Parallel()

	base := metrology.NewBaseline(baselineResult("s", metrology.StatusVerified, 0, 0, 0, 0))
	now := baselineResult("s", metrology.StatusVerified, 1, 1, 1, 1)

	if got := base.Compare(now, metrology.DefaultRegressionFactor); len(got) != 0 {
		t.Errorf("divided by a zero baseline: %+v", got)
	}
}

func TestBaselineRoundTripsAndRefusesAnotherSchema(t *testing.T) {
	t.Parallel()

	base := metrology.NewBaseline(baselineResult("s", metrology.StatusVerified, 0.05, 0.10, 0.15, 0.20))

	data, err := base.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}

	got, err := metrology.LoadBaseline(data)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}

	if got.Suites["s"].P99 != 0.15 {
		t.Errorf("p99 survived as %v, want 0.15", got.Suites["s"].P99)
	}

	_, err = metrology.LoadBaseline([]byte("{\"schema_version\": 999}"))
	if !errors.Is(err, metrology.ErrSchemaVersion) {
		t.Errorf("error = %v, want %v", err, metrology.ErrSchemaVersion)
	}

	_, err = metrology.LoadBaseline([]byte("not json"))
	if !errors.Is(err, metrology.ErrDecodeBaseline) {
		t.Errorf("error = %v, want %v", err, metrology.ErrDecodeBaseline)
	}
}
