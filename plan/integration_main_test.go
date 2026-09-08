//go:build integration

package plan_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/remote"

	// The kernel-backed sources reach their backend through a registration
	// rather than an import, so a build that wants them says so (#112). This
	// suite constructs real DE441/DE442 providers, so it wants them; a
	// production plan build that never asks for a kernel does not, which is
	// the point of the split.
	_ "github.com/TuSKan/astrogo/ephemeris/jpl"
)

// TestMain grants download consent for this package's integration-tagged
// suite, which constructs real eph.NewProvider(ctx, eph.Planets, ...) instances
// against large JPL kernels (de442 ~115 MB, de441 parts multi-GB each) —
// a network/cache dependency that predates remote's consent-gating (see
// ephemeris/jpl's own TestMain for the same rationale). Since these tests
// only run under the "integration" build tag, this consent is scoped to
// that explicit opt-in rather than the default test suite.
func TestMain(m *testing.M) {
	remote.EnableDownloads(0, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)
	remote.EnableDownloads(0, remote.JPLHorizonsSPK)

	os.Exit(m.Run())
}
