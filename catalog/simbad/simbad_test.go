package simbad

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/unit"
)

func TestParseCSV(t *testing.T) {
	f, err := os.Open("testdata/m31.csv")
	if err != nil {
		t.Fatalf("failed to open test fixture: %v", err)
	}

	t.Cleanup(func() {
		err := f.Close()
		if err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	targets, err := ParseCSV(f)
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 unique target, got %d", len(targets))
	}

	tgt := targets[0]
	if tgt.ID != "NAME M  31" {
		t.Errorf("unexpected ID: %s", tgt.ID)
	}

	if tgt.Kind != resolve.KindGalaxy {
		t.Errorf("unexpected Kind: %s", tgt.Kind)
	}

	if len(tgt.Aliases) != 3 {
		t.Errorf("expected 3 aliases, got %v", tgt.Aliases)
	}

	if !tgt.HasCoord {
		t.Fatalf("Coord is missing")
	}

	if math.Abs(tgt.Coord.RA().Degrees()-10.68470833) > 1e-6 {
		t.Errorf("unexpected RA: %f", tgt.Coord.RA().Degrees())
	}
}

func TestResolveMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		data, err := os.ReadFile("testdata/m31.csv")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/csv")

		if _, err := w.Write(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	// temporarily override the global endpoint for testing
	// In a complete implementation we might want to dependency-inject `tapSyncURL`,
	// but for testing we can define a client specifically talking to it.
	// Since we defined tapSyncURL as const, we just test the public method? Actually,
	// test ParseCSV is the real test. We can just test Provider behavior if we can mock Client Transport.

	p := New()

	redirect(t, server.URL)

	tgt, err := p.Resolve(context.Background(), "m31")
	if err != nil {
		t.Fatalf("failed to resolve target")
	}

	if tgt.ID != "NAME M  31" {
		t.Errorf("unexpected ID: %s", tgt.ID)
	}
}

func TestRetryTimeout(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++

		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	redirect(t, server.URL)

	p := New()

	ctx := context.Background()
	req := resolve.ObjectRequest{Query: "test"}
	iter := p.ResolveObject(ctx, req)

	iter(func(_ resolve.Target, err error) bool {
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		return false
	})

	if attempts == 0 {
		t.Errorf("expected multiple attempts")
	}
}

func TestParseEmptyCSV(t *testing.T) {
	f, err := os.Open("testdata/empty.csv")
	if err != nil {
		t.Fatalf("failed to open test fixture: %v", err)
	}

	t.Cleanup(func() {
		err := f.Close()
		if err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	targets, err := ParseCSV(f)
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	if len(targets) != 0 {
		t.Fatalf("Expected 0 targets for empty, got %d", len(targets))
	}
}

func TestParseMalformedCSV(t *testing.T) {
	f, err := os.Open("testdata/malformed.csv")
	if err != nil {
		t.Fatalf("failed to open test fixture: %v", err)
	}

	t.Cleanup(func() {
		err := f.Close()
		if err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	_, err = ParseCSV(f)
	if err == nil {
		t.Fatalf("Expected ParseCSV to fail on malformed data")
	}
}

// TestParseCSV_PopulatesVMag guards against a real bug found via live
// verification: SIMBAD's TAP response names the flux column "V"
// (uppercase, matching the unaliased `allfluxes.V` in BuildResolveQuery's
// SELECT list) — ParseCSV originally looked up "v" (lowercase) and so
// never populated VMag/HasVMag from a real response, silently, since the
// column is optional.
func TestParseCSV_PopulatesVMag(t *testing.T) {
	f, err := os.Open("testdata/vmag.csv")
	if err != nil {
		t.Fatalf("failed to open test fixture: %v", err)
	}

	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	targets, err := ParseCSV(f)
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	tgt := targets[0]
	if !tgt.HasVMag || tgt.VMag != -1.46 {
		t.Errorf("VMag = %v (HasVMag=%v), want -1.46 (HasVMag=true)", tgt.VMag, tgt.HasVMag)
	}

	// The fixture's rvz_radvel column (-5.5) was already being parsed here
	// but never asserted on -- confirm HasRadialVelocity is set alongside
	// the value, not just the value itself.
	if !tgt.HasRadialVelocity || tgt.RadialVelocity != unit.KmPerSec(-5.5) {
		t.Errorf("RadialVelocity = %v (HasRadialVelocity=%v), want -5.5 km/s (HasRadialVelocity=true)",
			tgt.RadialVelocity, tgt.HasRadialVelocity)
	}
}

// TestParseCSV_ZeroRadialVelocityStillHasFlag is the actual regression
// case HasRadialVelocity exists for: a genuine 0 km/s measured RV (moving
// neither toward nor away) must still set HasRadialVelocity=true -- the
// old `RadialVelocity != 0` presence check this field replaced would have
// silently treated this identically to "no RV on file at all".
func TestParseCSV_ZeroRadialVelocityStillHasFlag(t *testing.T) {
	f, err := os.Open("testdata/rv_zero.csv")
	if err != nil {
		t.Fatalf("failed to open test fixture: %v", err)
	}

	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	targets, err := ParseCSV(f)
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	tgt := targets[0]
	if !tgt.HasRadialVelocity {
		t.Error("expected HasRadialVelocity=true for a genuine 0 km/s measured RV")
	}

	if tgt.RadialVelocity != 0 {
		t.Errorf("RadialVelocity = %v, want 0", tgt.RadialVelocity)
	}
}

func TestBuildBrightQuery(t *testing.T) {
	adql := BuildBrightQuery(resolve.BrightRequest{MaxVMag: 2, Limit: 50})

	if !strings.Contains(adql, "WHERE allfluxes.V < 2") {
		t.Errorf("expected a magnitude WHERE clause, got: %s", adql)
	}

	if !strings.Contains(adql, "allfluxes.V AS vmag") {
		t.Errorf("expected allfluxes.V aliased to vmag, got: %s", adql)
	}

	// ORDER BY must reference the output alias, not the qualified
	// table.column form — confirmed live against SIMBAD's TAP service that
	// "ORDER BY allfluxes.V ASC" is rejected ("Incorrect ADQL query:
	// Encountered '.'"), while "ORDER BY vmag ASC" succeeds.
	if !strings.Contains(adql, "ORDER BY vmag ASC") {
		t.Errorf("expected brightest-first ordering by the vmag alias, got: %s", adql)
	}

	if strings.Contains(adql, "ident") {
		t.Errorf("expected no ident join (no name to match against a bulk browse), got: %s", adql)
	}

	if !strings.Contains(adql, "TOP 50") {
		t.Errorf("expected the requested limit, got: %s", adql)
	}
}

// simbadKind follows SIMBAD's own object-type hierarchy. Each row is a code
// and the path SIMBAD's otypedef table gives it, copied from the live service,
// with the kind an observer would call it.
//
// Stars that the old string match lost, because a candidate code drops the
// '*': Aldebaran is "LP?". Galaxies it lost: M33 is "GiG", M77 "Sy2". Clusters
// it flattened, associations it called stars, and the detection-only codes
// that stay KindOther because SIMBAD has not said what the source is (#601).
//
// It was first written against SIMBAD's real bright-star codes, after a
// switch that knew only "Star", "V*" and "Em*" mislabeled nearly every real
// star; "Star" turned out not to be a code at all.
func TestSimbadKindFollowsSIMBADsHierarchy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		otype, path string
		want        resolve.Kind
	}{
		{"*", "*", resolve.KindStar},
		{"SB*", "* > ** > SB*", resolve.KindStar},
		{"PM*", "* > PM*", resolve.KindStar},
		{"dS*", "* > MS* > dS*", resolve.KindStar},
		{"RG*", "* > Ev* > RG*", resolve.KindStar},
		{"s*b", "* > Ma* > sg* > s*b", resolve.KindStar},
		{"s*r", "* > Ma* > sg* > s*r", resolve.KindStar},
		{"WD*", "* > Ev* > WD*", resolve.KindStar},
		{"V*", "* > V*", resolve.KindStar},
		{"Em*", "* > Em*", resolve.KindStar},
		{"EB*", "* > ** > EB*", resolve.KindStar},
		{"LP?", "* > Ev* > LP*", resolve.KindStar},
		{"bC?", "* > Ma* > bC*", resolve.KindStar},
		{"EB?", "* > ** > EB*", resolve.KindStar},
		{"s?r", "* > Ma* > sg* > s*r", resolve.KindStar},
		{"Y*?", "* > Y*O", resolve.KindStar},
		{"HXB", "* > ** > XB* > HXB", resolve.KindStar},
		{"**", "* > **", resolve.KindDoubleStar},
		{"**?", "* > **", resolve.KindDoubleStar},
		{"PN", "* > Ev* > PN", resolve.KindNebula},
		{"PN?", "* > Ev* > PN", resolve.KindNebula},
		{"OpC", "Cl* > OpC", resolve.KindOpenCluster},
		{"GlC", "Cl* > GlC", resolve.KindGlobularCluster},
		{"Gl?", "Cl* > GlC", resolve.KindGlobularCluster},
		{"Cl*", "Cl*", resolve.KindStarCluster},
		{"Cl?", "Cl*", resolve.KindStarCluster},
		{"As*", "As*", resolve.KindStarCluster},
		{"St*", "As* > St*", resolve.KindStarCluster},
		{"MGr", "As* > MGr", resolve.KindStarCluster},
		{"G", "G", resolve.KindGalaxy},
		{"AGN", "G > AGN", resolve.KindGalaxy},
		{"Sy2", "G > AGN > SyG > Sy2", resolve.KindGalaxy},
		{"GiG", "G > GiG", resolve.KindGalaxy},
		{"SBG", "G > SBG", resolve.KindGalaxy},
		{"QSO", "G > AGN > QSO", resolve.KindGalaxy},
		{"BLL", "G > AGN > QSO > Bla > BLL", resolve.KindGalaxy},
		{"GrG", "GrG", resolve.KindGalaxy},
		{"ClG", "ClG", resolve.KindGalaxy},
		{"PaG", "PaG", resolve.KindGalaxy},
		{"IG", "IG", resolve.KindGalaxy},
		{"HII", "ISM > HII", resolve.KindNebula},
		{"GNe", "ISM > Cld > GNe", resolve.KindNebula},
		{"RNe", "ISM > Cld > GNe > RNe", resolve.KindNebula},
		{"DNe", "ISM > Cld > DNe", resolve.KindNebula},
		{"Cld", "ISM > Cld", resolve.KindNebula},
		{"SNR", "ISM > SNR", resolve.KindSupernovaRemnant},
		{"SR?", "ISM > SNR", resolve.KindSupernovaRemnant},
		{"X", "X", resolve.KindOther},
		{"UV", "UV", resolve.KindOther},
		{"Rad", "Rad", resolve.KindOther},
		{"IR", "IR", resolve.KindOther},
		{"EmO", "Opt > EmO", resolve.KindOther},
		{"gLe", "grv > gLS > gLe", resolve.KindOther},
		{"reg", "reg", resolve.KindOther},
		{"?", "", resolve.KindOther},
		{"err", "err", resolve.KindOther},
	}

	for _, tt := range tests {
		if got := simbadKind(tt.otype, tt.path); got != tt.want {
			t.Errorf("simbadKind(%q, %q) = %q, want %q", tt.otype, tt.path, got, tt.want)
		}
	}
}

// With no path, which only an otype missing from otypedef would have, the
// code stands in for a one-level path: the hierarchy's roots still classify.
func TestSimbadKindWithoutAPath(t *testing.T) {
	t.Parallel()

	for otype, want := range map[string]resolve.Kind{
		"*":             resolve.KindStar,
		"G":             resolve.KindGalaxy,
		"Cl*":           resolve.KindStarCluster,
		"As*":           resolve.KindStarCluster,
		"ISM":           resolve.KindNebula,
		"unknown-otype": resolve.KindOther,
		"":              resolve.KindOther,
	} {
		if got := simbadKind(otype, ""); got != want {
			t.Errorf("simbadKind(%q, \"\") = %q, want %q", otype, got, want)
		}
	}
}

func TestParseBrightCSV(t *testing.T) {
	f, err := os.Open("testdata/bright.csv")
	if err != nil {
		t.Fatalf("failed to open test fixture: %v", err)
	}

	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	targets, err := ParseBrightCSV(f)
	if err != nil {
		t.Fatalf("ParseBrightCSV failed: %v", err)
	}

	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}

	// Row order must be preserved (brightest-first, per the ADQL ORDER BY) —
	// unlike ParseCSV, there's no ident-join dedup map to lose it through.
	wantIDs := []string{"* alf CMa", "* alf Car", "* bet Ori"}
	for i, want := range wantIDs {
		if targets[i].ID != want {
			t.Errorf("targets[%d].ID = %q, want %q (order not preserved)", i, targets[i].ID, want)
		}
	}

	sirius := targets[0]
	if !sirius.HasVMag || sirius.VMag != -1.46 {
		t.Errorf("Sirius VMag = %v (HasVMag=%v), want -1.46", sirius.VMag, sirius.HasVMag)
	}

	if sirius.Kind != resolve.KindStar {
		t.Errorf("Sirius Kind = %q, want %q", sirius.Kind, resolve.KindStar)
	}

	// SIMBAD's ICRS positions are at J2000, which is JD 2451545.0 in TT; it
	// was built on the UTC scale, 64 s apart (#611).
	if !sirius.Epoch.Equal(time.J2000()) {
		t.Errorf("Sirius Epoch = %v, want J2000", sirius.Epoch)
	}

	if !sirius.HasCoord || math.Abs(sirius.Coord.RA().Degrees()-101.28715) > 1e-5 {
		t.Errorf("Sirius Coord wrong: HasCoord=%v RA=%v", sirius.HasCoord, sirius.Coord.RA().Degrees())
	}

	if len(sirius.Aliases) != 0 {
		t.Errorf("expected no aliases from a query with no ident join, got %v", sirius.Aliases)
	}
}

func TestSearchBrightMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		data, err := os.ReadFile("testdata/bright.csv")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/csv")

		if _, err := w.Write(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	p := New()

	redirect(t, server.URL)

	var got []resolve.Target

	iter := p.SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 2, Limit: 50})
	iter(func(tgt resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got = append(got, tgt)

		return true
	})

	if len(got) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(got))
	}

	if got[0].ID != "* alf CMa" {
		t.Errorf("expected Sirius first, got %q", got[0].ID)
	}
}

