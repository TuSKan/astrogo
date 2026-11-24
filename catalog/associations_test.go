package catalog

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
)

// TestConsolidateRecordsWhyRecordsAreOne: a merged Target names every record
// it was built from and the identifier that joined them, with the provider
// that asserted the cross-identification as the authority, whichever record
// comes first.
func TestConsolidateRecordsWhyRecordsAreOne(t *testing.T) {
	t.Parallel()

	simbad := Record{Provider: "simbad", Target: Target{ID: "M  44", Aliases: []string{"NGC 2632"}, Catalog: "SIMBAD"}}
	openngc := Record{Provider: "openngc", Target: Target{ID: "NGC 2632", Catalog: "openngc"}}

	for _, order := range [][]Record{{simbad, openngc}, {openngc, simbad}} {
		got := Consolidate(order)
		if len(got) != 1 {
			t.Fatalf("Consolidate gave %d targets, want one", len(got))
		}

		m := got[0]

		if len(m.Sources) != 2 {
			t.Fatalf("Sources = %+v, want both records", m.Sources)
		}

		for i, r := range order {
			if want := (resolve.SourceRecord{Provider: r.Provider, ID: r.Target.ID}); m.Sources[i] != want {
				t.Errorf("Sources[%d] = %+v, want %+v", i, m.Sources[i], want)
			}
		}

		if len(m.Associations) != 1 {
			t.Fatalf("Associations = %+v, want the one join", m.Associations)
		}

		a := m.Associations[0]

		if a.Relationship != resolve.SameObject || a.Status != resolve.Accepted {
			t.Errorf("association is %v, %v; want same object, accepted", a.Relationship, a.Status)
		}

		if want := (resolve.Identifier{Namespace: "ngc", Value: "2632"}); a.Evidence != want {
			t.Errorf("Evidence = %+v, want %+v", a.Evidence, want)
		}

		if a.Authority != "simbad" {
			t.Errorf("Authority = %q, want simbad, whose record lists NGC 2632 beside its own ID", a.Authority)
		}

		if a.Separation != 0 {
			t.Errorf("Separation = %v for an identifier's association", a.Separation)
		}
	}
}

// TestConsolidateRecordsAnAmbiguity: a group left apart keeps its records
// as they came, each naming the records it was kept apart from, as an
// ambiguous association with the identifier that would have joined them.
func TestConsolidateRecordsAnAmbiguity(t *testing.T) {
	t.Parallel()

	nebula := Record{Provider: "simbad", Target: Target{ID: "NGC  1990", Catalog: "SIMBAD"}}
	star := Record{Provider: "simbad", Target: Target{ID: "* eps Ori", Catalog: "SIMBAD"}}
	alnilam := Record{Provider: "openngc", Target: Target{ID: "NGC 1990", Aliases: []string{"Eps Ori"}, Catalog: "openngc"}}

	got := Consolidate([]Record{nebula, star, alnilam})
	if len(got) != 3 {
		t.Fatalf("Consolidate gave %d targets, want the three records apart", len(got))
	}

	for i, want := range []int{1, 1, 2} {
		if n := len(got[i].Associations); n != want {
			t.Errorf("target %d (%s) has %d associations, want %d", i, got[i].ID, n, want)
		}

		if len(got[i].Sources) != 1 {
			t.Errorf("target %d has Sources %+v, want its own record", i, got[i].Sources)
		}

		for _, a := range got[i].Associations {
			if a.Status != resolve.Ambiguous {
				t.Errorf("target %d: association %+v is not ambiguous", i, a)
			}

			if a.Evidence == (resolve.Identifier{}) {
				t.Errorf("target %d: an ambiguous association states no identifier", i)
			}
		}
	}
}

