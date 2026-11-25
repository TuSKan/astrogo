//go:build integration || network || validation

package plan_test

// The kernel-backed sources reach their backend through a registration rather
// than an import, so a build that wants them says so (#112). These suites
// construct real DE441/DE442 providers, so they want them; a production plan
// build that never asks for a kernel does not, which is the point of the split.
//
// The file covers all three tags rather than "integration" alone because the
// tests needing the backend are spread across them: the small-body and
// planetary-moon queries are network-tagged, and the AstroPixels/NASA/USNO
// comparisons are validation-tagged. Gated to integration, this file was simply
// not compiled under `go test -tags=network ./plan/`, so nothing registered the
// kernel backend and 24 sub-checks across two tests failed with "this build has
// no kernel backend" (#238).
import _ "github.com/TuSKan/astrogo/ephemeris/jpl"
