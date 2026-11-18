package catalog

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/internal/posmatch"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// mas is v milliarcseconds.
func mas(v float64) angle.Angle { return angle.Arcsec(v / 1000) }

// at is a still position at (raDeg, decDeg + northArcsec), at J2000, with
// the errors e.
func at(id, name string, raDeg, decDeg, northArcsec float64, e resolve.AstrometricError) Target {
	return Target{
		ID: id, Name: name, HasCoord: true, Epoch: time.J2000(), AstrometricError: e,
		Coord: coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg).Add(angle.Arcsec(northArcsec))),
	}
}

// sigma is a position error of v arcseconds on each axis.
func sigma(v float64) resolve.AstrometricError {
	return resolve.AstrometricError{RA: angle.Arcsec(v), Dec: angle.Arcsec(v)}
}

// resolverOf is a Resolver over providers and cone searchers, at the default
// threshold.
func resolverOf(providers []resolve.Provider, cones ...coneProvider) *Resolver {
	return &Resolver{
		providers: providers, coneSearchers: cones,
		cfg: resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}
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
// publish their position errors are matched by D², not by the 2″ threshold.
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

		got, err := resolverOf([]resolve.Provider{simbad, vizier}).Search(context.Background(), "Obj")
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
// within reach of two records of one provider is matched with neither, and
// each of the three keeps an ambiguous association to what it was kept apart
// from.
func TestResolver_PositionalMatchKeepsTwoRecordsOfOneProviderApart(t *testing.T) {
	t.Parallel()

	simbad := &mockProvider{name: "simbad", targets: map[string]Target{
		"s": at("S1", "Pair", 50, 10, 0, resolve.AstrometricError{}),
	}}
	vizier := &mockProvider{name: "vizier", targets: map[string]Target{
		"a": at("VA", "Pair", 50, 10, 0.5, resolve.AstrometricError{}),
		"b": at("VB", "Pair", 50, 10, -0.5, resolve.AstrometricError{}),
	}}

	got, err := resolverOf([]resolve.Provider{simbad, vizier}).Search(context.Background(), "Pair")
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

// TestResolver_ConeSearchBridgeKeepsABinaryApart: two Gaia rows within 2″ of
// a SIMBAD star, a close binary's two components, are not both that star.
// Neither is folded in, and both are kept as ambiguous associations.
func TestResolver_ConeSearchBridgeKeepsABinaryApart(t *testing.T) {
	t.Parallel()

	simbad := &mockProvider{name: "simbad", targets: map[string]Target{
		"star": at("S1", "Star", 50, 10, 0, resolve.AstrometricError{}),
	}}
	gaia := &mockConeSearcher{targets: []Target{
		at("G1", "", 50, 10, 0.5, resolve.AstrometricError{}),
		at("G2", "", 50, 10, -0.5, resolve.AstrometricError{}),
	}}

	got, err := resolverOf([]resolve.Provider{simbad}, coneProvider{name: "gaia", cs: gaia}).Resolve(context.Background(), "star")
	testutil.AssertNoError(t, err)

	if got.Provenance["Coord"] == "gaia" || len(got.Sources) != 1 {
		t.Errorf("a Gaia row was folded in: Coord from %q, sources %v", got.Provenance["Coord"], got.Sources)
	}

	if n := statuses(got); n[resolve.Ambiguous] != 2 {
		t.Errorf("associations by status %v, want 2 ambiguous", n)
	}
}

// cyg61Simbad and cyg61Gaia are 61 Cyg A as SIMBAD and Gaia DR3 publish it,
// read 2026-11-18. SIMBAD's position is Gaia's moved to J2000.0 without a
// radial velocity, with Gaia's errors at J2016.0. The Gaia row carries its
// DR3 radial velocity, a column catalog/gaia does not read.
var (
	cyg61Simbad = Target{
		ID: "*  61 Cyg A", Name: "61 Cyg A", Aliases: []string{"Gaia DR3 1872046609345556480"},
		Coord:    coord.NewICRS(angle.Deg(316.7247482895925), angle.Deg(38.74941731943694)),
		HasCoord: true, Epoch: time.J2000(),
		PmRA: mas(4164.209), PmDec: mas(3249.614), Parallax: mas(285.9949),
		RadialVelocity: unit.KmPerSec(-65.82), HasRadialVelocity: true,
		AstrometricError: resolve.AstrometricError{
			RA: mas(0.0388), Dec: mas(0.0485), PmRA: mas(0.055), PmDec: mas(0.055),
			Epoch: time.FromJD(2457389.0, time.TT),
		},
	}
	cyg61Gaia = Target{
		ID: "1872046609345556480", Name: "Gaia DR3 1872046609345556480", Designation: "Gaia DR3 1872046609345556480",
		Coord:    coord.NewICRS(angle.Deg(316.7484792940004), angle.Deg(38.76386244649797)),
		HasCoord: true, Epoch: time.FromJD(2457389.0, time.TT),
		PmRA: mas(4164.2086922846665), PmDec: mas(3249.613883848584), Parallax: mas(285.99494829578117),
		RadialVelocity: unit.KmPerSec(-65.97495), HasRadialVelocity: true,
		AstrometricError: resolve.AstrometricError{
			RA: mas(0.03876041), Dec: mas(0.048496146), PmRA: mas(0.055289943), PmDec: mas(0.055022996),
			RADec: -0.07811369, RAPmRA: 0.12697393, RAPmDec: -0.07891878,
			DecPmRA: -0.18858151, DecPmDec: -0.33916456, PmRAPmDec: 0.0030485608,
		},
	}
)

// TestResolver_ConeSearchBridgeFoldsByIdentifierFirst: moved to J2000.0
// with its radial velocity, 61 Cyg A's Gaia row lands 26 mas from SIMBAD's
// position, which SIMBAD moved there without one, and is incompatible with it
// by D² against errors under 1 mas. SIMBAD lists the row's designation, so
// the row is folded in by that identifier.
func TestResolver_ConeSearchBridgeFoldsByIdentifierFirst(t *testing.T) {
	t.Parallel()

	ev, ok, err := posmatch.Compare(cyg61Simbad, cyg61Gaia, defaultPositionMatchThreshold)
	testutil.AssertNoError(t, err)

	if !ev.HasD2 || ok {
		t.Fatalf("test setup: by position, D² %g (computed %v), compatible %v; want incompatible", ev.D2, ev.HasD2, ok)
	}

	simbad := &mockProvider{name: "simbad", targets: map[string]Target{"star": cyg61Simbad}}
	gaia := &mockConeSearcher{targets: []Target{cyg61Gaia}}

	got, err := resolverOf([]resolve.Provider{simbad}, coneProvider{name: "gaia", cs: gaia}).Resolve(context.Background(), "star")
	testutil.AssertNoError(t, err)

	if got.Provenance["Coord"] != "gaia" {
		t.Errorf("Coord from %q, want the folded Gaia row's", got.Provenance["Coord"])
	}

	want := resolve.Identifier{Namespace: "gaia", Release: "DR3", Value: "1872046609345556480"}
	if len(got.Associations) != 1 || got.Associations[0].Evidence != want || got.Associations[0].Status != resolve.Accepted {
		t.Errorf("associations %+v, want one accepted on %+v", got.Associations, want)
	}

	if got.Provenance["AstrometricError"] != "gaia" || got.AstrometricError != cyg61Gaia.AstrometricError {
		t.Errorf("errors from %q: %+v, want the Gaia row's, with its position", got.Provenance["AstrometricError"], got.AstrometricError)
	}
}
