package coord_test

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
// this package's tests, tagged or not, and several assert what holds only
// without one. A context window's interpolation is held to 0.01 mas, the frame
// to turn at exactly the sidereal rate over an hour of UT1, an example to print
// its numbers. A tagged build keeps the network and the shared cache, so
// without the pin those would follow whatever bulletin this machine holds. A
// test that sets a model of its own puts the pin back with keepEOP.
func TestMain(m *testing.M) {
	epoch.RegisterModel(epoch.ZeroModel{})

	os.Exit(remotetest.Run(m.Run))
}

// keepEOP puts back, when tb ends, the Earth orientation it found: the zero
// model TestMain pins, chosen explicitly, rather than epoch.ResetEOP's default,
// which would let the lazy load replace it for every test after this one.
func keepEOP(tb testing.TB) {
	tb.Helper()

	model, explicit := epoch.GetModel(), epoch.EOPSource() == "explicit"

	tb.Cleanup(func() {
		if explicit {
			epoch.RegisterModel(model)

			return
		}

		epoch.ResetEOP()
	})
}