func TestProviderInterface(t *testing.T) {
	p := New()
	if p.Name() != "simbad" {
		t.Errorf("expected simbad, got %s", p.Name())
	}

	caps := p.Capabilities()
	if len(caps) != 2 || caps[0] != resolve.CapObjectResolution || caps[1] != resolve.CapMagnitudeBrowse {
		t.Errorf("expected CapObjectResolution and CapMagnitudeBrowse, got %v", caps)
	}

	// Triggers internal error paths since we didn't mock
	_, _ = p.Resolve(context.Background(), "non_existent_body")
	_, _ = p.Search(context.Background(), "non_existent_body")
}

// redirect points endpoint id at a test server for the duration of one
// test. It replaces the old http.RoundTripper injection: remote/api's
// Client is opaque by design, and every request resolves its URL through
// remote.URL(id) anyway, so the registry is the natural seam.
func redirect(t *testing.T, url string) {
	t.Helper()

	scope := remote.Capture(remote.SIMBAD)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(remote.SIMBAD, url); err != nil {
		t.Fatalf("SetURL(%s): %v", remote.SIMBAD, err)
	}
}

// TestIdentifierVariantsCoversSIMBADPadding pins the spellings the exact
// match depends on, without a network call.
//
// SIMBAD right-justifies a catalogue number in a fixed-width field — "M  31",
// "HD   3969" — and its ADQL cannot normalize (REPLACE, LOWER, ILIKE and
// ivo_nocasematch are all rejected by the live parser). So these variants are
// the whole mechanism: if the padded forms stop being generated, resolution
// silently reverts to matching only what the user typed.
func TestIdentifierVariantsCoversSIMBADPadding(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"M31", []string{"M31", "M 31", "M  31", "M   31", "M    31", "NAME M31"}},
		{"NGC5128", []string{"NGC5128", "NGC 5128", "NGC  5128", "NAME NGC5128"}},
		{"Sirius", []string{"Sirius", "NAME Sirius"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()

			got := identifierVariants(tc.query)

			have := make(map[string]bool, len(got))
			for _, g := range got {
				have[g] = true
			}

			for _, w := range tc.want {
				if !have[w] {
					t.Errorf("variants(%q) is missing %q; got %q", tc.query, w, got)
				}
			}
		})
	}

	if v := identifierVariants("   "); v != nil {
		t.Errorf("a blank query produced %q; it must match nothing rather than everything", v)
	}
}

