package lsk_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// TestMain grants download consent for this package's default test suite.
// reader_test.go constructs a real jpl.Provider against the planetary
// de440s kernel — a network/cache dependency that predates remote's
// consent-gating (see ephemeris/jpl's own TestMain for the same rationale).
func TestMain(m *testing.M) {
	remote.EnableDownloads(0, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)

	os.Exit(m.Run())
}
