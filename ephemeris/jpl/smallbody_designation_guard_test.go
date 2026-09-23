//go:build network

package jpl_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestBareNumberDoesNotReturnADifferentBody covers the quietest failure this
// path had.
//
// Asking Horizons for "1" returns comet 1000036, not 1 Ceres, because a bare
// number resolves against the major-body and comet indices before the
// numbered-asteroid record. Every mechanical check passed: a kernel arrived,
// a segment parsed, and a new body appeared in SupportedBodies. The caller
// simply got somebody else, and nothing said so.
func TestBareNumberDoesNotReturnADifferentBody(t *testing.T) {
	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")

	start := time.FromJD(2460305.5, time.TDB)
	stop := time.FromJD(2460355.5, time.TDB)

	p, err := jpl.NewProvider(context.Background(), core.SmallBody, "1",
		jpl.WithTimeInterval(start, stop))
	if err == nil {
		bodies := p.SupportedBodies()

		_ = p.Close()

		t.Fatalf(`NewProvider(..., "1") succeeded with bodies %v; it must not hand back `+
			`whatever Horizons resolves a bare "1" to`, bodies)
	}

	if !errors.Is(err, jpl.ErrWrongSmallBody) {
		// Inside the branch, not before it: the refusal this test is about
		// must never be read as an outage. Only an error that is not the
		// expected one is classified, and Horizons under load is one.
		skipIfHorizonsCouldNotAnswer(t, err)
		t.Fatalf(`NewProvider(..., "1") = %v, want ErrWrongSmallBody`, err)
	}

	t.Logf("refused as it should: %v", err)
}

// TestSemicolonDesignationLoadsTheRequestedBody is the other half: the guard
// must not reject the form that works.
func TestSemicolonDesignationLoadsTheRequestedBody(t *testing.T) {
	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")

	start := time.FromJD(2460305.5, time.TDB)
	stop := time.FromJD(2460355.5, time.TDB)

	for _, designation := range []string{"1;", "433;", "433"} {
		t.Run(designation, func(t *testing.T) {
			p, err := jpl.NewProvider(context.Background(), core.SmallBody, designation,
				jpl.WithTimeInterval(start, stop))
			if err != nil {
				// Horizons generates the kernel on request, and fails in two
				// ways that are its own: an HTTP status, and a fault reported
				// in the body of a 200. Each has its own classifier.
				if spk.TransientHorizonsFault(err) {
					t.Skipf("%q: Horizons could not serve the SPK: %v", designation, err)
				}

				testutil.SkipOnUpstreamFailure(t, err)
				t.Fatalf("%q: %v", designation, err)
			}

			defer func() { _ = p.Close() }()

			t.Logf("%q -> %v", designation, p.SupportedBodies())
		})
	}
}

// TestHorizonsRefusalIsReported covers the other silent mode: Horizons
// answering with an explanation that was thrown away.
//
// Asked for 101955 Bennu it says "SPK creation is not available for
// pre-computed objects in the major body index" — a complete answer, which
// arrived as an empty kernel list and a nil error, leaving the caller to
// guess at designation syntax that was never wrong.
func TestHorizonsRefusalIsReported(t *testing.T) {
	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")

	start := time.FromJD(2460305.5, time.TDB)
	stop := time.FromJD(2460355.5, time.TDB)

	p, err := jpl.NewProvider(context.Background(), core.SmallBody, "101955;",
		jpl.WithTimeInterval(start, stop))
	if err == nil {
		_ = p.Close()

		t.Skip("Horizons now generates an SPK for 101955; the refusal this covers is gone")
	}

	if !errors.Is(err, spk.ErrHorizonsRefused) {
		// See TestBareNumberDoesNotReturnADifferentBody for why this is inside
		// the branch.
		skipIfHorizonsCouldNotAnswer(t, err)
		t.Fatalf("NewProvider for 101955; = %v, want ErrHorizonsRefused", err)
	}

	// The service's own sentence has to survive to the caller, because it
	// is more specific than anything this package could infer.
	if !strings.Contains(err.Error(), "pre-computed objects") {
		t.Errorf("error does not carry Horizons' explanation: %v", err)
	}

	t.Logf("reported: %v", err)
}

// skipIfHorizonsCouldNotAnswer skips t when err is Horizons failing to serve
// the kernel rather than refusing the request: a status or a dropped
// connection, which testutil classifies, or a fault Horizons reports in the
// body of a 200, which spk does.
//
// Found by the tagged sweep. These tests expect a specific refusal, so the
// per-call-site audit for an unguarded fetch — which looked for a t.Fatal
// straight after `if err != nil` — did not see them, and a 503 from Horizons
// reached the "want ErrWrongSmallBody" failure instead.
func skipIfHorizonsCouldNotAnswer(t *testing.T, err error) {
	t.Helper()

	if spk.TransientHorizonsFault(err) {
		t.Skipf("Horizons could not serve the kernel: %v", err)
	}

	testutil.SkipOnUpstreamFailure(t, err)
}
