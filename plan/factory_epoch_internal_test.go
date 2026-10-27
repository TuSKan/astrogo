package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestElementsFromTargetKeepsTheOsculationEpoch: a comet published in comet
// form is built osculating at the epoch its source gave (#803), as the MPC's
// comet file gives one for Hale-Bopp, and at its perihelion time when the
// source gave none, as SBDB's comet answers do not.
func TestElementsFromTargetKeepsTheOsculationEpoch(t *testing.T) {
	t.Parallel()

	tp := time.FromJDParts(2450536.5, 0.0319, time.TT)
	epoch := time.FromJDParts(2461305.5, 0, time.TT)

	halebopp := resolve.Target{
		Name: "C/1995 O1 (Hale-Bopp)", Kind: resolve.KindComet, HasElements: true,
		Epoch:              epoch,
		Eccentricity:       0.994897,
		PerihelionDistance: unit.AU(0.925246),
		PerihelionTime:     tp,
		Inclination:        angle.Deg(89.7360),
		AscendingNode:      angle.Deg(281.8047),
		ArgPeriapsis:       angle.Deg(130.7250),
	}

	el, err := elementsFromTarget(halebopp)
	if err != nil {
		t.Fatal(err)
	}

	if !el.Epoch().Equal(epoch) {
		t.Errorf("Epoch = %v, want the published osculation epoch %v", el.Epoch(), epoch)
	}

	unpublished := halebopp
	unpublished.Epoch = time.Time{}

	el, err = elementsFromTarget(unpublished)
	if err != nil {
		t.Fatal(err)
	}

	if !el.Epoch().Equal(tp) {
		t.Errorf("Epoch with none published = %v, want the perihelion time %v", el.Epoch(), tp)
	}
}
