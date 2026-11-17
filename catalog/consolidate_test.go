package catalog_test

import (
	"slices"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

func icrs(raDeg, decDeg float64) coord.ICRS {
	return coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg))
}

// SIMBAD's star eps Ori as its bright query returns it with its identifiers,
// SIMBAD's nebula NGC 1990, and OpenNGC's NGC 1990, which OpenNGC catalogs as
// the star (identifiers as each service lists them, 2026-11-16).
func simbadAlnilam() catalog.Record {
	return catalog.Record{Provider: "simbad", Target: catalog.Target{
		ID: "* eps Ori", Name: "Alnilam", Catalog: "SIMBAD",
		Coord: icrs(84.05338894, -1.20191914), HasCoord: true, Epoch: time.J2000(),
		PmRA: angle.Arcsec(0.00144), PmDec: angle.Arcsec(-0.00078), Parallax: angle.Arcsec(0.00165),
		VMag: 1.69, HasVMag: true, RadialVelocity: unit.KmPerSec(25.9), HasRadialVelocity: true,
		Aliases: []string{"* eps Ori", "HD  37128", "HIP 26311", "BD-01   969", "NAME Alnilam", "V* eps Ori"},
	}}
}

func simbadNebula() catalog.Record {
	return catalog.Record{Provider: "simbad", Target: catalog.Target{
		ID: "NGC  1990", Name: "NGC 1990", Catalog: "SIMBAD",
		Coord: icrs(84.0, -1.2), HasCoord: true,
		Aliases: []string{"LBN   940", "LBN 205.42-17.78", "NGC  1990"},
	}}
}

func openngcNGC1990() catalog.Record {
	return catalog.Record{Provider: "openngc", Target: catalog.Target{
		ID: "NGC1990", Name: "Alnilam", Catalog: "openngc",
		Coord: icrs(84.053417, -1.201917), HasCoord: true,
		VMag: 1.7, HasVMag: true,
		Aliases: []string{"Alnilam", "BD -01 0969", "HD 037128", "HIP 026311", "Eps Ori", "46 Ori"},
	}}
}

// TestConsolidateMergesByIdentity: OpenNGC's NGC 1990 and SIMBAD's eps Ori
// share their designations, so they are one target, while an object sharing
// none stays its own (#1136).
func TestConsolidateMergesByIdentity(t *testing.T) {
	t.Parallel()

	vega := catalog.Record{Provider: "simbad", Target: catalog.Target{ID: "* alf Lyr", Name: "Vega", Aliases: []string{"HD 172167"}}}

	got := catalog.Consolidate([]catalog.Record{simbadAlnilam(), vega, openngcNGC1990()})
	if len(got) != 2 {
		t.Fatalf("got %d targets, want Alnilam and Vega: %q", len(got), ids(got))
	}

	if got[0].ID != "* eps Ori" || got[1].ID != "* alf Lyr" {
		t.Errorf("targets %q, want the first record of each object, in order", ids(got))
	}

	for _, id := range []string{"NGC1990", "HD 037128", "Eps Ori", "HD  37128"} {
		if !slices.Contains(got[0].Aliases, id) {
			t.Errorf("merged Alnilam lost the identifier %q: %q", id, got[0].Aliases)
		}
	}
}

// TestConsolidateSelectsEachMeasurement: the merge takes each field from the
// provider the policy ranks for it, not every field from the first record.
// With OpenNGC's record first, the identifying fields are OpenNGC's, the
// position comes from SIMBAD with its epoch, parallax and proper motion, the
// V magnitude from SIMBAD, and SIMBAD's radial velocity, which OpenNGC has
// none of, is kept.
func TestConsolidateSelectsEachMeasurement(t *testing.T) {
	t.Parallel()

	simbad := simbadAlnilam()

	got := catalog.Consolidate([]catalog.Record{openngcNGC1990(), simbad})
	if len(got) != 1 {
		t.Fatalf("got %d targets, want one: %q", len(got), ids(got))
	}

	m := got[0]

	if m.ID != "NGC1990" || m.Provenance["ID"] != "openngc" {
		t.Errorf("ID %q from %q, want OpenNGC's, the first record's", m.ID, m.Provenance["ID"])
	}

	if m.Coord != simbad.Target.Coord || m.PmRA != simbad.Target.PmRA || m.Parallax != simbad.Target.Parallax ||
		!m.Epoch.Equal(simbad.Target.Epoch) {
		t.Errorf("astrometry %v pm %v plx %v epoch %v, want SIMBAD's solution whole", m.Coord, m.PmRA, m.Parallax, m.Epoch)
	}

	for _, field := range []string{"Coord", "PmRA", "PmDec", "Parallax", "Epoch", "VMag", "RadialVelocity"} {
		if m.Provenance[field] != "simbad" {
			t.Errorf("%s from %q, want simbad", field, m.Provenance[field])
		}
	}

	if m.VMag != 1.69 || !m.HasRadialVelocity || m.RadialVelocity != simbad.Target.RadialVelocity {
		t.Errorf("VMag %v, radial velocity %v (%v), want SIMBAD's", m.VMag, m.RadialVelocity, m.HasRadialVelocity)
	}
}

