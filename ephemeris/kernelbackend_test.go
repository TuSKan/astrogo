package ephemeris_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/epoch"
)

// This package deliberately does not import ephemeris/jpl, so these tests run
// in a build with no kernel backend — which is the state every consumer starts
// in after #112, and the one whose error message has to be worth reading.
//
// The trade the registration makes is a compile-time failure for a runtime one.
// That is only acceptable while the runtime failure says what to do, so the
// message is tested rather than assumed.

// TestKernelSourcesWithoutABackendSayWhatToImport is the whole cost of the
// change, in one assertion.
func TestKernelSourcesWithoutABackendSayWhatToImport(t *testing.T) {
	if core.HasKernelBackend() {
		t.Skip("a backend is registered in this build; the message under test cannot occur")
	}

	for _, source := range []eph.Source{
		eph.Planets, eph.SmallBody, eph.Asteroids, eph.Comets, eph.Moons,
	} {
		_, err := eph.NewProvider(context.Background(), source, "de440")
		if !errors.Is(err, core.ErrNoKernelBackend) {
			t.Fatalf("source %v: err = %v, want ErrNoKernelBackend", source, err)
		}

		// The import path, verbatim, because a message that merely reports
		// something missing describes a build decision as though it were a
		// limit of the library.
		if !strings.Contains(err.Error(), `import _ "github.com/TuSKan/astrogo/ephemeris/jpl"`) {
			t.Errorf("source %v: the error does not name the import to add:\n  %v", source, err)
		}
	}
}

// TestSOFAPathNeedsNoBackend is the other half: everything that never wanted a
// kernel keeps working in a build that has none. If this failed, the split
// would have moved the cost rather than removed it.
func TestSOFAPathNeedsNoBackend(t *testing.T) {
	st, err := eph.At(eph.Default(), epoch.J2000()).Geometric(eph.MarsBarycenter, eph.Earth)
	if err != nil {
		t.Fatalf("Default().State(Mars): %v", err)
	}

	if st.Pos.Norm() == 0 {
		t.Error("the SOFA provider returned a zero position")
	}
}
