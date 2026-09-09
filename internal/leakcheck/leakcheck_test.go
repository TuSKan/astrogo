package leakcheck_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"

	"github.com/TuSKan/astrogo/internal/leakcheck"
)

// TestLeakedGoroutinesFindsOne is the check that makes the guard worth having.
//
// A leak detector that has never caught anything is indistinguishable from one
// that cannot, and this one is installed as a TestMain — where a false negative
// is completely silent, because the package simply passes. So it is pointed at
// a goroutine that will never return.
//
// The leaked goroutine is deliberately not cleaned up: there is no way to stop
// a `select {}` and nothing to gain from one, since the test binary is about to
// exit anyway. It is a handful of bytes for the rest of the run.
func TestLeakedGoroutinesFindsOne(t *testing.T) {
	// Not parallel: it reads every goroutine in the process, so a sibling
	// test's workers would show up as its subject.
	started := make(chan struct{})

	go func() {
		close(started)

		select {} // never returns, which is the point
	}()

	<-started

	got := leakcheck.LeakedGoroutines(200 * time.Millisecond)
	if got == "" {
		t.Fatal("a goroutine blocked forever was not reported; a leak check that " +
			"cannot fail is a package that only looks clean")
	}

	// The report has to name something a reader can act on. "select (no
	// cases)" is what the runtime calls a goroutine parked forever.
	if !strings.Contains(got, "goroutine") {
		t.Errorf("report does not name a goroutine:\n%s", got)
	}
}

// TestLeakedGoroutinesIgnoresTheTestRunner pins the other half: the check must
// not report the goroutine running the tests, or every package fails always
// and the guard is turned off within a day.
//
// This runs in the same binary as the test above, so it cannot assert a clean
// result outright — that one's leak is still parked. What it can assert is
// that the runner and the check's own frames are not in the report, which is
// the part that would break every package rather than one.
func TestLeakedGoroutinesIgnoresTheTestRunner(t *testing.T) {
	got := leakcheck.LeakedGoroutines(50 * time.Millisecond)

	for _, unwanted := range []string{
		"testing.(*M).Run",
		"leakcheck.LeakedGoroutines",
		"leakcheck.survivingGoroutines",
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("report includes %q, which is the framework rather than a leak:\n%s",
				unwanted, got)
		}
	}
}

// TestLeakedGoroutinesWaitsForStragglers covers the settle window.
//
// A goroutine on its way out is not a leak, and the difference is only visible
// in time. Without the wait this check would fail on any test whose last
// worker had not yet been scheduled to return — a flake that would look like a
// real finding and would be chased for hours.
func TestLeakedGoroutinesWaitsForStragglers(t *testing.T) {
	done := make(chan struct{})

	go func() {
		time.Sleep(20 * time.Millisecond)
		close(done)
	}()

	// Long enough to outlast the sleeper, so it must not be reported.
	got := leakcheck.LeakedGoroutines(2 * time.Second)

	<-done

	if strings.Contains(got, "TestLeakedGoroutinesWaitsForStragglers") {
		t.Errorf("a goroutine that finished within the settle window was reported:\n%s", got)
	}
}
