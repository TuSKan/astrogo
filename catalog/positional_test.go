package catalog

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/internal/posmatch"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/catalog/simbad"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

// mas is v milliarcseconds.
func mas(v float64) angle.Angle { return angle.Arcsec(v / 1000) }

// at is a still position at (raDeg, decDeg + northArcsec), at J2000, with
// the errors e.
func at(id, name string, raDeg, decDeg, northArcsec float64, e resolve.AstrometricError) Target {
	return Target{
		ID: id, Name: name, HasCoord: true, Epoch: epoch.J2000(), AstrometricError: e,
		Coord: coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg).Add(angle.Arcsec(northArcsec))),
	}
}

// sigma is a position error of v arcseconds on each axis.
func sigma(v float64) resolve.AstrometricError {
	return resolve.AstrometricError{RA: angle.Arcsec(v), Dec: angle.Arcsec(v)}
}

// resolverOf is a Resolver over providers and cone searchers, at
// resolution.
func resolverOf(resolution angle.Angle, providers []resolve.Provider, cones ...coneProvider) *Resolver {
	return &Resolver{
		providers: providers, coneSearchers: cones,
		cfg: resolverConfig{resolution: resolution, cap: defaultCap},
	}
}

// mockIDResolver is a resolve.IdentifierResolver over a fixed set of
// records, answering with those whose identity keys an asked identifier
// names, and recording what it was asked.
type mockIDResolver struct {
	targets []Target
	asked   []resolve.Identifier
}

func (m *mockIDResolver) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapIdentifierResolution}
}

func (m *mockIDResolver) ResolveIdentifiers(_ context.Context, ids []resolve.Identifier) resolve.SeqIterator[Target] {
	m.asked = append(m.asked, ids...)

	var found []Target

	for _, t := range m.targets {
		for _, id := range resolve.Identifiers(t) {
			if slices.Contains(ids, id) {
				found = append(found, t)

				break
			}
		}
	}

	return resolve.SliceSeq(found)
}

// countingConeSearcher is a mockConeSearcher that counts its searches.
type countingConeSearcher struct {
	mockConeSearcher

	searches atomic.Int32
}

func (c *countingConeSearcher) ConeSearch(ctx context.Context, req resolve.ConeRequest) resolve.SeqIterator[Target] {
	c.searches.Add(1)

	return c.mockConeSearcher.ConeSearch(ctx, req)
}

// statuses counts t's associations by status.
func statuses(t Target) map[resolve.AssociationStatus]int {
	n := make(map[resolve.AssociationStatus]int)
	for _, a := range t.Associations {
		n[a.Status]++
	}

	return n
}

// TestResolver_PositionalMatchWeighsPublishedErrors: two records that both
// publish their position errors are matched by D², with no resolution set.
// Two 0.1″ apart with 1 mas errors are two objects (D² = 5000), and two 3″
// apart where one is known to 1″ are one (D² = 9).
func TestResolver_PositionalMatchWeighsPublishedErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		north, sigma float64
		want         int
		d2           float64
	}{
		{"0.1″ apart, 1 mas errors", 0.1, 0.001, 2, 0},
		{"3″ apart, 1″ errors", 3, 1, 1, 9 / (1 + 1e-6)},
	} {
		simbad := &mockProvider{name: "simbad", targets: map[string]Target{
			"s": at("S1", "Obj", 83.822, -5.391, 0, sigma(0.001)),
		}}
		vizier := &mockProvider{name: "vizier", targets: map[string]Target{
			"v": at("V1", "Obj", 83.822, -5.391, tc.north, sigma(tc.sigma)),
		}}

		got, err := resolverOf(0, []resolve.Provider{simbad, vizier}).Search(context.Background(), "Obj")
		testutil.AssertNoError(t, err)

		if len(got) != tc.want {
			t.Fatalf("%s: %d targets, want %d", tc.name, len(got), tc.want)
		}

		if tc.want != 1 {
			continue
		}

		if len(got[0].Associations) != 1 || !got[0].Associations[0].HasD2 {
			t.Fatalf("%s: associations %+v, want one with D²", tc.name, got[0].Associations)
		}

		testutil.AssertRelNear(t, tc.name+": D²", got[0].Associations[0].D2, tc.d2, 1e-6)
	}
}

// TestResolver_PositionalMatchKeepsTwoRecordsOfOneProviderApart: a record
// within a 2″ resolution of two records of one provider is matched with
// neither, and each of the three keeps an ambiguous association to what it
// was kept apart from.
func TestResolver_PositionalMatchKeepsTwoRecordsOfOneProviderApart(t *testing.T) {
	t.Parallel()

	simbad := &mockProvider{name: "simbad", targets: map[string]Target{
		"s": at("S1", "Pair", 50, 10, 0, resolve.AstrometricError{}),
	}}
	vizier := &mockProvider{name: "vizier", targets: map[string]Target{
		"a": at("VA", "Pair", 50, 10, 0.5, resolve.AstrometricError{}),
		"b": at("VB", "Pair", 50, 10, -0.5, resolve.AstrometricError{}),
	}}

	got, err := resolverOf(angle.Arcsec(2), []resolve.Provider{simbad, vizier}).Search(context.Background(), "Pair")
	testutil.AssertNoError(t, err)

	if len(got) != 3 {
		t.Fatalf("%d targets, want the three records apart: %+v", len(got), got)
	}

	for _, tgt := range got {
		want := 1
		if tgt.ID == "S1" {
			want = 2
		}

		if n := statuses(tgt); n[resolve.Ambiguous] != want || n[resolve.Accepted] != 0 {
			t.Errorf("%s: associations by status %v, want %d ambiguous", tgt.ID, n, want)
		}
	}
}

