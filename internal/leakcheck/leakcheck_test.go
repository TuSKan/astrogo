package leakcheck_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/internal/leakcheck"
	"github.com/TuSKan/astrogo/time"
)

// settle is how long these tests will wait for a goroutine to park.
//
// The runtime proves a leak by unreachability, but only for a goroutine that
// has actually blocked — one that has been spawned and not yet scheduled is
// merely runnable, and correctly not a leak. Measured here, a single
// runtime.Gosched was enough; this is generous because a loaded CI runner is
// not this machine.
//
// It is a bound on patience, not a settle window of the kind the earlier
// version of this package needed: nothing here concludes anything from the
// time elapsing.
const (
	settle = 2 * time.Second
	poll   = 5 * time.Millisecond
)

// TestLeakedProvesTheShapesItClaims is the check that makes this guard worth
// having, and the reason it is not simply trusted.
//
// A leak detector that has never caught anything is indistinguishable from one
// that cannot, and this one runs from TestMain — where a false negative is
// completely silent, because the package just passes.
//
// The shapes are the ones the doc comment claims, asserted rather than
// described. The leaked goroutines are deliberately never released: there is
// no way to release a goroutine blocked on an unreachable channel, which is
// the definition being tested.
func TestLeakedProvesTheShapesItClaims(t *testing.T) {
	// Not parallel: the profile is process-wide, so a sibling test's
	// goroutines would appear in this one's result.
	before := countLeaks(t, mustLeaked(t))

	// Blocked forever on a channel that goes out of scope with it.
	func() {
		ch := make(chan int)

		go func() { <-ch }()
	}()

	// Blocked on a WaitGroup nothing will ever mark done.
	func() {
		var wg sync.WaitGroup

		wg.Add(1)

		go func() { wg.Wait() }()
	}()

	after, report := waitForLeaks(t, before+2)
	if after < before+2 {
		t.Fatalf("two goroutines blocked on unreachable synchronisation were reported "+
			"as %d leaks, want at least %d; a leak check that cannot fail is a package "+
			"that only looks clean\n%s", after, before+2, report)
	}

	// The report has to name somewhere a reader can go.
	if !strings.Contains(report, "leakcheck_test") {
		t.Errorf("the report does not name the code that leaked:\n%s", report)
	}
}

// TestLeakedIgnoresAGoroutineThatWillWake is the half an earlier version of
// this package could not get right.
//
// That version counted every goroutine still running when the tests ended, so
// it needed a settle window and a guess at how long stragglers take — which
// makes a slow worker indistinguishable from a leak in both directions. The
// runtime's definition has no such ambiguity: a sleeping goroutine is blocked
// on a timer that will fire, so it is not leaked, however long it sleeps.
//
// The count is re-read repeatedly rather than once, so that the sleeper is
// given every chance to be miscounted rather than merely being too young to
// notice.
func TestLeakedIgnoresAGoroutineThatWillWake(t *testing.T) {
	before := countLeaks(t, mustLeaked(t))

	// An hour is far longer than any test run, so if duration mattered this
	// would be reported.
	go func() { time.Sleep(time.Hour) }()

	// And one that is merely slow rather than stuck.
	done := make(chan struct{})

	go func() {
		time.Sleep(20 * time.Millisecond)
		close(done)
	}()

	for range 20 {
		report := mustLeaked(t)
		if got := countLeaks(t, report); got != before {
			t.Fatalf("a goroutine that will wake was reported as leaked: %d, want %d\n%s",
				got, before, report)
		}

		time.Sleep(poll)
	}

	<-done
}

// mustLeaked reads the profile, failing the test rather than the package if it
// is unavailable.
func mustLeaked(t *testing.T) string {
	t.Helper()

	report, err := leakcheck.Leaked()
	if err != nil {
		t.Fatalf("Leaked: %v", err)
	}

	return report
}

// waitForLeaks polls until the profile reports at least want leaks, and
// returns what it last saw either way.
//
// Polling is for the goroutines to park, not for a leak to develop: a parked
// goroutine's unreachability is decided by the GC cycle the profile runs, and
// that answer does not change with more waiting.
func waitForLeaks(t *testing.T, want int) (int, string) {
	t.Helper()

	var (
		report string
		got    int
	)

	for waited := time.Duration(0); waited < settle; waited += poll {
		report = mustLeaked(t)

		if got = countLeaks(t, report); got >= want {
			return got, report
		}

		time.Sleep(poll)
	}

	return got, report
}

// countLeaks reads the count off the profile header, or 0 when the profile is
// empty.
func countLeaks(t *testing.T, profile string) int {
	t.Helper()

	if profile == "" {
		return 0
	}

	head, _, _ := strings.Cut(profile, "\n")

	_, total, ok := strings.Cut(head, "total ")
	if !ok {
		t.Fatalf("profile header has no total: %q", head)
	}

	n := 0

	for _, c := range strings.TrimSpace(total) {
		if c < '0' || c > '9' {
			break
		}

		n = n*10 + int(c-'0')
	}

	return n
}
