//go:build integration

package plan_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// TestMain grants download consent for this package's integration-tagged
// suite, which constructs real eph.NewProvider(ctx, eph.Planets, ...) instances
// against large JPL kernels (de442 ~115 MB, de441 parts multi-GB each) —
// a network/cache dependency that predates remote's consent-gating (see
// ephemeris/jpl's own TestMain for the same rationale). Since these tests
// only run under the "integration" build tag, this consent is scoped to
// that explicit opt-in rather than the default test suite.
func TestMain(m *testing.M) {
	remote.EnableDownloads(remote.NAIFSPK, 0)
	remote.EnableDownloads(remote.NAIFLSK, 0)
	remote.EnableDownloads(remote.JPLHorizons, 0)

	os.Exit(m.Run())
}
