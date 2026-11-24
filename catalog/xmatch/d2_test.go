package xmatch

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// withErrors is a still Target at (10°, 20° + northArcsec) at J2000, with a
// position error of sigmaArcsec on each axis.
func withErrors(id string, northArcsec, sigmaArcsec float64) resolve.Target {
	return resolve.Target{
		ID: id, HasCoord: true, Epoch: time.J2000(),
		Coord:            coord.NewICRS(angle.Deg(10), angle.Deg(20).Add(angle.Arcsec(northArcsec))),
		AstrometricError: resolve.AstrometricError{RA: angle.Arcsec(sigmaArcsec), Dec: angle.Arcsec(sigmaArcsec)},
	}
}

// TestMatchWeighsPublishedErrors: B1, 0.1″ from A with both known to 1 mas,
// is another object (D² = 5000) although it is closest and within 2″; B2,
// 1.5″ away and known to 1″, may be A (D² = 2.25) and is its pair.
func TestMatchWeighsPublishedErrors(t *testing.T) {
	t.Parallel()

	a := withErrors("A", 0, 0.001)
	pairs := Match([]resolve.Target{a}, []resolve.Target{withErrors("B1", 0.1, 0.001), withErrors("B2", 1.5, 1)})

	if len(pairs) != 1 || pairs[0].B.ID != "B2" {
		t.Fatalf("pairs %+v, want A with B2", pairs)
	}
}

// TestMatchWithNoResolutionPairsByIdentifiersAlone: by default no resolution
// is set, so two Targets that publish no position error and share no
// identifier are not paired however close they are, while two sharing an
// identifier are paired however far apart.
func TestMatchWithNoResolutionPairsByIdentifiersAlone(t *testing.T) {
	t.Parallel()

	near := withErrors("A1", 0, 0)
	nearby := withErrors("B1", 0.5, 0)
	named := withErrors("A2", 0, 0)
	named.Aliases = []string{"HD 37128"}
	far := withErrors("B2", 3600, 0)
	far.Aliases = []string{"HD 37128"}

	pairs := Match([]resolve.Target{near, named}, []resolve.Target{nearby, far})
	if len(pairs) != 1 || pairs[0].A.ID != "A2" || pairs[0].B.ID != "B2" {
		t.Fatalf("pairs %+v, want only A2 with B2, by their shared identifier", pairs)
	}

	if pairs := Match([]resolve.Target{near}, []resolve.Target{nearby}, WithResolution(angle.Arcsec(1))); len(pairs) != 1 {
		t.Errorf("with a 1″ resolution: %d pairs, want A1 with B1", len(pairs))
	}
}
