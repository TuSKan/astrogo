package catalog

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

type mockProvider struct {
	targets map[string]Target
	name    string

	// failWith, when set, is returned by both Resolve and Search instead of
	// consulting targets — the stand-in for a provider that cannot answer
	// (an outage, a cancelled context, a rate limit) as opposed to one that
	// answers "no such object". Keeping those separable is the whole point of
	// the error-returning Provider interface.
	failWith error
}

func (p *mockProvider) Name() string { return p.name }

func (p *mockProvider) Resolve(_ context.Context, query string) (Target, error) {
	if p.failWith != nil {
		return Target{}, p.failWith
	}

	t, ok := p.targets[resolve.Normalize(query)]
	if !ok {
		return Target{}, fmt.Errorf("%w: %q", resolve.ErrNotFound, query)
	}

	return t, nil
}

func (p *mockProvider) Search(_ context.Context, query string) ([]Target, error) {
	if p.failWith != nil {
		return nil, p.failWith
	}

	var res []Target

	q := resolve.Normalize(query)
	for _, t := range p.targets {
		if resolve.Normalize(t.Name) == q || resolve.Normalize(t.ID) == q {
			res = append(res, t)
		}
	}

	return res, nil
}

// mockConeSearcher is a resolve.ConeSearcher stub that ignores its request
// and always yields a fixed set of targets — matching production
// ConeSearchers (Gaia/VizieR), the request-driven filtering happens
// downstream in Resolver.foldConeSearchResults, not inside the searcher.
type mockConeSearcher struct {
	targets []Target
}

func (m *mockConeSearcher) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapConeSearch}
}

func (m *mockConeSearcher) ConeSearch(_ context.Context, _ resolve.ConeRequest) resolve.SeqIterator[Target] {
	return resolve.SliceSeq(m.targets)
}

// containsNormalized reports whether id appears in aliases, comparing under
// resolve.Normalize (matching how the alias-graph cross-match itself keys
// its buckets).
func containsNormalized(aliases []string, id string) bool {
	want := resolve.Normalize(id)
	for _, a := range aliases {
		if resolve.Normalize(a) == want {
			return true
		}
	}

	return false
}

func TestNewProvider(t *testing.T) {
	for _, src := range []Source{OpenNGC, SIMBAD, MAST, JPL, SBDB, Gaia, VizieR, NORAD} {
		p, err := NewProvider(src)
		if err != nil {
			t.Errorf("NewProvider(%d): unexpected error: %v", src, err)
			continue
		}

		if p == nil {
			t.Errorf("NewProvider(%d): got nil Provider, want a concrete one", src)
		}
	}
}

func TestNewProvider_UnknownSource(t *testing.T) {
	_, err := NewProvider(Source(999))
	if !errors.Is(err, ErrUnknownSource) {
		t.Errorf("NewProvider(999) error = %v, want ErrUnknownSource", err)
	}
}

// TestNewProvider_BrightObjectSearcherTypeAssertion confirms the pattern
// NewProvider's doc comment describes — a caller type-asserting the
// returned Provider to a narrower capability interface, the same way this
// package's own NewResolver does internally for resolve.ConeSearcher —
// actually works for a provider that implements
// resolve.BrightObjectSearcher.
func TestNewProvider_BrightObjectSearcherTypeAssertion(t *testing.T) {
	p, err := NewProvider(OpenNGC)
	if err != nil {
		t.Fatalf("NewProvider(OpenNGC): %v", err)
	}

	if _, ok := p.(resolve.BrightObjectSearcher); !ok {
		t.Error("expected the OpenNGC provider to implement resolve.BrightObjectSearcher")
	}
}

