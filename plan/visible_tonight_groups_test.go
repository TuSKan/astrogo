package plan_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
)

// simbadStar is a SIMBAD bright-query row: position and V from SIMBAD's basic
// and allfluxes tables, and memberOf from its h_link table.
func simbadStar(id, name string, ra, dec, v float64, memberOf ...string) resolve.Target {
	return resolve.Target{
		ID: id, Name: name, Kind: resolve.KindStar, Catalog: "SIMBAD",
		Coord: coord.NewICRS(angle.Deg(ra), angle.Deg(dec)), HasCoord: true,
		VMag: v, HasVMag: true, MemberOf: memberOf,
	}
}

// The rows SIMBAD gives for alf Cen and Acrux, read through its CfA mirror on
// 2026-11-17. alf Cen's V -0.1 is A's and B's light together, and h_link
// makes A and B its members; alf01 and alf02 Cru are members of nothing.
var (
	alfCen  = simbadStar("* alf Cen", "Rigil Kentaurus", 219.9020833333333, -60.83397222222223, -0.1)
	alfCenA = simbadStar("* alf Cen A", "alf Cen A", 219.90205833170774, -60.83399268831004, 0.01, "* alf Cen")
	alfCenB = simbadStar("* alf Cen B", "alf Cen B", 219.89609628987276, -60.83752756558407, 1.33, "* alf Cen")
	alf1Cru = simbadStar("* alf01 Cru", "Acrux", 186.6495666666667, -63.09909166666667, 1.28)
	alf2Cru = simbadStar("* alf02 Cru", "alf02 Cru", 186.65184166666666, -63.09952222222222, 1.58)
)

func memberNames(vo plan.VisibleObject) []string {
	names := make([]string, len(vo.Members))
	for i, m := range vo.Members {
		names[i] = m.Target.Name
	}

	return names
}

// TestVisibleTonightListsEachPointOfLightOnce is example 20's night, which
// listed Rigil Kentaurus three times and Acrux twice (#1182). From Quinta
// Calixto on 2026-08-01 both are up after dusk. Each is now one row with its
// stars among the members: alf Cen at its system's magnitude, which holds A
// and B, and Acrux at alf01's and alf02's light combined, 0.61 mag brighter
// than alf01 alone.
func TestVisibleTonightListsEachPointOfLightOnce(t *testing.T) {
	t.Parallel()

	sources := []resolve.BrightObjectSearcher{&mockBrightSource{
		targets: []resolve.Target{alfCen, alfCenA, alfCenB, alf1Cru, alf2Cru},
	}}

	results, err := plan.VisibleTonight(context.Background(), quintaCalixtoSite(t), testNight, sources, ephemeris.Default(), 3)
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	for _, name := range []string{"alf Cen A", "alf Cen B", "alf02 Cru"} {
		if _, listed := findByName(results, name); listed {
			t.Errorf("%s is a row of its own; it is one point of light with its neighbors", name)
		}
	}

	rigil, ok := findByName(results, "Rigil Kentaurus")
	if !ok {
		t.Fatal("Rigil Kentaurus is not listed")
	}

	if got := memberNames(rigil); len(got) != 3 {
		t.Errorf("Rigil Kentaurus's members = %q, want the system and both stars", got)
	}

	// The row is the system's, at the system's magnitude dimmed by the air:
	// the components' light is in it already.
	if system := rigil.Members[0]; system.Target.ID != "* alf Cen" || rigil.ApparentMag != system.ApparentMag {
		t.Errorf("Rigil Kentaurus is at %v, its first member %q at %v; want the system's own magnitude",
			rigil.ApparentMag, system.Target.ID, system.ApparentMag)
	}

	acrux, ok := findByName(results, "Acrux")
	if !ok {
		t.Fatal("Acrux is not listed")
	}

	if got := memberNames(acrux); len(got) != 2 {
		t.Fatalf("Acrux's members = %q, want alf01 and alf02 Cru", got)
	}

	// Both stars are dimmed by the same air, so the combination is 0.61 mag
	// brighter than alf01 alone, as it is in the catalog: 1.28 against
	// 0.667096.
	if d := acrux.Members[0].ApparentMag - acrux.ApparentMag; math.Abs(d-(1.28-0.667096)) > 0.005 {
		t.Errorf("Acrux is %.4f mag brighter than alf01 alone, want %.4f", d, 1.28-0.667096)
	}
}

// TestVisibleTonightWithoutResolutionListsEveryStar: WithResolution(0) lists
// every star as a row of its own, as VisibleTonight did before observing
// groups.
func TestVisibleTonightWithoutResolutionListsEveryStar(t *testing.T) {
	t.Parallel()

	sources := []resolve.BrightObjectSearcher{&mockBrightSource{
		targets: []resolve.Target{alfCen, alfCenA, alfCenB, alf1Cru, alf2Cru},
	}}

	results, err := plan.VisibleTonight(context.Background(), quintaCalixtoSite(t), testNight, sources, ephemeris.Default(), 3,
		plan.WithResolution(0))
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	for _, name := range []string{"Rigil Kentaurus", "alf Cen A", "alf Cen B", "Acrux", "alf02 Cru"} {
		vo, ok := findByName(results, name)
		if !ok {
			t.Errorf("%s is not listed", name)
			continue
		}

		if vo.Members != nil {
			t.Errorf("%s has members %q, want none", name, memberNames(vo))
		}
	}
}