// TestBuildResolveQueryIsExactNotSubstring is the guard on the defect itself.
//
// The query this replaced was `WHERE ident.id LIKE '%<query>%'`, which for
// "M31" matched 15,843 rows and returned an unordered ten of them — none of
// which was M31. A LIKE reappearing in the identity query would restore that
// silently, since the failure looks like a plausible object rather than an
// error.
func TestBuildResolveQueryIsExactNotSubstring(t *testing.T) {
	t.Parallel()

	q := BuildResolveQuery(resolve.ObjectRequest{Query: "M31", Limit: 10})

	if strings.Contains(q, "LIKE") {
		t.Errorf("the identity query uses LIKE:\n%s", q)
	}

	if !strings.Contains(q, "'M  31'") {
		t.Errorf("the identity query does not offer SIMBAD's own padded spelling:\n%s", q)
	}

	// The subquery is what preserves the alias fan-out: filtering the joined
	// ident directly would return only the identifier that matched.
	if !strings.Contains(q, "SELECT oidref FROM ident WHERE id IN") {
		t.Errorf("the identity query does not select by oid, so aliases would be lost:\n%s", q)
	}

	if BuildResolveQuery(resolve.ObjectRequest{Query: "  "}) != "" {
		t.Error("a blank query produced a query; it must produce none")
	}
}