func TestResolver_Resolve(t *testing.T) {
	p1 := &mockProvider{
		name: "p1",
		targets: map[string]Target{
			"m42": {ID: "m42", Name: "Orion Nebula", Catalog: "p1"},
		},
	}
	p2 := &mockProvider{
		name: "p2",
		targets: map[string]Target{
			"m31": {ID: "m31", Name: "Andromeda", Catalog: "p2"},
		},
	}

	r := &Resolver{providers: []resolve.Provider{p1, p2}}

	tests := []struct {
		wantErr error
		query   string
		wantID  string
	}{
		{nil, "M42", "m42"},
		{nil, "m 42", "m42"},
		{nil, "Messier 42", "m42"},
		{nil, "M31", "m31"},
		{ErrNotFound, "M43", ""},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got, err := r.Resolve(context.Background(), tt.query)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && got.ID != tt.wantID {
				t.Errorf("Resolve() got ID = %v, want %v", got.ID, tt.wantID)
			}
		})
	}
}

// TestResolver_ProviderPriority confirms that when multiple providers
// resolve the same physical object (linked here by a shared alias), Resolve
// merges their disjoint fields into one Target rather than any single
// provider's whole record winning outright — and that Provenance records
// which provider contributed each field.
func TestResolver_ProviderPriority(t *testing.T) {
	p1 := &mockProvider{name: "gaia", targets: map[string]Target{
		"target": {
			ID: "gaia-1", Aliases: []string{"shared"},
			Coord: coord.NewICRS(angle.Deg(101.287155), angle.Deg(-16.716116)), HasCoord: true,
			Epoch: time.J2000,
			PmRA:  angle.Arcsec(-0.379), PmDec: angle.Arcsec(-1.303), Parallax: angle.Arcsec(0.379),
		},
	}}
	p2 := &mockProvider{name: "simbad", targets: map[string]Target{
		"target": {
			ID: "* alf CMa", Name: "Sirius", Aliases: []string{"shared", "HD 48915"},
			VMag: -1.46, HasVMag: true,
		},
	}}
	p3 := &mockProvider{name: "mast", targets: map[string]Target{
		"target": {
			ID: "MAST-1", Name: "Sirius A", Aliases: []string{"shared"},
		},
	}}

	r := &Resolver{
		providers: []resolve.Provider{p1, p2, p3},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	got, err := r.Resolve(context.Background(), "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ID != "gaia-1" || got.Provenance["ID"] != "gaia" {
		t.Errorf("expected gaia's ID (first-registered with a value), got ID=%q provenance=%q", got.ID, got.Provenance["ID"])
	}

	if got.Name != "Sirius" || got.Provenance["Name"] != "simbad" {
		t.Errorf("expected simbad's Name (first non-empty after gaia), got Name=%q provenance=%q", got.Name, got.Provenance["Name"])
	}

	if !got.HasCoord || got.Provenance["Coord"] != "gaia" {
		t.Errorf("expected gaia's Coord (astrometric precedence), got HasCoord=%v provenance=%q", got.HasCoord, got.Provenance["Coord"])
	}

	testutil.AssertNear(t, "merged PmRA", got.PmRA.Arcseconds(), -0.379, 1e-9)

	if got.Provenance["PmRA"] != "gaia" {
		t.Errorf("expected gaia's PmRA as part of the coupled astrometric cluster, provenance=%q", got.Provenance["PmRA"])
	}

	if got.VMag != -1.46 || got.Provenance["VMag"] != "simbad" {
		t.Errorf("expected simbad's VMag, got %v/%q", got.VMag, got.Provenance["VMag"])
	}

	for _, id := range []string{"gaia-1", "* alf CMa", "HD 48915", "MAST-1"} {
		if !containsNormalized(got.Aliases, id) {
			t.Errorf("expected merged Aliases to include %q, got %v", id, got.Aliases)
		}
	}
}

// TestResolver_MergePreservesOrbitalElements is a regression test for a
// real bug: resolve.Target's osculating-orbital-element fields
// (HasElements/SemiMajorAxis/.../MeanAnomaly, added alongside
// catalog/sbdb's element decoding) were never wired into mergeGroup's
// scalarFieldRules, so a Resolver-mediated Resolve() silently dropped
// them even though the underlying SBDB provider populated them
// correctly — only a direct sbdb.New() caller kept them. Also confirms
// the elements' own Epoch wins over a differently-sourced generic Epoch,
// since pairing these elements with a foreign epoch would be physically
// wrong.
func TestResolver_MergePreservesOrbitalElements(t *testing.T) {
	elementsEpoch := time.FromJDParts(2461200.5, 0, time.TDB)

	p1 := &mockProvider{name: "sbdb", targets: map[string]Target{
		"target": {
			ID: "20000001", Name: "1 Ceres", Aliases: []string{"shared"}, HasElements: true,
			Epoch:         elementsEpoch,
			SemiMajorAxis: 2.77,
			Eccentricity:  0.0797,
			Inclination:   angle.Deg(10.6),
			AscendingNode: angle.Deg(80.2),
			ArgPeriapsis:  angle.Deg(73.3),
			MeanAnomaly:   angle.Deg(274),
		},
	}}
	p2 := &mockProvider{name: "simbad", targets: map[string]Target{
		"target": {
			ID: "simbad-1", Aliases: []string{"shared"}, Epoch: time.J2000, // a different, unrelated epoch — must not win
		},
	}}

	r := &Resolver{
		providers: []resolve.Provider{p1, p2},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	got, err := r.Resolve(context.Background(), "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !got.HasElements {
		t.Fatal("expected merged Target to carry HasElements=true from sbdb")
	}

	if got.Provenance["OrbitalElements"] != "sbdb" {
		t.Errorf("expected OrbitalElements provenance = sbdb, got %q", got.Provenance["OrbitalElements"])
	}

	testutil.AssertNear(t, "merged SemiMajorAxis", got.SemiMajorAxis, 2.77, 1e-9)
	testutil.AssertNear(t, "merged Eccentricity", got.Eccentricity, 0.0797, 1e-9)
	testutil.AssertNear(t, "merged Inclination", got.Inclination.Degrees(), 10.6, 1e-9)
	testutil.AssertNear(t, "merged AscendingNode", got.AscendingNode.Degrees(), 80.2, 1e-9)
	testutil.AssertNear(t, "merged ArgPeriapsis", got.ArgPeriapsis.Degrees(), 73.3, 1e-9)
	testutil.AssertNear(t, "merged MeanAnomaly", got.MeanAnomaly.Degrees(), 274, 1e-9)

	if !got.Epoch.Equal(elementsEpoch) {
		t.Errorf("expected merged Epoch to be the elements' own epoch %v, got %v", elementsEpoch, got.Epoch)
	}

	if got.Provenance["Epoch"] != "sbdb" {
		t.Errorf("expected Epoch provenance = sbdb (paired with the elements, not simbad's unrelated epoch), got %q", got.Provenance["Epoch"])
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"M 42", "m42"},
		{"NGC 1976", "ngc1976"},
		{"Messier 42", "m42"},
		{"  Orion  ", "orion"},
	}

	for _, tt := range tests {
		if got := resolve.Normalize(tt.input); got != tt.want {
			t.Errorf("resolve.Normalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func BenchmarkResolve(b *testing.B) {
	p := &mockProvider{
		name: "p",
		targets: map[string]Target{
			"m42": {ID: "m42", Name: "Orion Nebula", Catalog: "p"},
		},
	}

	r := &Resolver{providers: []resolve.Provider{p}}
	for b.Loop() {
		_, _ = r.Resolve(context.Background(), "M42")
	}
}

// ── mergeGroup: field-precedence merge unit tests ───────────────────────────

func TestMergeGroup_IdentityFieldsFirstNonEmptyWins(t *testing.T) {
	g := group{candidates: []candidate{
		{provider: "p1", target: Target{}},
		{provider: "p2", target: Target{ID: "id2", Name: "Name2", Catalog: "cat2"}},
		{provider: "p3", target: Target{ID: "id3", Name: "Name3", Catalog: "cat3"}},
	}}

	got := mergeGroup(g)

	if got.ID != "id2" || got.Name != "Name2" || got.Catalog != "cat2" {
		t.Fatalf("expected first-non-empty (p2) to win identity fields, got %+v", got)
	}

	if got.Provenance["ID"] != "p2" || got.Provenance["Name"] != "p2" || got.Provenance["Catalog"] != "p2" {
		t.Errorf("expected identity field provenance = p2, got %+v", got.Provenance)
	}
}

func TestMergeGroup_AliasesAlwaysUnioned(t *testing.T) {
	g := group{candidates: []candidate{
		{provider: "p1", target: Target{ID: "id1", Aliases: []string{"alpha"}}},
		{provider: "p2", target: Target{ID: "id2", Aliases: []string{"alpha", "beta"}}},
	}}

	got := mergeGroup(g)

	want := []string{"id1", "alpha", "id2", "beta"}
	if len(got.Aliases) != len(want) {
		t.Fatalf("Aliases = %v, want %v", got.Aliases, want)
	}

	for i, w := range want {
		if resolve.Normalize(got.Aliases[i]) != resolve.Normalize(w) {
			t.Errorf("Aliases[%d] = %q, want %q", i, got.Aliases[i], w)
		}
	}
}

// TestMergeGroup_AstrometricClusterComesFromOneProvider confirms
// Coord/PmRA/PmDec/Parallax are treated as one coupled cluster taken
// entirely from the highest-precedence trustworthy provider, never mixed
// field-by-field across providers (which would describe an internally
// inconsistent astrometric solution).
func TestMergeGroup_AstrometricClusterComesFromOneProvider(t *testing.T) {
	gaiaCoord := coord.NewICRS(angle.Deg(10), angle.Deg(20))
	simbadCoord := coord.NewICRS(angle.Deg(11), angle.Deg(21)) // deliberately different, to prove it's not used

	g := group{candidates: []candidate{
		{provider: "simbad", target: Target{
			Coord: simbadCoord, HasCoord: true,
			PmRA: angle.Arcsec(99), PmDec: angle.Arcsec(99), Parallax: angle.Arcsec(99),
		}},
		{provider: "gaia", target: Target{
			Coord: gaiaCoord, HasCoord: true,
			PmRA: angle.Arcsec(1.2), PmDec: angle.Arcsec(-3.4), Parallax: angle.Arcsec(5.6),
		}},
	}}

	got := mergeGroup(g)

	if !got.Coord.Equal(gaiaCoord) {
		t.Errorf("Coord = %v, want gaia's %v (gaia outranks simbad)", got.Coord, gaiaCoord)
	}

	testutil.AssertNear(t, "PmRA", got.PmRA.Arcseconds(), 1.2, 1e-9)
	testutil.AssertNear(t, "PmDec", got.PmDec.Arcseconds(), -3.4, 1e-9)
	testutil.AssertNear(t, "Parallax", got.Parallax.Arcseconds(), 5.6, 1e-9)

	for _, field := range []string{"Coord", "PmRA", "PmDec", "Parallax"} {
		if got.Provenance[field] != "gaia" {
			t.Errorf("Provenance[%q] = %q, want gaia", field, got.Provenance[field])
		}
	}
}

// TestMergeGroup_UntrustworthyCoordFallsBackToNextProvider exercises
// trustworthyCoord's guard against the bug class where Gaia/MAST set
// HasCoord unconditionally even when the underlying position is actually
// (0,0) — the merge must skip straight past that candidate to the next
// provider in astrometric precedence.
func TestMergeGroup_UntrustworthyCoordFallsBackToNextProvider(t *testing.T) {
	simbadCoord := coord.NewICRS(angle.Deg(15), angle.Deg(25))

	g := group{candidates: []candidate{
		{provider: "gaia", target: Target{Coord: coord.ICRS{}, HasCoord: true}},
		{provider: "simbad", target: Target{Coord: simbadCoord, HasCoord: true}},
	}}

	got := mergeGroup(g)

	if !got.Coord.Equal(simbadCoord) {
		t.Errorf("expected fallback to simbad's coord since gaia's is untrustworthy, got %v", got.Coord)
	}

	if got.Provenance["Coord"] != "simbad" {
		t.Errorf("Provenance[Coord] = %q, want simbad", got.Provenance["Coord"])
	}
}

// TestMergeGroup_VMagPrecedence confirms real photometry (SIMBAD) outranks
// Gaia's derived G->V estimate.
func TestMergeGroup_VMagPrecedence(t *testing.T) {
	g := group{candidates: []candidate{
		{provider: "gaia", target: Target{VMag: 5.0, HasVMag: true}},
		{provider: "simbad", target: Target{VMag: 4.5, HasVMag: true}},
	}}

	got := mergeGroup(g)

	if got.VMag != 4.5 || got.Provenance["VMag"] != "simbad" {
		t.Errorf("expected simbad's VMag (real photometry outranks Gaia's derived estimate), got VMag=%v provenance=%v",
			got.VMag, got.Provenance["VMag"])
	}
}

// TestMergeGroup_RadialVelocityZeroSurvivesMerge regression-tests the
// HasRadialVelocity fix: RadialVelocity == 0 is physically legitimate (a
// star moving neither toward nor away), so the merge must not treat it the
// same as "no RV on file" the way a bare `RadialVelocity != 0` presence
// check would.
func TestMergeGroup_RadialVelocityZeroSurvivesMerge(t *testing.T) {
	g := group{candidates: []candidate{
		{provider: "simbad", target: Target{RadialVelocity: 0, HasRadialVelocity: true}},
	}}

	got := mergeGroup(g)

	if !got.HasRadialVelocity {
		t.Error("expected HasRadialVelocity=true to survive the merge for a genuine zero RV")
	}

	if got.RadialVelocity != 0 {
		t.Errorf("expected RadialVelocity=0, got %v", got.RadialVelocity)
	}

	if got.Provenance["RadialVelocity"] != "simbad" {
		t.Errorf("expected provenance simbad, got %v", got.Provenance["RadialVelocity"])
	}
}

// TestMergeGroup_RadialVelocityUnsetIsDropped confirms the inverse: a
// candidate with no RV on file at all (HasRadialVelocity=false) does not
// contribute a value, so a differently-sourced field on the same merged
// Target doesn't spuriously pick up a stray 0.
func TestMergeGroup_RadialVelocityUnsetIsDropped(t *testing.T) {
	g := group{candidates: []candidate{
		{provider: "simbad", target: Target{RadialVelocity: 0, HasRadialVelocity: false, VMag: 5.0, HasVMag: true}},
	}}

	got := mergeGroup(g)

	if got.HasRadialVelocity {
		t.Error("expected HasRadialVelocity=false to survive the merge when no provider set it")
	}

	if _, ok := got.Provenance["RadialVelocity"]; ok {
		t.Error("expected no RadialVelocity provenance entry when no candidate had HasRadialVelocity=true")
	}
}

// TestMergeGroup_PhysicalParamsClusterIncludesDiameterAndAlbedo regression-
// tests the SBDB-only H/G/M1/K1/M2/K2/G1/G2/Diameter/Albedo cluster rule —
// real Eros phys_par values, confirming the merge carries the measured
// Diameter/Albedo through as one coupled unit alongside H/G, not just the
// pre-existing H/G/M1/K1 fields the rule predates.
func TestMergeGroup_PhysicalParamsClusterIncludesDiameterAndAlbedo(t *testing.T) {
	g := group{candidates: []candidate{
		{provider: "sbdb", target: Target{
			H: 10.40, G: 0.46, HasH: true,
			Diameter: 16.84, HasDiameter: true,
			Albedo: 0.25, HasAlbedo: true,
		}},
	}}

	got := mergeGroup(g)

	if !got.HasH || got.H != 10.40 {
		t.Errorf("H = %v (has=%v), want 10.40 (has=true)", got.H, got.HasH)
	}

	if !got.HasDiameter || got.Diameter != 16.84 {
		t.Errorf("Diameter = %v (has=%v), want 16.84 (has=true)", got.Diameter, got.HasDiameter)
	}

	if !got.HasAlbedo || got.Albedo != 0.25 {
		t.Errorf("Albedo = %v (has=%v), want 0.25 (has=true)", got.Albedo, got.HasAlbedo)
	}

	if got.Provenance["PhysicalParams"] != "sbdb" {
		t.Errorf("expected provenance sbdb, got %v", got.Provenance["PhysicalParams"])
	}
}

// ── Cross-match integration tests (via Resolver.Resolve/Search) ────────────

// TestResolver_CrossMatchByAlias confirms two providers sharing the same ID
// string (the direct generalization of bigsky's "join on shared Tycho ID")
// merge into one Target combining their disjoint fields.
func TestResolver_CrossMatchByAlias(t *testing.T) {
	p1 := &mockProvider{name: "gaia", targets: map[string]Target{
		"sirius": {
			ID: "gaia-1", Coord: coord.NewICRS(angle.Deg(101.28), angle.Deg(-16.71)),
			HasCoord: true, Epoch: time.J2000,
		},
	}}
	p2 := &mockProvider{name: "simbad", targets: map[string]Target{
		"sirius": {ID: "gaia-1", Aliases: []string{"HD 48915"}, VMag: -1.46, HasVMag: true},
	}}

	r := &Resolver{
		providers: []resolve.Provider{p1, p2},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	got, err := r.Resolve(context.Background(), "sirius")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !got.HasCoord {
		t.Error("expected merged target to carry gaia's Coord")
	}

	if got.VMag != -1.46 {
		t.Errorf("expected merged target to carry simbad's VMag, got %v", got.VMag)
	}

	if !containsNormalized(got.Aliases, "HD 48915") {
		t.Errorf("expected merged Aliases to include HD 48915, got %v", got.Aliases)
	}
}

// TestResolver_CrossMatchByPosition_SameEpochMerges confirms two providers
// with no shared alias/ID, but positions within the default 2" threshold at
// the same epoch, merge via the positional fallback.
func TestResolver_CrossMatchByPosition_SameEpochMerges(t *testing.T) {
	p1 := &mockProvider{name: "p1", targets: map[string]Target{
		"a": {ID: "A1", Name: "TestObj", Coord: coord.NewICRS(angle.Deg(83.822), angle.Deg(-5.391)), HasCoord: true, Epoch: time.J2000},
	}}
	p2 := &mockProvider{name: "p2", targets: map[string]Target{
		"b": {ID: "B2", Name: "TestObj", Coord: coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911)), HasCoord: true, Epoch: time.J2000},
	}}

	r := &Resolver{
		providers: []resolve.Provider{p1, p2},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	got, err := r.Search(context.Background(), "TestObj")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected positional fallback to merge into 1 target, got %d: %+v", len(got), got)
	}

	if !containsNormalized(got[0].Aliases, "A1") || !containsNormalized(got[0].Aliases, "B2") {
		t.Errorf("expected merged Aliases to include both source IDs, got %v", got[0].Aliases)
	}
}

// TestResolver_CrossMatchByPosition_TooFarDoesNotMerge is the negative case:
// two candidates well outside the match threshold must remain separate
// Targets, not be forced together.
func TestResolver_CrossMatchByPosition_TooFarDoesNotMerge(t *testing.T) {
	p1 := &mockProvider{name: "p1", targets: map[string]Target{
		"a": {ID: "A1", Name: "TestObj", Coord: coord.NewICRS(angle.Deg(83.80), angle.Deg(-5.39)), HasCoord: true, Epoch: time.J2000},
	}}
	p2 := &mockProvider{name: "p2", targets: map[string]Target{
		"b": {ID: "B2", Name: "TestObj", Coord: coord.NewICRS(angle.Deg(83.82), angle.Deg(-5.39)), HasCoord: true, Epoch: time.J2000},
	}}

	r := &Resolver{
		providers: []resolve.Provider{p1, p2},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	got, err := r.Search(context.Background(), "TestObj")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected two distinct targets beyond the match threshold, got %d: %+v", len(got), got)
	}
}

// TestResolver_CrossMatchByPosition_EpochMismatchAppliesPropagation builds a
// second candidate 50 (simulated) years after the first, carrying the exact
// proper motion needed to have arrived at its stored position from the
// first candidate's — proving the positional fallback genuinely propagates
// through coord.PropagateEpoch before comparing, not just comparing raw
// stored coordinates. The sanity check confirms the raw (unpropagated)
// separation is far outside the threshold, so a regression that skipped
// propagation would fail this test honestly instead of passing by
// coincidence.
func TestResolver_CrossMatchByPosition_EpochMismatchAppliesPropagation(t *testing.T) {
	baseRA, baseDec := angle.Deg(10), angle.Deg(0)
	pmRA, pmDec := angle.Arcsec(1.0), angle.Arcsec(0)
	parallax := angle.Arcsec(0.01)

	laterEpoch := time.J2000.Add((50 * 365.25 * 24) * time.Hour)

	base := coord.NewICRSWithKinematics(baseRA, baseDec, pmRA, pmDec, parallax, 0)

	laterCoord, err := coord.PropagateEpoch(base, time.J2000, laterEpoch)
	if err != nil {
		t.Fatalf("test setup: PropagateEpoch: %v", err)
	}

	if sep := coord.Separation(base, laterCoord); sep.Arcseconds() < defaultPositionMatchThreshold.Arcseconds() {
		t.Fatalf("test setup invalid: raw separation %.3f\" already within threshold", sep.Arcseconds())
	}

	p1 := &mockProvider{name: "p1", targets: map[string]Target{
		"a": {ID: "P1", Name: "TestStar", Coord: base, HasCoord: true, Epoch: time.J2000},
	}}
	p2 := &mockProvider{name: "p2", targets: map[string]Target{
		"b": {
			ID: "P2", Name: "TestStar", Coord: laterCoord, HasCoord: true, Epoch: laterEpoch,
			PmRA: pmRA, PmDec: pmDec, Parallax: parallax,
		},
	}}

	r := &Resolver{
		providers: []resolve.Provider{p1, p2},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	got, err := r.Search(context.Background(), "TestStar")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected epoch-normalized positional match to merge into 1 target, got %d: %+v", len(got), got)
	}
}

// TestResolver_ConeSearchBridge_FoldsInAstrometry confirms a registered
// ConeSearcher (Gaia/VizieR's real role) contributes its astrometry into a
// group anchored by a name-resolvable provider's position, and that its
// higher astrometric precedence then wins the merge.
func TestResolver_ConeSearchBridge_FoldsInAstrometry(t *testing.T) {
	anchor := coord.NewICRS(angle.Deg(50), angle.Deg(10))
	coneCoord := coord.NewICRS(angle.Deg(50.0001), angle.Deg(10.0001)) // ~0.5" from anchor

	p1 := &mockProvider{name: "simbad", targets: map[string]Target{
		"star": {ID: "S1", Name: "Star", Coord: anchor, HasCoord: true, Epoch: time.J2000},
	}}

	cs := &mockConeSearcher{targets: []Target{
		{ID: "GAIA123", Coord: coneCoord, HasCoord: true, Epoch: time.J2000, Parallax: angle.Arcsec(5)},
	}}

	r := &Resolver{
		providers:     []resolve.Provider{p1},
		coneSearchers: []coneProvider{{name: "gaia", cs: cs}},
		cfg:           resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	got, err := r.Resolve(context.Background(), "star")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Provenance["Coord"] != "gaia" {
		t.Errorf("expected the bridged gaia candidate to win Coord precedence, provenance=%q", got.Provenance["Coord"])
	}

	testutil.AssertNear(t, "Parallax", got.Parallax.Arcseconds(), 5, 1e-9)

	if got.Provenance["Parallax"] != "gaia" {
		t.Errorf("expected Parallax provenance = gaia, got %q", got.Provenance["Parallax"])
	}

	if !containsNormalized(got.Aliases, "GAIA123") {
		t.Errorf("expected merged Aliases to include the bridged candidate's ID, got %v", got.Aliases)
	}
}

// ── Configurable chained setters ─────────────────────────────────────────────

func TestResolver_Limit(t *testing.T) {
	r := &Resolver{cfg: resolverConfig{cap: defaultCap}}

	if got := r.Limit(5); got != r {
		t.Error("Limit should return r for chaining")
	}

	if r.cfg.cap != 5 {
		t.Errorf("Limit(5) set cap = %d, want 5", r.cfg.cap)
	}
}

func TestResolver_PositionMatchThreshold(t *testing.T) {
	r := &Resolver{cfg: resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold}}

	if got := r.PositionMatchThreshold(angle.Arcsec(10)); got != r {
		t.Error("PositionMatchThreshold should return r for chaining")
	}

	testutil.AssertNear(t, "positionMatchThreshold", r.cfg.positionMatchThreshold.Arcseconds(), 10, 1e-9)
}

// TestResolver_Search_RespectsCap confirms Search actually caps its result
// count at cfg.cap rather than the old hardcoded 10.
func TestResolver_Search_RespectsCap(t *testing.T) {
	p := &mockProvider{name: "p", targets: map[string]Target{
		"1": {ID: "T1", Name: "TestObj"},
		"2": {ID: "T2", Name: "TestObj"},
		"3": {ID: "T3", Name: "TestObj"},
	}}

	// All three targets share no aliases/IDs/positions, so each remains its
	// own group; Search's rank-then-cap step is what limits the count.
	r := &Resolver{
		providers: []resolve.Provider{p},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: 2},
	}

	got, err := r.Search(context.Background(), "TestObj")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected Limit(2)'s configured cap to limit results to 2, got %d", len(got))
	}
}

// TestResolver_PositionMatchThreshold_Configurable demonstrates that
// widening PositionMatchThreshold changes cross-match behavior: the same
// two candidates fail to merge under a tight threshold but do merge under
// a looser one.
func TestResolver_PositionMatchThreshold_Configurable(t *testing.T) {
	newResolver := func(threshold angle.Angle) *Resolver {
		p1 := &mockProvider{name: "p1", targets: map[string]Target{
			"a": {ID: "A1", Name: "TestObj", Coord: coord.NewICRS(angle.Deg(83.80), angle.Deg(-5.39)), HasCoord: true, Epoch: time.J2000},
		}}
		p2 := &mockProvider{name: "p2", targets: map[string]Target{
			"b": {ID: "B2", Name: "TestObj", Coord: coord.NewICRS(angle.Deg(83.8003), angle.Deg(-5.39)), HasCoord: true, Epoch: time.J2000},
		}}

		return &Resolver{
			providers: []resolve.Provider{p1, p2},
			cfg:       resolverConfig{positionMatchThreshold: threshold, cap: defaultCap},
		}
	}

	// ~1.08" apart (0.0003 deg * 3600 * cos(-5.39deg)).
	tight := newResolver(angle.Arcsec(0.5))

	got, err := tight.Search(context.Background(), "TestObj")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected a tight 0.5\" threshold to keep candidates separate, got %d results", len(got))
	}

	loose := newResolver(angle.Arcsec(2))

	got, err = loose.Search(context.Background(), "TestObj")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected a loose 2\" threshold to merge the same candidates, got %d results", len(got))
	}
}
