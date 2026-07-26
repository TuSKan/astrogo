package simbad

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
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
	p.client.HTTPClient.Transport = &mockTransport{
		Handler: server.Config.Handler,
	}

	tgt, ok := p.Resolve(context.Background(), "m31")
	if !ok {
		t.Fatalf("failed to resolve target")
	}

	if tgt.ID != "NAME M  31" {
		t.Errorf("unexpected ID: %s", tgt.ID)
	}
}

type mockTransport struct {
	Handler http.Handler
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	// Mock executing the request.
	m.Handler.ServeHTTP(rec, req)
	resp := rec.Result()
	// Set the dummy Request so it doesn't fail downstream context tracking
	resp.Request = req

	return resp, nil
}

func TestRetryTimeout(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++

		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	p := New()
	p.client.HTTPClient.Timeout = 100 * time.Millisecond
	p.client.UserAgent = "TestUserAgent"

	// Create mock transport directly to our server
	p.client.HTTPClient.Transport = &mockTransport{
		Handler: server.Config.Handler,
	}

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

// TestMapSimbadKind_RealBrightStarOtypes is a regression test for a real
// bug found via live testing: the original switch only recognized "Star",
// "V*", and "Em*" as stellar, mapping every other real SIMBAD otype to
// KindOther — live SearchBright calls showed ordinary bright stars
// (Sirius, Canopus, Vega, Alpha Centauri, ...) actually come back as
// "SB*"/"PM*"/"dS*"/"RG*"/"s*b", none of which matched, so nearly every
// real star was mislabeled. SIMBAD's own OTYPES nomenclature ends every
// single-star classification in "*"; "**" (double/multiple star system)
// is the one documented exception, which maps to KindDoubleStar instead.
func TestMapSimbadKind_RealBrightStarOtypes(t *testing.T) {
	tests := []struct {
		otype string
		want  resolve.Kind
	}{
		{"SB*", resolve.KindStar},  // Sirius, Canopus
		{"PM*", resolve.KindStar},  // high proper-motion star
		{"dS*", resolve.KindStar},  // delta Scuti variable (Vega)
		{"RG*", resolve.KindStar},  // red giant (Arcturus)
		{"s*b", resolve.KindStar},  // blue supergiant (Rigel)
		{"s*r", resolve.KindStar},  // red supergiant (Betelgeuse)
		{"WD*", resolve.KindStar},  // white dwarf
		{"V*", resolve.KindStar},   // variable star
		{"Em*", resolve.KindStar},  // emission-line star
		{"Star", resolve.KindStar}, // generic
		{"**", resolve.KindDoubleStar},
		{"G", resolve.KindGalaxy},
		{"PN", resolve.KindNebula},
		{"unknown-otype", resolve.KindOther},
	}

	for _, tt := range tests {
		t.Run(tt.otype, func(t *testing.T) {
			if got := mapSimbadKind(tt.otype); got != tt.want {
				t.Errorf("mapSimbadKind(%q) = %q, want %q", tt.otype, got, tt.want)
			}
		})
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
	p.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

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
	_ = p.Search(context.Background(), "non_existent_body")
}