// TestResolver_ConeSearchBridgeKeepsABinaryApart: two Gaia rows within a 2″
// resolution of a SIMBAD star, a close binary's two components, are not both
// that star. Neither is folded in, and both are kept as ambiguous
// associations.
func TestResolver_ConeSearchBridgeKeepsABinaryApart(t *testing.T) {
	t.Parallel()

	simbad := &mockProvider{name: "simbad", targets: map[string]Target{
		"star": at("S1", "Star", 50, 10, 0, resolve.AstrometricError{}),
	}}
	gaia := &mockConeSearcher{targets: []Target{
		at("G1", "", 50, 10, 0.5, resolve.AstrometricError{}),
		at("G2", "", 50, 10, -0.5, resolve.AstrometricError{}),
	}}

	got, err := resolverOf(angle.Arcsec(2), []resolve.Provider{simbad}, coneProvider{name: "gaia", cs: gaia}).Resolve(context.Background(), "star")
	testutil.AssertNoError(t, err)

	if got.Provenance["Coord"] == "gaia" || len(got.Sources) != 1 {
		t.Errorf("a Gaia row was folded in: Coord from %q, sources %v", got.Provenance["Coord"], got.Sources)
	}

	if n := statuses(got); n[resolve.Ambiguous] != 2 {
		t.Errorf("associations by status %v, want 2 ambiguous", n)
	}
}

// cyg61Simbad and cyg61Gaia are 61 Cyg A as SIMBAD and Gaia DR3 publish it,
// read 2026-11-18. cyg61Simbad reads SIMBAD's row at face value: Gaia's
// position moved to J2000.0 without a radial velocity, with Gaia's errors at
// J2016.0. catalog/simbad no longer reports it so (#1248), which
// TestResolver_ASimbadStarTakenFromGaiaMatchesItsGaiaRowByPosition holds, but
// the pair stays two rows of one star that disagree by position. The Gaia row
// carries its DR3 radial velocity, a column catalog/gaia does not read.
var (
	cyg61Simbad = Target{
		ID: "*  61 Cyg A", Name: "61 Cyg A", Aliases: []string{"Gaia DR3 1872046609345556480"},
		Coord:    coord.NewICRS(angle.Deg(316.7247482895925), angle.Deg(38.74941731943694)),
		HasCoord: true, Epoch: epoch.J2000(),
		PmRA: mas(4164.209), PmDec: mas(3249.614), Parallax: mas(285.9949),
		RadialVelocity: unit.KmPerSec(-65.82), HasRadialVelocity: true,
		AstrometricError: resolve.AstrometricError{
			RA: mas(0.0388), Dec: mas(0.0485), PmRA: mas(0.055), PmDec: mas(0.055),
			Epoch: epoch.FromJD(2457389.0, epoch.TT),
		},
	}
	cyg61Gaia = Target{
		ID: "1872046609345556480", Name: "Gaia DR3 1872046609345556480", Designation: "Gaia DR3 1872046609345556480",
		Coord:    coord.NewICRS(angle.Deg(316.7484792940004), angle.Deg(38.76386244649797)),
		HasCoord: true, Epoch: epoch.FromJD(2457389.0, epoch.TT),
		PmRA: mas(4164.2086922846665), PmDec: mas(3249.613883848584), Parallax: mas(285.99494829578117),
		RadialVelocity: unit.KmPerSec(-65.97495), HasRadialVelocity: true,
		AstrometricError: resolve.AstrometricError{
			RA: mas(0.03876041), Dec: mas(0.048496146), PmRA: mas(0.055289943), PmDec: mas(0.055022996),
			RADec: -0.07811369, RAPmRA: 0.12697393, RAPmDec: -0.07891878,
			DecPmRA: -0.18858151, DecPmDec: -0.33916456, PmRAPmDec: 0.0030485608,
		},
	}
)

