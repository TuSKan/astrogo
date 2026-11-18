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
