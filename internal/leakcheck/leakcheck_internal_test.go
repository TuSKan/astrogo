package leakcheck

import (
	"runtime"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// The three functions behind the leak guard are tested here, in-package,
// against synthetic stack text.
//
// The external tests point the whole guard at a real leaked goroutine, which
// proves it fires. They cannot prove what it *does not* fire on, because the
// report renders only a goroutine's header and one frame — so an assertion
// that the runner's frame is absent from the output passes whether or not the
// runner is filtered. Mutating the filter away left those tests green. These
// close that.

// A stack fragment taken verbatim from a real runtime.Stack dump.
const (
	leakStack = `goroutine 42 [select (no cases)]:
github.com/TuSKan/astrogo/catalog.fanOut.func2()
	D:/Developments/astrogo/catalog/catalog.go:681 +0x8c
created by github.com/TuSKan/astrogo/catalog.fanOut
	D:/Developments/astrogo/catalog/catalog.go:678 +0x1f4`
)

// TestDescribeGoroutineNamesWhereItStarted covers the half of the report that
// makes it actionable.
//
// The header alone says what a goroutine is doing — "select (no cases)" — and
// nothing about who started it, which is the half a reader needs. Mutating the
// frame away left every external test green, because they only asserted that
// the report was non-empty.
func TestDescribeGoroutineNamesWhereItStarted(t *testing.T) {
	t.Parallel()

	got := describeGoroutine(leakStack)

	for _, want := range []string{
		"goroutine 42",           // which one
		"select (no cases)",      // what it is doing
		"astrogo/catalog.fanOut", // where it came from
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describeGoroutine = %q, want it to contain %q", got, want)
		}
	}

	// The file:line lines are noise in a one-line-per-goroutine report.
	if strings.Contains(got, ".go:") {
		t.Errorf("describeGoroutine = %q, want no file:line noise", got)
	}
}

// TestDescribeGoroutineFallsBackToTheHeader covers a stack with nothing but
// runtime frames — better a header alone than an empty line that says nothing.
func TestDescribeGoroutineFallsBackToTheHeader(t *testing.T) {
	t.Parallel()

	const runtimeOnly = `goroutine 7 [syscall]:
runtime.notetsleepg(0x8a2d20, 0xdf8475800)
	C:/Program Files/Go/src/runtime/lock_sema.go:295 +0x33`

	got := describeGoroutine(runtimeOnly)
	if !strings.HasPrefix(got, "goroutine 7 [syscall]:") {
		t.Errorf("describeGoroutine = %q, want it to fall back to the header", got)
	}
}

// TestLeakCheckConsultsTheProbe is the mutation that mattered most: a guard
// that never asks whether anything leaked.
//
// It is invisible from outside — the package passes, which is exactly what it
// would do if nothing had leaked — so nothing but this would notice.
func TestLeakCheckConsultsTheProbe(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		suiteCode  int
		probeSays  string
		wantCode   int
		wantProbed bool
	}{
		{"a clean suite with a leak fails", 0, "goroutine 42 [select]:", 1, true},
		{"a clean suite with no leak passes", 0, "", 0, true},
		{"a failing suite is not masked", 1, "goroutine 42 [select]:", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			probed := false
			probe := func(time.Duration) string {
				probed = true

				return tc.probeSays
			}

			if got := leakCheck(tc.suiteCode, probe); got != tc.wantCode {
				t.Errorf("leakCheck = %d, want %d", got, tc.wantCode)
			}

			if probed != tc.wantProbed {
				t.Errorf("probe consulted = %v, want %v — a guard that does not ask "+
					"cannot answer", probed, tc.wantProbed)
			}
		})
	}
}

// TestCurrentGoroutineHeaderIdentifiesOnlyTheCaller covers the filter that
// replaced a list of frame names.
//
// The name list looked right and was wrong in a way no unit test caught: by
// the time TestMain runs the check, m.Run has returned, so testing.(*M).Run is
// no longer on the stack and the check reported itself. Every package using
// the guard failed. Identity cannot drift that way — the goroutine to exclude
// is the one asking.
func TestCurrentGoroutineHeaderIdentifiesOnlyTheCaller(t *testing.T) {
	self := currentGoroutineHeader()

	if !strings.HasPrefix(self, "goroutine ") || !strings.HasSuffix(self, "[") {
		t.Fatalf("header = %q, want the form \"goroutine N [\"", self)
	}

	// It must match this goroutine in a full dump, and exactly one of them.
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]

	matches := 0

	for g := range strings.SplitSeq(string(buf), "\n\n") {
		if strings.HasPrefix(g, self) {
			matches++
		}
	}

	if matches != 1 {
		t.Errorf("header %q matched %d goroutines in a full dump, want exactly 1 — "+
			"a prefix that matches none reports the checker as a leak, and one that "+
			"matches several hides real ones", self, matches)
	}
}

// TestSurvivingGoroutinesDoesNotReportItself is the regression for the failure
// that trimming the old filter exposed: run from anywhere, the check must
// never name its own goroutine.
func TestSurvivingGoroutinesDoesNotReportItself(t *testing.T) {
	got := survivingGoroutines()

	if strings.Contains(got, "survivingGoroutines") {
		t.Errorf("the check reported itself:\n%s", got)
	}

	if strings.Contains(got, "currentGoroutineHeader") {
		t.Errorf("the check reported its own helper:\n%s", got)
	}
}
