package satellite

import (
	"math"
	"testing"
)

// TestGravityConstantsAreTheOnesSGP4Propagates asserts that what regime.go
// reads out of the constants package is, to the last digit, what the propagator
// is configured with.
//
// # Why this is worth a test of its own
//
// Because the failure it guards has already happened once, silently. This file
// used to carry private copies of these three numbers, and the copies were
// WGS-84's while the propagator was handed WGS-72 — which cost 93x the position
// error across Vallado's whole suite and was invisible to every test, because
// the arithmetic here was internally consistent with its own wrong inputs.
//
// Sourcing them from [constants.WGS72] removes the copy. It replaces it with two
// unit conversions (metres to kilometres, m³/s² to km³/s²) and a choice of which
// member to read, both of which are new ways to be wrong in exactly the same
// undetectable manner. So the values are pinned against the literals Vallado's
// getgravconst publishes for wgs72, which is the definition the propagator
// actually uses.
//
// Exact equality, not a tolerance: these are conventional constants, the
// conversions are by powers of ten, and "nearly WGS-72" is not a thing SGP4
// means.
func TestGravityConstantsAreTheOnesSGP4Propagates(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		got  float64
		want float64
	}{
		// Vallado, getgravconst(wgs72): radiusearthkm = 6378.135
		{"earth radius (km)", earthRadiusKM, 6378.135},
		// Vallado, getgravconst(wgs72): mu = 398600.8
		{"mu (km³/s²)", muKM3S2, 398600.8},
		// Vallado, getgravconst(wgs72): j2 = 0.001082616
		{"J2", j2, 0.001082616},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %.10g, want %.10g exactly — this must equal what SGP4 is "+
				"configured with, or the perigee branch reproduced here is a different "+
				"branch from the one the propagator takes", tc.name, tc.got, tc.want)
		}
	}

	// xke is derived rather than read, and it is the quantity the whole
	// Kozai-to-Brouwer correction is expressed in. Vallado computes it the same
	// way: 60/sqrt(radiusearthkm³/mu). A relative tolerance here rather than
	// equality, because this one is the result of a sqrt.
	const wantXKE = 0.07436691613317342

	if rel := math.Abs(xke-wantXKE) / wantXKE; rel > 1e-14 {
		t.Errorf("xke = %.17g, want %.17g (relative %.2g) — sqrt(GM) in earth radii^1.5 "+
			"per minute, the unit SGP4 works in", xke, wantXKE, rel)
	}
}
