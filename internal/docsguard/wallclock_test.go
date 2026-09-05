package docsguard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wallClockAllowed lists the test files permitted to read the wall clock, and
// why each one is.
//
// A reason rather than a bare list, for the same purpose it serves in the
// endpoint registry: the entry has to be written by whoever adds the call, and
// it can be checked later. A list of paths rots silently.
var wallClockAllowed = map[string]string{
	"atmosphere/builder_roundtrip_test.go": "asserts Age(now) is zero for a value just built; " +
		"the present is the input under test",
	"ephemeris/ephemeris_bench_test.go": "a benchmark, which measures a rate rather than " +
		"asserting an astronomical result",
	"ephemeris/jpl/validation/corpus_settled_test.go": "a corpus entry is 'settled' relative to " +
		"the present by definition",
	"ephemeris/jpl/validation/gen_corpus_test.go": "stamps the generation time into the corpus " +
		"manifest, which is the provenance record",
	"internal/changelog/assemble_test.go":                   "release dating",
	"remote/api/pace_test.go":                               "rate limiting; elapsed time is the subject",
	"remote/lock_test.go":                                   "lock acquisition timing is the subject",
	"skybrightness/dataset/airglow/almanac_network_test.go": "queries a live service for the current sky",
	"skybrightness/dataset/starlight/gaia_network_test.go":  "measures elapsed time to show the cache works",
	"skybrightness/gambons_allsky_test.go":                  "measures elapsed time for its own report",
	"time/internal/iers/fetch_test.go":                      "the fetch cooldown is defined relative to now",
	"time/time_test.go":                                     "TestNowUTC tests NowUTC",
}

// TestNoUndeclaredWallClockTests keeps tests from quietly depending on when
// they run.
//
// # The failure this prevents
//
// A test pinned to time.Now drifts on its own schedule and fails on some
// future Tuesday with no code change — the worst kind of failure, because the
// first suspicion falls on a recent commit rather than on the calendar.
//
// The near-term mechanism is Earth Orientation Parameters, not kernel spans.
// IERS finals2000A carries measured values to about a month behind the present
// and predictions for roughly a year ahead, and that boundary moves every
// week. A test at "now" crosses from measured to predicted and eventually off
// the end of the file, silently changing the numbers it computes on the way.
//
// Nineteen test files read the wall clock when this was filed. Six of them
// asserted astronomical results at whatever instant the suite happened to run,
// and those now use a fixed past epoch — final, unrevisable, and computing the
// same values in 2027 as today. The rest are here, each for a stated reason.
//
// # Comments are exempt
//
// A doc comment saying a test "previously used time.NowUTC()" is a record of
// what changed, not a live dependency. sofa_compare_test.go is exactly that
// case: it carries three such mentions and no call, which is why it does not
// appear above.
func TestNoUndeclaredWallClockTests(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	found := map[string]bool{}

	var checked int

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		rel = filepath.ToSlash(rel)

		// This file names the calls it is guarding against.
		if strings.HasSuffix(rel, "internal/docsguard/wallclock_test.go") {
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // an unreadable file is skipped, not fatal
		}

		checked++

		for line := range strings.SplitSeq(string(data), "\n") {
			code := strings.TrimSpace(line)
			if strings.HasPrefix(code, "//") {
				continue
			}

			if idx := strings.Index(code, "//"); idx >= 0 {
				code = code[:idx]
			}

			if !strings.Contains(code, "time.Now()") && !strings.Contains(code, "time.NowUTC()") {
				continue
			}

			found[rel] = true

			if _, ok := wallClockAllowed[rel]; !ok {
				t.Errorf("%s reads the wall clock and is not declared in "+
					"wallClockAllowed.\n"+
					"  A test at \"now\" drifts across the IERS measured/predicted "+
					"boundary and fails on a future date with no code change. Use a "+
					"fixed past epoch, or add an entry saying why the present is the "+
					"subject.", rel)
			}

			break // one report per file is enough
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	// An entry for a file that no longer reads the clock is stale, and would
	// silently re-permit it later.
	for rel := range wallClockAllowed {
		if !found[rel] {
			t.Errorf("wallClockAllowed lists %s, which no longer reads the wall "+
				"clock. Remove the entry, or it quietly re-permits the call later.", rel)
		}
	}

	if checked < 100 {
		t.Fatalf("only %d test files scanned; the walk is not reaching the module", checked)
	}

	t.Logf("%d test files scanned, %d read the wall clock, all declared",
		checked, len(found))
}
