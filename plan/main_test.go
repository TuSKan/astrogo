//go:build integration || network || validation

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

// TestMain grants download consent for this package's tagged suites, which
// construct real eph.NewProvider(ctx, eph.Planets, ...) instances against
// large JPL kernels (de442 ~115 MB, de441 parts multi-GB each) — a
// network/cache dependency that predates remote's consent-gating (see
// ephemeris/jpl's own TestMain for the same rationale). The default,
// untagged suite still gets neither the consent nor the backend, which is
// the split's actual point.
//
// It covers all three tags rather than "integration" alone because the tests
// needing it are spread across them: the small-body and planetary-moon
// queries are network-tagged, and the AstroPixels/NASA/USNO comparisons are
// validation-tagged. Gated to integration, this file was simply not compiled
// under `go test -tags=network ./plan/` — the command CLAUDE.md documents and
// the one pre-release.yml runs on its own — so nothing registered the kernel
// backend and 24 sub-checks across two tests failed with "this build has no
// kernel backend", a build-configuration problem wearing a capability
// error's clothes. It was invisible locally because the full gate runs all
// three tags at once, where this file does compile (#238).
func TestMain(m *testing.M) {
	remote.EnableDownloads(0, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)
	remote.EnableDownloads(0, remote.JPLHorizonsSPK)

	os.Exit(m.Run())
}