// TestSourceRecordCarriesTheRelease: a provider's data release, named in its
// Catalog, is the source record's Release.
func TestSourceRecordCarriesTheRelease(t *testing.T) {
	t.Parallel()

	got := sourceOf(candidate{target: Target{ID: "Gaia DR3 4472832130942575872", Catalog: "Gaia DR3"}, provider: "gaia"})
	if want := (resolve.SourceRecord{Provider: "gaia", Release: "DR3", ID: "Gaia DR3 4472832130942575872"}); got != want {
		t.Errorf("sourceOf = %+v, want %+v", got, want)
	}

	if got := sourceOf(candidate{target: Target{ID: "M 31", Catalog: "SIMBAD"}, provider: "simbad"}); got.Release != "" {
		t.Errorf("a living catalog's Release = %q, want empty", got.Release)
	}
}

// TestResolverRecordsAPositionalAssociation: two records no identifier
// joins, merged because their positions agree, carry an association that
// states the separation and astrogo as its authority.
func TestResolverRecordsAPositionalAssociation(t *testing.T) {
	t.Parallel()

	at := coord.NewICRS(angle.Deg(101.287155), angle.Deg(-16.716116))
	near := coord.NewICRS(angle.Deg(101.287155), angle.Deg(-16.716116+angle.Arcsec(0.5).Degrees()))

	p1 := &mockProvider{name: "simbad", targets: map[string]Target{
		"target": {ID: "* alf CMa", Coord: at, HasCoord: true, Epoch: epoch.J2000()},
	}}
	p2 := &mockProvider{name: "vizier", targets: map[string]Target{
		"target": {ID: "row-17", Coord: near, HasCoord: true, Epoch: epoch.J2000()},
	}}

	r := &Resolver{
		providers: []resolve.Provider{p1, p2},
		cfg:       resolverConfig{resolution: angle.Arcsec(2), cap: defaultCap},
	}

	got, err := r.Resolve(context.Background(), "target")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(got.Sources) != 2 || len(got.Associations) != 1 {
		t.Fatalf("Sources %+v, Associations %+v; want two records and one join", got.Sources, got.Associations)
	}

	a := got.Associations[0]
	if a.Authority != "astrogo" || a.Evidence != (resolve.Identifier{}) {
		t.Errorf("association %+v; want astrogo's, with no identifier", a)
	}

	if s := a.Separation.Arcseconds(); s < 0.49 || s > 0.51 {
		t.Errorf("Separation = %.3f″, want 0.5″", s)
	}
}

// TestResolverRecordsAConeSearchAssociation: a cone searcher's record folded
// into a group by position carries an association from the record that
// anchored the group, with the separation and astrogo as its authority.
func TestResolverRecordsAConeSearchAssociation(t *testing.T) {
	t.Parallel()

	p1 := &mockProvider{name: "simbad", targets: map[string]Target{
		"star": {ID: "S1", Coord: coord.NewICRS(angle.Deg(50), angle.Deg(10)), HasCoord: true, Epoch: epoch.J2000()},
	}}
	cs := &mockConeSearcher{targets: []Target{
		{ID: "GAIA123", Coord: coord.NewICRS(angle.Deg(50.0001), angle.Deg(10.0001)), HasCoord: true, Epoch: epoch.J2000(), Catalog: "Gaia DR3"},
	}}

	r := &Resolver{
		providers:     []resolve.Provider{p1},
		coneSearchers: []coneProvider{{name: "gaia", cs: cs}},
		cfg:           resolverConfig{resolution: angle.Arcsec(2), cap: defaultCap},
	}

	got, err := r.Resolve(context.Background(), "star")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(got.Associations) != 1 {
		t.Fatalf("Associations = %+v, want the fold", got.Associations)
	}

	a := got.Associations[0]

	wantFrom := resolve.SourceRecord{Provider: "simbad", ID: "S1"}
	wantTo := resolve.SourceRecord{Provider: "gaia", Release: "DR3", ID: "GAIA123"}

	if a.From != wantFrom || a.To != wantTo || a.Authority != "astrogo" {
		t.Errorf("association %+v; want simbad's S1 to gaia's DR3 GAIA123, by astrogo", a)
	}

	// 0.0001° in each coordinate at declination 10° is 0.505″.
	if s := a.Separation.Arcseconds(); s < 0.49 || s > 0.52 {
		t.Errorf("Separation = %.3f″, want 0.505″", s)
	}
}
