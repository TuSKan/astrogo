package plan_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
)

// TestVisibleTonightListsAnObjectOnce: two sources that both carry an object
// listed it twice, SIMBAD's Alnilam beside OpenNGC's NGC 1990, which OpenNGC
// catalogs as the star itself (#1136). They share identifiers, and are now
// one row. Rigil Kentaurus and its component A, 8 arcsec apart but with
// identifiers of their own, stay two: nothing merges by position.
func TestVisibleTonightListsAnObjectOnce(t *testing.T) {
	t.Parallel()

	at := func(raDeg, decDeg float64) coord.ICRS { return coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg)) }

	// SIMBAD's rows: an ID with its "* " marker and the friendly name.
	simbad := &mockBrightSource{targets: []resolve.Target{
		{ID: "* eps Ori", Name: "Alnilam", Kind: resolve.KindStar, Coord: at(84.053389, -1.201919), HasCoord: true, VMag: 1.69, HasVMag: true, Catalog: "SIMBAD"},
		{ID: "* alf Cen", Name: "Rigil Kentaurus", Kind: resolve.KindStar, Coord: at(219.902083, -60.833972), HasCoord: true, VMag: -0.1, HasVMag: true, Catalog: "SIMBAD"},
		{ID: "* alf Cen A", Name: "Rigil Kentaurus A", Kind: resolve.KindStar, Coord: at(219.902058, -60.833993), HasCoord: true, VMag: 0.01, HasVMag: true, Catalog: "SIMBAD"},
	}}

	// OpenNGC's row for NGC 1990, with the identifiers its CSV lists.
	openngc := &mockBrightSource{targets: []resolve.Target{
		{ID: "NGC1990", Name: "Alnilam", Kind: resolve.KindStar, Coord: at(84.053417, -1.201917), HasCoord: true, VMag: 1.69, HasVMag: true, Catalog: "openngc",
			Aliases: []string{"NGC 1990", "HD 037128", "HIP 026311", "Eps Ori", "46 Ori"}},
	}}

	results, err := plan.VisibleTonight(context.Background(), quintaCalixtoSite(t), testNight, 2,
		[]resolve.BrightObjectSearcher{simbad, openngc}, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	count := func(name string) int {
		n := 0

		for _, r := range results {
			if r.Target.Name == name {
				n++
			}
		}

		return n
	}

	if n := count("Alnilam"); n != 1 {
		t.Errorf("Alnilam listed %d times, want once", n)
	}

	if a, ok := findByName(results, "Alnilam"); ok {
		if a.Target.ID != "* eps Ori" {
			t.Errorf("kept %q, want the first source's entry, * eps Ori", a.Target.ID)
		}

		if !containsString(a.Target.Aliases, "NGC1990") || !containsString(a.Target.Aliases, "HIP 026311") {
			t.Errorf("the merged Alnilam lost OpenNGC's identifiers: %q", a.Target.Aliases)
		}
	}

	for _, name := range []string{"Rigil Kentaurus", "Rigil Kentaurus A"} {
		if n := count(name); n != 1 {
			t.Errorf("%s listed %d times, want once: a system and its component are two entries", name, n)
		}
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}

	return false
}
