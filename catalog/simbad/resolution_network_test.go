//go:build network

package simbad

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// simbadTAPHost is the reachability pre-check target, per this repository's
// policy that an unreachable external service skips rather than fails.
const simbadTAPHost = "simbad.cds.unistra.fr:80"

// TestResolveReturnsTheObjectAsked is the test that would have caught the
// substring-matching defect, and the reason it asserts positions.
//
// A name check alone passes on the wrong object: resolving "M87" returned
// `[AKM87] 23`, whose name plainly contains the query, while sitting 70
// degrees away in Cassiopeia. Only the coordinate says which object it is.
//
// The tolerance is a degree, which is enormous by this repository's standards
// and deliberately so: this is an identity check, not an astrometric one. The
// failures it exists to catch were 61 and 70 degrees, and an object 0.3
// degrees away — a catalogued source *inside* the galaxy asked for — is also
// the wrong object and also fails.
func TestResolveReturnsTheObjectAsked(t *testing.T) {
	testutil.RequireReachable(t, simbadTAPHost)

	const toleranceDeg = 1.0

	for _, tc := range []struct {
		query, wantID string
		raDeg, decDeg float64
		why           string
	}{
		{"M31", "M  31", 10.6847, 41.2688, "returned a nova inside the galaxy"},
		{"M87", "M  87", 187.7059, 12.3911, "returned a 2MASS source 70 degrees away"},
		{"M42", "M  42", 83.8201, -5.3876, "returned an object with no coordinates at all"},
		{"M13", "M  13", 250.4235, 36.4613, "returned an object 61 degrees away"},
		{"NGC 5128", "NAME Centaurus A", 201.3651, -43.0191, "returned a companion source"},
		{"Sirius", "Sirius", 101.2872, -16.7161, "the one name that always worked"},
		{"Andromeda Galaxy", "M  31", 10.6847, 41.2688, "a common name, via SIMBAD's NAME prefix"},
		{"m31", "M  31", 10.6847, 41.2688, "lower case"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			p := New()

			tgt, ok := p.Resolve(context.Background(), tc.query)
			if !ok {
				t.Fatalf("Resolve(%q) found nothing (%s)", tc.query, tc.why)
			}

			if !tgt.HasCoord {
				t.Fatalf("Resolve(%q) returned %q with no coordinates", tc.query, tgt.Name)
			}

			dRA := math.Abs(tgt.Coord.RA().Degrees()-tc.raDeg) * math.Cos(tc.decDeg*math.Pi/180)
			dDec := math.Abs(tgt.Coord.Dec().Degrees() - tc.decDeg)

			if sep := math.Hypot(dRA, dDec); sep > toleranceDeg {
				t.Errorf("Resolve(%q) returned %q at (%.4f, %+.4f), %.2f degrees from %q at "+
					"(%.4f, %+.4f) — %s",
					tc.query, tgt.Name, tgt.Coord.RA().Degrees(), tgt.Coord.Dec().Degrees(),
					sep, tc.wantID, tc.raDeg, tc.decDeg, tc.why)
			}
		})
	}
}

// TestResolveFindsNothingRatherThanSomethingWrong pins the other half of the
// contract: an unknown name is an answer a caller can act on, and the least
// wrong of ten wrong objects is not.
func TestResolveFindsNothingRatherThanSomethingWrong(t *testing.T) {
	testutil.RequireReachable(t, simbadTAPHost)

	p := New()

	if tgt, ok := p.Resolve(context.Background(), "NoSuchObjectXYZ123"); ok {
		t.Errorf("Resolve of a nonexistent name returned %q", tgt.Name)
	}
}