// TestConsolidateKeepsDisplayNamesOutOfIdentity: a record's display name is
// not made an alias, since an alias is an identity key. Were it, a label the
// first merge rejected as evidence would join objects on the next: here a
// third object whose ID is the discarded record's display name.
func TestConsolidateKeepsDisplayNamesOutOfIdentity(t *testing.T) {
	t.Parallel()

	first := catalog.Consolidate([]catalog.Record{
		{Provider: "simbad", Target: catalog.Target{ID: "HD 1", Name: "Kept"}},
		{Provider: "openngc", Target: catalog.Target{ID: "X 2", Name: "Label", Aliases: []string{"HD 1"}}},
	})
	if len(first) != 1 {
		t.Fatalf("got %d targets, want one: %q", len(first), ids(first))
	}

	if first[0].Name != "Kept" || slices.Contains(first[0].Aliases, "Label") {
		t.Errorf("merged name %q, aliases %q: want the first record's name, and no display name among the aliases",
			first[0].Name, first[0].Aliases)
	}

	second := catalog.Consolidate([]catalog.Record{
		{Provider: "merged", Target: first[0]},
		{Provider: "other", Target: catalog.Target{ID: "Label"}},
	})
	if len(second) != 2 {
		t.Errorf("a display name joined two objects on a second pass: %q", ids(second))
	}
}

// TestConsolidateKeepsAnAmbiguity: OpenNGC's NGC 1990 names both SIMBAD's
// star and SIMBAD's nebula, two objects SIMBAD keeps apart. That is an
// ambiguity, not an identity, so nothing merges and each record is returned
// as it came: one object too many rather than two made one.
func TestConsolidateKeepsAnAmbiguity(t *testing.T) {
	t.Parallel()

	in := []catalog.Record{simbadAlnilam(), simbadNebula(), openngcNGC1990()}

	got := catalog.Consolidate(in)
	if len(got) != 3 {
		t.Fatalf("got %d targets, want all three records unmerged: %q", len(got), ids(got))
	}

	for i, r := range in {
		if got[i].ID != r.Target.ID || got[i].Provenance != nil || !slices.Equal(got[i].Aliases, r.Target.Aliases) {
			t.Errorf("record %d came back changed: %+v", i, got[i])
		}
	}

	// Two records from one provider that share a designation themselves are
	// a duplicate listing, not an ambiguity.
	dup := openngcNGC1990()
	dup.Target.ID, dup.Target.Aliases = "IC 9999", []string{"NGC 1990"}

	if got := catalog.Consolidate([]catalog.Record{simbadAlnilam(), openngcNGC1990(), dup}); len(got) != 1 {
		t.Errorf("a provider's duplicate rows and SIMBAD's star gave %d targets, want one: %q", len(got), ids(got))
	}
}

// TestConsolidateRanksUnknownProviders: a provider the policy does not rank
// still supplies a field no ranked provider has, after them. A user's own
// catalog holding the only position must not lose it to the merge.
func TestConsolidateRanksUnknownProviders(t *testing.T) {
	t.Parallel()

	got := catalog.Consolidate([]catalog.Record{
		{Provider: "simbad", Target: catalog.Target{ID: "HD 1", Name: "Star"}},
		{Provider: "mycatalog", Target: catalog.Target{ID: "M-1", Aliases: []string{"HD 1"},
			Coord: icrs(10, 20), HasCoord: true, VMag: 5, HasVMag: true}},
	})
	if len(got) != 1 {
		t.Fatalf("got %d targets, want one: %q", len(got), ids(got))
	}

	if !got[0].HasCoord || got[0].Provenance["Coord"] != "mycatalog" || !got[0].HasVMag || got[0].Provenance["VMag"] != "mycatalog" {
		t.Errorf("merged %+v: want mycatalog's position and magnitude, the only ones", got[0])
	}

	// A ranked provider still comes first.
	ranked := catalog.Consolidate([]catalog.Record{
		{Provider: "mycatalog", Target: catalog.Target{ID: "M-1", Aliases: []string{"HD 1"}, Coord: icrs(10, 20), HasCoord: true}},
		{Provider: "simbad", Target: catalog.Target{ID: "HD 1", Coord: icrs(10.001, 20), HasCoord: true}},
	})
	if len(ranked) != 1 || ranked[0].Provenance["Coord"] != "simbad" {
		t.Errorf("merged %+v: want SIMBAD's position over an unranked provider's", ranked)
	}
}

// TestConsolidateReturnsALoneRecordAsItCame: a record matching no other is
// not run through the merge, which would rebuild it field by field.
func TestConsolidateReturnsALoneRecordAsItCame(t *testing.T) {
	t.Parallel()

	in := simbadAlnilam()

	got := catalog.Consolidate([]catalog.Record{in})
	if len(got) != 1 || got[0].Provenance != nil || !slices.Equal(got[0].Aliases, in.Target.Aliases) || got[0].Name != in.Target.Name {
		t.Errorf("got %+v, want the record unchanged", got)
	}

	if got := catalog.Consolidate(nil); len(got) != 0 {
		t.Errorf("Consolidate(nil) = %q, want none", ids(got))
	}
}

func ids(ts []catalog.Target) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}

	return out
}

// TestConsolidateUnionsMemberships: what a merged object is a member of is
// every record's, as its identifiers are, each named once in its own
// spelling; each catalog records the hierarchy it knows (#1182).
func TestConsolidateUnionsMemberships(t *testing.T) {
	t.Parallel()

	got := catalog.Consolidate([]catalog.Record{
		{Provider: "simbad", Target: catalog.Target{ID: "* alf Cen A", MemberOf: []string{"* alf Cen"}}},
		{Provider: "other", Target: catalog.Target{
			ID: "HD 128620", Aliases: []string{"* alf Cen A"},
			MemberOf: []string{"* alf Cen", " WDS J14396-6050AB ", ""},
		}},
	})
	if len(got) != 1 {
		t.Fatalf("got %d targets, want one: the two records share * alf Cen A", len(got))
	}

	if want := []string{"* alf Cen", "WDS J14396-6050AB"}; !slices.Equal(got[0].MemberOf, want) {
		t.Errorf("MemberOf = %q, want %q", got[0].MemberOf, want)
	}
}
