//go:build integration

package workload_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/remote"

	// Observed places depend on UT1 and polar motion, and a benchmark of them
	// without Earth Orientation Parameters measures a different computation:
	// every epoch would silently take zero DUT1.
	_ "github.com/TuSKan/astrogo/remote/eop"

	// eph.NewProvider reaches the SPK backend through a registration.
	_ "github.com/TuSKan/astrogo/ephemeris/jpl"
)

// TestMain grants the consent jpl's own suite grants: the planetary kernel
// and its leap-second file, and the EOP bulletin. Nothing here asks for
// anything larger — the one satellite kernel a benchmark uses is read only if
// a previous consented run already cached it (see openCachedKernel).
func TestMain(m *testing.M) {
	remote.EnableDownloads(0, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	os.Exit(m.Run())
}
