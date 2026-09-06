package satellite_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite"
)

// The measured behaviour of Satellite.Verified is asserted against Vallado's
// suite under the validation tag. These are the ordinary-tier tests, so the
// predicate has coverage in a plain `go test ./...` too — a signal about
// trustworthiness should not itself be untested outside a tagged run.

// TestVerifiedClearsAnOrdinaryOrbit uses the ISS: a 400 km perigee, nowhere
// near SGP4's simplified-drag branch, and the case the library is most often
// pointed at.
func TestVerifiedClearsAnOrdinaryOrbit(t *testing.T) {
	t.Parallel()

	sat, err := satellite.NewFromTLE("ISS", valladoLine1, valladoLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	ok, reason := sat.Verified()
	if !ok {
		t.Errorf("the ISS was flagged as unverified: %s", reason)
	}

	if reason != "" {
		t.Errorf("a cleared satellite carried a reason: %q", reason)
	}
}

// TestVerifiedFlagsALowPerigeeOrbit is the other half, on the worst case in
// Vallado's suite: COSMOS 2405, perigee 127 km, which astrogo misses by
// 3440 km.
func TestVerifiedFlagsALowPerigeeOrbit(t *testing.T) {
	t.Parallel()

	// Vallado's own annotation: "Near Earth, perigee = 127.20 (< 156) s4 mod".
	const (
		line1 = "1 28350U 04020A   06167.21788666  .16154492  76267-5  18678-3 0  8894"
		line2 = "2 28350  64.9977 345.6130 0024870 260.7578  99.9590 16.47856722116490"
	)

	sat, err := satellite.NewFromTLE("COSMOS 2405", line1, line2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	ok, reason := sat.Verified()
	if ok {
		t.Fatal("a 127 km perigee was reported as verified; that is the case astrogo " +
			"misses by 3440 km")
	}

	// The reason has to be usable: a caller logging it needs to know what was
	// wrong and roughly how much it matters, not just that something was.
	for _, want := range []string{"perigee", "220", "3440"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q does not mention %q", reason, want)
		}
	}
}
