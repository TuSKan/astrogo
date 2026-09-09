// Package leakcheck fails a package's tests when a goroutine outlives them.
//
// A leaked goroutine is invisible to every other check in this repository. It
// does not fail a test, does not race, does not allocate enough to notice and
// does not appear in a coverage report — it shows up in production as a
// process that grows, by which time the fan-out that leaked it is a hundred
// commits back.
//
// astrogo starts goroutines in three places: internal/parallel's chunked
// workers and catalog's two fan-outs. All of them join their work before
// returning, and this is what keeps that true.
package leakcheck

import (
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// LeakSettleTimeout bounds how long a package waits for stragglers before
// calling a goroutine leaked.
//
// A goroutine that is on its way out — an HTTP transport closing idle
// connections, a worker between its last send and its return — is not a leak,
// and the difference is only visible in time. One second is far longer than
// any of those take and far shorter than a leak, which never ends.
const LeakSettleTimeout = time.Second

// RunWithLeakCheck runs a package's tests and reports a non-zero exit code if
// any goroutine outlives them.
//
// Use it from TestMain in a package that starts goroutines:
//
//	func TestMain(m *testing.M) { os.Exit(leakcheck.RunWithLeakCheck(m)) }
//
// # Why its own package and not internal/testutil
//
// It needs a clock, and internal/testutil deliberately cannot have one: it is
// imported by nearly every test in the repository including astrogo/time's, so
// it sits below astrogo/time and expresses its one timeout as an untyped
// nanosecond constant to avoid the import. This package sits above
// astrogo/time and uses it, which docsguard's stdtime rule requires of
// everything outside time/ — a rule that caught this file living in the wrong
// place.
//
// # What this catches that nothing else does
//
// A leaked goroutine is invisible to every other check in this repository. It
// does not fail a test, does not race, does not allocate enough to notice, and
// does not appear in a coverage report. It shows up in production as a process
// that grows, and by then the fan-out that leaked it is a hundred commits back.
//
// The three places astrogo starts goroutines — internal/parallel's chunked
// workers, catalog's cone-search fan-out and its provider fan-out — all join
// their work before returning, and this guard is what keeps that true. It also
// checks something subtler for free: a package whose tests forget to Close an
// api.Client leaves the transport's connection goroutines running, so the
// guard fails and names them.
//
// # Why a package-level check and not synctest
//
// [testing/synctest] detects the same thing more precisely, by waiting for
// every goroutine in its bubble, and it is the better tool where it applies:
// pure concurrency, a fake clock, no network. It cannot be used for most of
// this repository, whose concurrent code exists specifically to talk to
// several observatories at once. This check works anywhere, including under
// the network and integration tags, at the cost of naming the goroutine rather
// than the test that started it.
//
// A failure prints each surviving goroutine's state and the first frame that
// is not the runtime or the test framework, which is normally enough to name
// the culprit; run the package with -run on a narrower set to bisect.
func RunWithLeakCheck(m *testing.M) int {
	return leakCheck(m.Run(), LeakedGoroutines)
}

// leakCheck is RunWithLeakCheck's decision, separated from m.Run so it can be
// tested against a probe that answers on demand.
//
// Without this seam the only testable part would be the predicate, and the
// mutation that matters most — not consulting the predicate at all — would
// pass unnoticed. A guard that silently does nothing is the exact failure this
// file exists to prevent elsewhere.
func leakCheck(code int, probe func(time.Duration) string) int {
	// Only when the suite passed. A failing test may have left work in flight
	// by design, and reporting that as a leak on top of the real failure buries
	// it.
	if code != 0 {
		return code
	}

	if leaked := probe(LeakSettleTimeout); leaked != "" {
		_, _ = os.Stderr.WriteString("goroutines outlived the tests:\n" + leaked +
			"\nEach line names a surviving goroutine and where it started. Something " +
			"started work and did not wait for it, or a client was not closed.\n")

		return 1
	}

	return code
}

// LeakedGoroutines returns one line per goroutine still running after the
// caller's own, or "" when everything has finished. It polls until settle
// elapses, so a goroutine on its way out is not mistaken for one that will
// never leave.
//
// Exported separately from [RunWithLeakCheck] for the same reason
// [github.com/TuSKan/astrogo/internal/testutil.Reachable] is exported
// separately from RequireReachable: the predicate is testable and the wrapper
// that exits the process is not.
func LeakedGoroutines(settle time.Duration) string {
	deadline := time.Now().Add(settle)

	for {
		found := survivingGoroutines()
		if found == "" {
			return ""
		}

		if time.Now().After(deadline) {
			return found
		}

		// Yield first, so a goroutine that only needs a scheduling slot gets
		// one without waiting out a sleep.
		runtime.Gosched()
		time.Sleep(settle / 50)
	}
}

// survivingGoroutines snapshots the stacks and drops the caller's own.
func survivingGoroutines() string {
	self := currentGoroutineHeader()

	// runtime.Stack truncates rather than failing, so the buffer is sized for
	// a pathological case: a genuine leak of hundreds of goroutines is exactly
	// when the report matters most and a truncated one names the wrong ones.
	buf := make([]byte, 1<<21)
	buf = buf[:runtime.Stack(buf, true)]

	var out []string

	for g := range strings.SplitSeq(string(buf), "\n\n") {
		if strings.TrimSpace(g) == "" {
			continue
		}

		if strings.HasPrefix(g, self) {
			continue
		}

		out = append(out, describeGoroutine(g))
	}

	sort.Strings(out)

	if len(out) == 0 {
		return ""
	}

	return strings.Join(out, "\n") + "\n"
}

// currentGoroutineHeader returns "goroutine N [" for the goroutine calling it,
// which is the only stack this check must never report.
//
// # Why identity rather than a list of names
//
// The first version filtered by frame text — testing.(*M).Run, the runtime's
// GC workers, os/signal's loop, this package's own functions. Removing each
// entry in turn showed only one of them ever fired, because runtime.Stack does
// not report system goroutines at all; the rest were guarding against stacks
// that never appear.
//
// Then trimming to that one entry broke every package that uses the guard from
// TestMain, and for a reason the name list could not express: by the time
// TestMain runs the check, m.Run has *returned*, so testing.(*M).Run is no
// longer on the stack. The goroutine to exclude is not identifiable by what it
// is running — it is running this very function — but by being this one.
//
// runtime.Stack with all=false dumps only the caller, whose header carries its
// id. Nothing else can share it.
func currentGoroutineHeader() string {
	var buf [64]byte

	n := runtime.Stack(buf[:], false)

	head, _, _ := strings.Cut(string(buf[:n]), "\n")

	// "goroutine 42 [running]:" -> "goroutine 42 [", which cannot prefix any
	// other goroutine's header.
	if i := strings.IndexByte(head, '['); i > 0 {
		return head[:i+1]
	}

	return head
}

// describeGoroutine renders one stack as its header plus the first frame that
// is not the runtime or the test framework.
//
// The header alone — "goroutine 63 [select (no cases)]:" — says what the
// goroutine is doing and nothing about who started it, which is the half a
// reader needs. Finding that out from a bare header means re-running with a
// profile; carrying one frame here means not having to.
func describeGoroutine(stack string) string {
	lines := strings.Split(stack, "\n")
	head := lines[0]

	for _, l := range lines[1:] {
		l = strings.TrimSpace(l)

		// Stacks alternate a frame line with a file:line line; the frames are
		// the ones carrying a call.
		if !strings.Contains(l, "(") || strings.Contains(l, ".go:") {
			continue
		}

		switch {
		case strings.HasPrefix(l, "runtime."),
			strings.HasPrefix(l, "testing."),
			strings.HasPrefix(l, "sync."):
			continue
		}

		return head + " " + l
	}

	return head
}