// TestResolver_BridgeFindsTheGaiaRowByItsIdentifier: SIMBAD lists 61 Cyg
// A's Gaia DR3 designation, so with no resolution set the Resolver asks Gaia
// for that source and makes no cone search. Moved to J2000.0 with its radial
// velocity, the row lands 26 mas from SIMBAD's position read at face value,
// which SIMBAD moved there without one, and is incompatible with it by D²
// against errors under 1 mas; it is folded in by the identifier.
func TestResolver_BridgeFindsTheGaiaRowByItsIdentifier(t *testing.T) {
	t.Parallel()

	ev, ok, err := posmatch.Compare(cyg61Simbad, cyg61Gaia, 0)
	testutil.AssertNoError(t, err)

	if !ev.HasD2 || ok {
		t.Fatalf("test setup: by position, D² %g (computed %v), compatible %v; want incompatible", ev.D2, ev.HasD2, ok)
	}

	simbad := &mockProvider{name: "simbad", targets: map[string]Target{"star": cyg61Simbad}}
	ids := &mockIDResolver{targets: []Target{cyg61Gaia}}
	cone := &countingConeSearcher{targets: []Target{cyg61Gaia}}

	r := resolverOf(0, []resolve.Provider{simbad}, coneProvider{name: "gaia", cs: cone})
	r.idResolvers = []idProvider{{name: "gaia", ir: ids}}

	got, err := r.Resolve(context.Background(), "star")
	testutil.AssertNoError(t, err)

	want := resolve.Identifier{Namespace: "gaia", Release: "DR3", Value: "1872046609345556480"}
	if !slices.Contains(ids.asked, want) {
		t.Errorf("Gaia was asked for %+v, not for SIMBAD's %+v", ids.asked, want)
	}

	if n := cone.searches.Load(); n != 0 {
		t.Errorf("%d cone searches with no resolution set, want none", n)
	}

	if got.Provenance["Coord"] != "gaia" {
		t.Errorf("Coord from %q, want the folded Gaia row's", got.Provenance["Coord"])
	}

	if len(got.Associations) != 1 || got.Associations[0].Evidence != want || got.Associations[0].Status != resolve.Accepted {
		t.Errorf("associations %+v, want one accepted on %+v", got.Associations, want)
	}

	if got.Provenance["AstrometricError"] != "gaia" || got.AstrometricError != cyg61Gaia.AstrometricError {
		t.Errorf("errors from %q: %+v, want the Gaia row's, with its position", got.Provenance["AstrometricError"], got.AstrometricError)
	}
}

// TestResolver_ASimbadStarTakenFromGaiaMatchesItsGaiaRowByPosition is
// #1248 where the Resolver compares rows: 61 Cyg A as catalog/simbad reads
// SIMBAD's row is at Gaia's epoch, and is compatible with its Gaia DR3 row
// by D², which it was not while read at J2000.0.
func TestResolver_ASimbadStarTakenFromGaiaMatchesItsGaiaRowByPosition(t *testing.T) {
	t.Parallel()

	rows, err := simbad.ParseCSV(strings.NewReader("main_id,ra,dec,pmra,pmdec,plx_value,rvz_radvel," +
		"coo_err_maj,coo_err_min,coo_err_angle,coo_bibcode,pm_err_maj,pm_err_min,pm_err_angle\n" +
		"*  61 Cyg A,316.7247482895925,38.74941731943694,4164.209,3249.614,285.9949,-65.82," +
		"0.0388,0.0485,90,2020yCat.1350....0G,0.055,0.055,90\n"))
	testutil.AssertNoError(t, err)

	if len(rows) != 1 {
		t.Fatalf("%d rows, want 1", len(rows))
	}

	ev, ok, err := posmatch.Compare(rows[0], cyg61Gaia, 0)
	testutil.AssertNoError(t, err)

	if !ev.HasD2 || !ok {
		t.Errorf("SIMBAD's 61 Cyg A against its Gaia row: D² %g (computed %v), compatible %v; want compatible",
			ev.D2, ev.HasD2, ok)
	}

	t.Logf("D² %.4f, %.4f mas apart", ev.D2, ev.Separation.Arcsec()*1000)
}

// TestResolver_BridgeFindsTheGaiaRowByItsHipparcosNumber: a SIMBAD star that
// lists its Hipparcos number but no Gaia designation is asked of Gaia by that
// number, and Gaia's row, which carries the number from Gaia's own
// cross-match, joins it by that identifier.
func TestResolver_BridgeFindsTheGaiaRowByItsHipparcosNumber(t *testing.T) {
	t.Parallel()

	star := at("NAME Barnard's star", "Barnard's star", 269.452, 4.693, 0, resolve.AstrometricError{})
	star.Aliases = []string{"HIP 87937"}

	row := at("4472832130942575872", "Gaia DR3 4472832130942575872", 269.4485, 4.7394, 0, resolve.AstrometricError{})
	row.Designation = row.Name
	row.Aliases = []string{"HIP 87937", "TYC 425-2502-1"}

	ids := &mockIDResolver{targets: []Target{row}}
	r := resolverOf(0, []resolve.Provider{&mockProvider{name: "simbad", targets: map[string]Target{"star": star}}})
	r.idResolvers = []idProvider{{name: "gaia", ir: ids}}

	got, err := r.Resolve(context.Background(), "star")
	testutil.AssertNoError(t, err)

	want := resolve.Identifier{Namespace: "hip", Value: "87937"}
	if !slices.Contains(ids.asked, want) {
		t.Errorf("Gaia was asked for %+v, not for %+v", ids.asked, want)
	}

	if got.Provenance["Coord"] != "gaia" || len(got.Associations) != 1 || got.Associations[0].Evidence != want {
		t.Errorf("Coord from %q, associations %+v; want Gaia's row joined on %+v", got.Provenance["Coord"], got.Associations, want)
	}
}
