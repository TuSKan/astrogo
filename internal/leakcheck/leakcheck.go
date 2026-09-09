// Package leakcheck fails a package's tests when the runtime proves a
// goroutine leaked.
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
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
	"strings"
	"testing"
)

// profileName is the runtime profile that does the work.
//
// Go 1.27 added it. The runtime runs a GC cycle that marks a goroutine leaked
// when it is blocked on something nothing else can reach — an unreachable
// channel, a WaitGroup that can never be Done — so a leak here is *proved*,
// not inferred from having outlived a deadline.
//
// Measured against six shapes, it reports the first three and none of the
// rest:
//
//	blocked forever on an unreachable channel   reported
//	blocked on a WaitGroup never Done           reported
//	select {} forever                           reported
//	blocked on a mutex never unlocked           not reported
//	time.Sleep(time.Hour)                       not reported — it will wake
//	a goroutine that finished                   not reported
//
// The last two are the point. An earlier version of this package counted every
// goroutine still running when the tests ended, which meant waiting a second
// for stragglers and hoping that was long enough — a check that could report a
// slow worker as a leak and a leak as a slow worker. This one cannot: the
// sleeper is not blocked on anything unreachable, so it is simply not a leak.
//
// The mutex case is a real gap rather than a design choice, and it is worth
// knowing before trusting a clean result too far: this proves the leaks it
// reports, and does not prove their absence.
//
// One more condition, measured rather than assumed: a goroutine is only
// provably leaked once it has *parked*. Spawned and not yet scheduled, it is
// runnable, and correctly not a leak — so a check run in the same instant that
// something leaked can see nothing. That costs this package nothing, because
// [RunWithLeakCheck] runs after every test has returned, but it is why the
// tests here poll rather than look once.
const profileName = "goroutineleak"

// RunWithLeakCheck runs a package's tests and reports a non-zero exit code if
// the runtime proves any goroutine leaked.
//
// Use it from TestMain in a package that starts goroutines:
//
//	func TestMain(m *testing.M) { os.Exit(leakcheck.RunWithLeakCheck(m)) }
//
// It also catches something subtler for free: a test that forgets to Close an
// api.Client can leave the transport's connection goroutines blocked, and they
// show up here with the stack that created them.
//
// # Why not testing/synctest
//
// [testing/synctest] detects the same thing more precisely still, by waiting
// for every goroutine in its bubble, and it names the *test* rather than the
// goroutine. It is the better tool where it applies: pure concurrency, a fake
// clock, no network. It cannot be used for most of this repository, whose
// concurrent code exists specifically to talk to several observatories at
// once. This works anywhere, including under the network and integration tags.
func RunWithLeakCheck(m *testing.M) int {
	return leakCheck(m.Run(), Leaked)
}

// leakCheck is RunWithLeakCheck's decision, separated from m.Run so it can be
// tested against a probe that answers on demand.
//
// Without this seam the only testable part would be the probe, and the
// mutation that matters most — not consulting it at all — would pass
// unnoticed. A guard that silently does nothing is the exact failure this
// package exists to prevent elsewhere.
func leakCheck(code int, probe func() (string, error)) int {
	// Only when the suite passed. A failing test may have left work in flight
	// by design, and reporting that on top of the real failure buries it.
	if code != 0 {
		return code
	}

	leaked, err := probe()
	if err != nil {
		// Never silently: a guard that cannot run must say so, or a package
		// with no goroutine profile passes exactly as one with no leaks does.
		_, _ = fmt.Fprintf(os.Stderr, "leakcheck: %v\n", err)

		return 1
	}

	if leaked != "" {
		_, _ = os.Stderr.WriteString("the runtime proved these goroutines leaked:\n" +
			leaked +
			"\nEach is blocked on something nothing else can reach, so it can never " +
			"resume. Something started work and did not wait for it, or a client was " +
			"not closed.\n")

		return 1
	}

	return code
}

// Leaked returns the goroutine-leak profile when the runtime has proved any
// goroutine leaked, and "" when it has not.
//
// The error is for the profile being unavailable — a toolchain without it —
// which is deliberately not the same answer as "nothing leaked". Reporting an
// absent profile as a clean result is how a guard becomes decoration.
//
// Exported separately from [RunWithLeakCheck] for the same reason
// [github.com/TuSKan/astrogo/internal/testutil.Reachable] is exported
// separately from RequireReachable: the predicate is testable and the wrapper
// that exits the process is not.
func Leaked() (string, error) {
	p := pprof.Lookup(profileName)
	if p == nil {
		return "", fmt.Errorf("%w: this toolchain has no %q profile (Go 1.27 added it), "+
			"so nothing checked whether goroutines leaked", ErrProfileUnavailable, profileName)
	}

	var buf bytes.Buffer

	// debug=1 symbolizes the stacks, which is the difference between a report
	// naming catalog.fanOut and one naming a hexadecimal address.
	if err := p.WriteTo(&buf, 1); err != nil {
		return "", fmt.Errorf("leakcheck: writing the %q profile: %w", profileName, err)
	}

	return leakedReport(buf.String()), nil
}

// leakedReport is the profile when it holds a leak and "" when it does not.
//
// Separated from [Leaked] because it is the whole decision, and because
// [Leaked] can only read the process it runs in: by the time any test has
// leaked a goroutine on purpose, no later call can observe a clean profile
// again. A pure function can be shown both answers.
//
// The distinction is not cosmetic. The runtime emits a header even when
// nothing leaked, so returning the profile unconditionally would report every
// package as leaking, forever.
func leakedReport(profile string) string {
	if leakCount(profile) == 0 {
		return ""
	}

	return profile
}

// leakCount reads the count off the profile's header line, which reads
// "goroutineleak profile: total 3".
//
// Parsed rather than counting stack lines because one leaked goroutine can
// contribute several frames, and because the header is the runtime's own
// answer rather than this package's arithmetic over its output.
func leakCount(profile string) int {
	head, _, _ := strings.Cut(profile, "\n")

	_, total, ok := strings.Cut(head, "total ")
	if !ok {
		return 0
	}

	var n int

	for _, c := range strings.TrimSpace(total) {
		if c < '0' || c > '9' {
			break
		}

		n = n*10 + int(c-'0')
	}

	return n
}
