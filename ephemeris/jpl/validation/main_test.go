package validation_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/internal/remotetest"

	// TestScientificStability (validation-tagged) compares topocentric alt/az
	// against a static Horizons corpus at sub-arcsecond tolerance, which needs
	// real DUT1 and polar motion, not the zero-EOP fallback. Without this
	// import no loader is registered and every epoch here silently uses zero
	// DUT1 and polar motion, which measured on this corpus moves the
	// topocentric gap from a p50 of 1.4 arcsec against a 3 arcsec contract to
	// a max of 8.8 arcsec — a failure that looks like a physics regression and
	// is a missing import.
	//
	// No explicit fetch call is needed: the first Time.EOP()/UTC()/UT1() query
	// any test here makes triggers epoch's lazy load (disk cache, then the
	// download, then zero-EOP degradation with a one-time warning).
	_ "github.com/TuSKan/astrogo/remote/eop"
)

// TestMain runs this package's untagged tests offline against an empty
// cache, so a test that needs the network or a cached file fails instead of
// downloading or reading what this machine holds; see remotetest.Run. The
// validation-tagged tests keep the network and the shared cache.
func TestMain(m *testing.M) { os.Exit(remotetest.Run(m.Run)) }
