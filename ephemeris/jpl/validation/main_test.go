package validation_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/internal/remotetest"
)

// TestMain runs this package's untagged tests offline against an empty
// cache, so a test that needs the network or a cached file fails instead of
// downloading or reading what this machine holds; see remotetest.Run. The
// validation-tagged tests keep the network and the shared cache, and with it
// the IERS bulletin TestScientificStability's sub-arcsecond tolerance needs:
// without it every epoch takes zero DUT1 and polar motion, which on this
// corpus moves the topocentric gap from a p50 of 1.4 arcsec against a 3 arcsec
// contract to a max of 8.8 arcsec.
func TestMain(m *testing.M) { os.Exit(remotetest.Run(m.Run)) }