// TestBuildSearchQueryIsAnchoredAndOrdered keeps the fuzzy path honest.
//
// Search is allowed to be fuzzy — that is its purpose — but not unbounded and
// not unordered. Two identical searches for "M42" used to return different
// objects, because TOP N without ORDER BY is whatever the planner produces.
func TestBuildSearchQueryIsAnchoredAndOrdered(t *testing.T) {
	t.Parallel()

	q := BuildSearchQuery(resolve.ObjectRequest{Query: "M42", Limit: 10})

	if strings.Contains(q, "'%M42%'") {
		t.Errorf("search is still wrapped in wildcards on both sides:\n%s", q)
	}

	if !strings.Contains(q, "LIKE 'M42%'") {
		t.Errorf("search is not anchored at the start of the identifier:\n%s", q)
	}

	if !strings.Contains(q, "ORDER BY") {
		t.Errorf("search has no ORDER BY, so its results are not reproducible:\n%s", q)
	}
}

// TestNoQueryOrdersByAQualifiedColumn holds every query this package sends to
// SIMBAD's rule that ORDER BY takes no table.column: its parser answers
// "Incorrect ADQL query: Encountered '.'" and an HTTP 400. BuildBrightQuery
// learned that against the live service, and BuildSearchQuery broke it with
// basic.main_id as a tie-break, which failed every Search (#705). Offline,
// so it catches the next one without a network.
func TestNoQueryOrdersByAQualifiedColumn(t *testing.T) {
	t.Parallel()

	for name, q := range map[string]string{
		"BuildResolveQuery": BuildResolveQuery(resolve.ObjectRequest{Query: "M42", Limit: 10}),
		"BuildSearchQuery":  BuildSearchQuery(resolve.ObjectRequest{Query: "M42", Limit: 10}),
		"BuildBrightQuery":  BuildBrightQuery(resolve.BrightRequest{MaxVMag: 2, Limit: 50}),
	} {
		_, orderBy, found := strings.Cut(q, "ORDER BY")
		if !found {
			continue
		}

		if strings.Contains(orderBy, ".") {
			t.Errorf("%s orders by a qualified column, which SIMBAD rejects: ORDER BY%s", name, orderBy)
		}
	}
}
