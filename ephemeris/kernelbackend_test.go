package ephemeris_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
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
	st, err := eph.Default().State(eph.Mars, time.J2000)
	if err != nil {
		t.Fatalf("Default().State(Mars): %v", err)
	}

	if st.Pos.Norm() == 0 {
		t.Error("the SOFA provider returned a zero position")
	}
}

// TestSatellitesNeedNoBackend: SGP4 is not kernel-backed, and grouping it with
// the sources that are would be an easy mistake to make in the switch this
// change edited.
func TestSatellitesNeedNoBackend(t *testing.T) {
	// A real ISS element set, checksum and all. My first attempt invented the
	// digits and the parser refused it, correctly — a TLE carries its own
	// checksum and this package validates it.
	const (
		l1 = "1 25544U 98067A   26109.48995873  .00010082  00000-0  19194-3 0  9999"
		l2 = "2 25544  51.6329 230.6068 0006631 325.6576  34.3983 15.48833250562656"
	)

	p, err := eph.NewProvider(context.Background(), eph.Satellites, "ISS", eph.WithTLE(l1, l2))
	if err != nil {
		t.Fatalf("Satellites source failed in a build with no kernel backend: %v", err)
	}

	defer func() { _ = p.Close() }()
}
