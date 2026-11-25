package plan_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/remotetest"
)

// TestMain pins Earth orientation to zero, and runs this package's untagged
// tests offline against an empty cache (see remotetest.Run).
//
// Zero in every build, because these tests were written against it: before
// epoch loaded the IERS bulletin itself (#1264) nothing registered a loader in
// this package's tests, tagged or not. The Skyfield comparisons rest on it,
// since Skyfield applies no polar motion, and so does the subsolar point's
// agreement with local sidereal time. A tagged build keeps the network and
// the shared cache, and would otherwise apply this machine's bulletin.
func TestMain(m *testing.M) {
	epoch.RegisterModel(epoch.ZeroModel{})

	os.Exit(remotetest.Run(m.Run))
}
