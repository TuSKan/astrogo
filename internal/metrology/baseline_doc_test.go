//go:build validation

package metrology_test

import (
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
)

// checkBaseline and updateBaseline gate the two things TestResultsHoldTheBaseline
// does, for the reason updateAccuracy gates the table: it reads documents the
// suites write while they run, so it must run after them, never alongside.
var (
	checkBaseline = flag.Bool("check-baseline", false,
		"compare the collected results with docs/validation-baseline.json")
	updateBaseline = flag.Bool("update-baseline", false,
		"record the collected results in docs/validation-baseline.json")
)

// baselineDoc is the accuracy each suite reached at the last accepted run,
// beside the table that renders the same results.
var baselineDoc = filepath.Join("..", "..", "docs", "validation-baseline.json")

// TestResultsHoldTheBaseline compares every collected result with the
// accuracy its suite reached at the last accepted run, and fails for a
// percentile that has more than doubled ([metrology.DefaultRegressionFactor])
// even while the contract still holds (#1109).
//
// # Why it exists
//
// A contract is a floor, and a suite can fall a long way without reaching it:
// a p99 going from 0.15 to 0.62 arcseconds under a 1 arcsecond contract passes
// every assertion a contract can make. [metrology.Baseline] remembers what the
// number used to be. Until #1109 nothing recorded a baseline or compared with
// one, so it remembered nothing.
//
// # How it runs
//
// The Validation workflow runs it after each job's suites, with -check-baseline,
// against the results that job wrote to ASTROGO_METROLOGY_OUT. A suite with no
// entry, or a result that is NOT VERIFIED, is logged and not compared.
//
// # How the baseline moves
//
// Only as a reviewed diff: put the result documents of an accepted run of both
// jobs in one directory (the workflow uploads each job's as an artifact), then
//
//	ASTROGO_METROLOGY_OUT=<that directory> \
//	  go test -tags=validation -run TestResultsHoldTheBaseline ./internal/metrology/ -args -update-baseline
//
// A suite those results did not verify keeps the entry it had. Investigate a
// regression before recording it, as the failure itself says.
func TestResultsHoldTheBaseline(t *testing.T) {
	if !*checkBaseline && !*updateBaseline {
		t.Skipf("pass -args -check-baseline or -update-baseline to use %s; see this test's doc comment",
			baselineDoc)
	}

	results := collectResults(t)

	if *updateBaseline {
		writeBaseline(t, results)

		return
	}

	data, err := os.ReadFile(baselineDoc)
	if err != nil {
		t.Fatalf("reading %s: %v", baselineDoc, err)
	}

	base, err := metrology.LoadBaseline(data)
	if err != nil {
		t.Fatalf("%s: %v", baselineDoc, err)
	}

	verified, compared := 0, 0

	for _, r := range results {
		if r.Status == metrology.StatusNotVerified {
			t.Logf("  %-40s not compared: NOT VERIFIED in this run", r.Suite)

			continue
		}

		verified++

		if _, ok := base.Suites[r.Suite]; !ok {
			t.Logf("  %-40s not compared: no entry in %s yet", r.Suite, baselineDoc)

			continue
		}

		base.Check(t, r, metrology.DefaultRegressionFactor)

		compared++
	}

	switch {
	case verified == 0:
		// Every endpoint down at once says nothing about accuracy either way,
		// and the report already shows each suite as NOT VERIFIED.
		t.Skipf("all %d results are NOT VERIFIED: nothing to compare with %s", len(results), baselineDoc)
	case compared == 0:
		// Verified results and not one with an entry is a baseline that no
		// longer names the suites, renamed or never recorded, and a check that
		// compares nothing must not pass.
		t.Fatalf("none of the %d verified results has an entry in %s: record the baseline again "+
			"(see this test's doc comment)", verified, baselineDoc)
	}

	t.Logf("%d of %d results compared with %s", compared, len(results), baselineDoc)
}

// writeBaseline records results in the checked-in baseline, keeping the entry
// of any suite they did not verify.
func writeBaseline(t *testing.T, results []metrology.Result) {
	t.Helper()

	var base metrology.Baseline

	data, err := os.ReadFile(baselineDoc)

	switch {
	case err == nil:
		if base, err = metrology.LoadBaseline(data); err != nil {
			t.Fatalf("%s: %v", baselineDoc, err)
		}
	case errors.Is(err, fs.ErrNotExist):
		// The first baseline starts from nothing.
	default:
		t.Fatalf("reading %s: %v", baselineDoc, err)
	}

	out, err := base.Update(results...).JSON()
	if err != nil {
		t.Fatalf("%v", err)
	}

	if err := os.WriteFile(baselineDoc, out, 0o600); err != nil {
		t.Fatalf("writing %s: %v", baselineDoc, err)
	}

	t.Logf("recorded %d results in %s", len(results), baselineDoc)
}
