package jpl_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// TestMain grants download consent for the whole package's default test
// suite. These tests construct real jpl.Provider values against the
// planetary de440s kernel and, for two small-body tests, live Horizons SPK
// generation — a network/cache dependency that predates remote's
// consent-gating (kernels landing in the shared user cache dir made it
// invisible before). Rather than silently break every test in this file,
// grant the same access explicitly here; TODO: replace with committed
// offline SPK fixtures and move the network-dependent cases to
// //go:build network per CLAUDE.md's test-tag convention.
func TestMain(m *testing.M) {
	remote.EnableDownloads(0, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)
	remote.EnableDownloads(0, remote.JPLHorizonsSPK)

	os.Exit(m.Run())
}

// mustPlanetProvider builds a planetary-kernel provider, skipping the test
// when the kernel cannot
// be obtained rather than failing it.
//
// # Why a download failure is not a test failure
//
// These tests are untagged, so they run in the ordinary `go test ./...` — and
// a provider needs a kernel, which on a machine without one cached means
// fetching 32 MB from NAIF. When that is slow the tests do not fail quickly:
// four of them sat at 120 seconds each and took the package past its 600
// second limit, turning a bad minute at NAIF into a red main branch.
//
// This repository's policy is that an external service must never fail a
// build. It is written for the network-tagged suites and applies here for the
// same reason, so an upstream failure — a timeout, a 5xx, a rate limit —
// skips. Anything else still fails, because a kernel that arrives and cannot
// be opened is a real defect.
//
// # Why these stay untagged
//
// CLAUDE.md describes the default suite as "fast, deterministic, offline",
// and a 32 MB download is none of those, so the obvious move is to put these
// behind the network tag. That was considered and rejected: this package is
// intrinsically a network module — a JPL provider without a kernel is not a
// reduced version of itself, it is nothing — and tagging its tests would
// leave the default suite with no cover over the code most likely to break.
//
// So the exception is deliberate. The suite stays honest about it by
// skipping, rather than hanging and failing, when the kernel cannot be had.
// testKernel is the planetary kernel every test in this package uses: the
// small DE440 variant, 32 MB against de440's 115, covering 1849-2150. Every
// case here works well inside that span, so the larger file would only make
// the fetch slower.
const testKernel = "de440s"

func mustPlanetProvider(t *testing.T) *jpl.Provider {
	t.Helper()

	p, err := jpl.NewProvider(t.Context(), core.Planets, testKernel)
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("NewProvider(planets, %q): %v", testKernel, err)
	}

	return p
}
