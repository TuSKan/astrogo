//go:build integration

package plan_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// requireKernel turns a provider-construction failure into the right kind of
// test outcome, and exists because getting that wrong cost this suite six red
// checks on a pull request that touched none of these packages (#348).
//
// # The service under test is not the service that fails
//
// Every test in this file's neighbours validates astrogo against an external
// reference — AstroPixels' phase tables, NASA's eclipse catalogues, USNO's
// almanac — and each already skips when *that* service cannot be reached.
// None of them guarded the step before: loading a DE44x kernel, which is a
// download from NAIF at jpl.nasa.gov and an entirely different service.
//
// So NAIF going down failed tests whose subject is a NASA eclipse page, with
// the message "Failed to create DE441 provider". Four of the six failures in
// #348 were that, and the repository's own rule is that external downtime must
// never read as a failure — CLAUDE.md, under the network tag: "never fail CI
// for external downtime".
//
// # Why skip rather than fall back
//
// plan/usno_test.go's newEph falls back to eph.Default() when a kernel will not
// load, which is right for a test that only needs *an* ephemeris. It is wrong
// here: these tests compare astrogo's DE441 answers against a published table
// to arcsecond or better, and the analytic default is not that. Silently
// swapping it in would turn an unreachable NAIF into a wrong-looking comparison
// against the right reference, which is worse than either a skip or a failure.
//
// # A slow NAIF is a NAIF that is down
//
// The fetch runs under kernelContext, which ends before this binary does, so a
// download NAIF is too slow to finish ends in context.DeadlineExceeded rather
// than hanging. That is read as the upstream's failure, the same as a 5xx or a
// refused dial: testutil.UpstreamFailure, not the narrower Unreachable, which
// deliberately declines a bare deadline.
//
// # What stays fatal
//
// Anything that is not the network. A kernel that downloads and then fails to
// parse, a download refused for want of consent, a missing body — all of those
// are astrogo's problem or the caller's, and all of them keep failing the
// build.
func requireKernel(t *testing.T, what string, err error) {
	t.Helper()

	if err == nil {
		return
	}

	if reason, ok := testutil.UpstreamFailure(err); ok {
		t.Skipf("%s: NAIF did not deliver the kernel (%s): %v (external, not astrogo)", what, reason, err)
	}

	t.Fatalf("%s: %v", what, err)
}

// kernelMargin is what kernelContext leaves of this test binary's budget for
// the rest of the package. Without a download to make, the package's other
// integration tests take about a minute and a half on CI.
const kernelMargin = 3 * time.Minute

// kernelContext is the context to fetch a kernel under. It ends kernelMargin
// before this test binary's own deadline.
//
// remote.NAIFSPK registers a 30-minute DownloadTimeout, right for a caller
// deliberately fetching a 1.6 GB DE441 part and longer than this binary's whole
// budget. Unbounded, a slow NAIF did not skip a test, it hung it:
// TestAstroPixels_MoonPhases sat inside the download for 9m32s and the package
// died on its own timeout, failing a pull request that touched nothing here
// (#471).
// Bounded, the first fetch ends in context.DeadlineExceeded and skips through
// requireKernel, every later one finds the deadline already past and skips at
// once, and the rest of the package still runs.
//
// With no -timeout there is no budget to protect, and the fetch is unbounded.
func kernelContext(t *testing.T) context.Context {
	t.Helper()

	deadline, ok := t.Deadline()
	if !ok {
		return t.Context()
	}

	ctx, cancel := context.WithDeadline(t.Context(), deadline.Add(-kernelMargin))
	t.Cleanup(cancel)

	return ctx
}

// TestKernelContextLeavesTheBinaryItsMargin holds kernelContext to its
// arithmetic: a fetch under it must end kernelMargin before the binary does.
func TestKernelContextLeavesTheBinaryItsMargin(t *testing.T) {
	t.Parallel()

	binary, bounded := t.Deadline()
	got, hasDeadline := kernelContext(t).Deadline()

	switch {
	case !bounded && hasDeadline:
		t.Errorf("no -timeout, yet the kernel context ends at %v", got)
	case bounded && !hasDeadline:
		t.Errorf("the binary ends at %v, and the kernel context never does", binary)
	case bounded && !got.Equal(binary.Add(-kernelMargin)):
		t.Errorf("the kernel context ends at %v, want %v before the binary's %v", got, kernelMargin, binary)
	}
}

// TestRequireKernelSkipsAnExhaustedBudget: a fetch that runs out of
// kernelContext's budget is NAIF's slowness, and skips. The narrower
// testutil.Unreachable, which requireKernel used to ask, declines a bare
// deadline, and this was a failure.
func TestRequireKernelSkipsAnExhaustedBudget(t *testing.T) {
	t.Parallel()

	var inner *testing.T

	// Read after the subtest has finished: a parallel subtest runs once this
	// function returns, and the parent's cleanup waits for it.
	t.Cleanup(func() {
		if !inner.Skipped() {
			t.Error("a fetch that ran out of its budget did not skip")
		}
	})

	t.Run("budget exhausted", func(t *testing.T) {
		t.Parallel()

		inner = t
		requireKernel(t, "a kernel fetched under kernelContext", context.DeadlineExceeded)
	})
}
