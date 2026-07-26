package openngc

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// newTestProvider builds a Provider directly from a fixed set of targets,
// exercising Resolve/Search logic without any network access — New()'s
// fetch pipeline is covered separately in fetch_test.go.
func newTestProvider() *Provider {
	targets := []resolve.Target{
		{ID: "NGC1976", Name: "Orion Nebula", Kind: resolve.KindNebula, Catalog: "openngc", Aliases: []string{"M42", "M 42", "Messier 42"}, VMag: 4.0, HasVMag: true},
		{ID: "NGC224", Name: "Andromeda Galaxy", Kind: resolve.KindGalaxy, Catalog: "openngc", Aliases: []string{"M31", "M 31", "Messier 31"}, VMag: 3.4, HasVMag: true},
		{ID: "NGC9999", Name: "Faint Test Object", Kind: resolve.KindNebula, Catalog: "openngc"},
	}

	p := &Provider{targets: targets, byKey: make(map[string]int)}

	for i, t := range targets {
		p.byKey[resolve.Normalize(t.ID)] = i
		if t.Name != "" {
			p.byKey[resolve.Normalize(t.Name)] = i
		}

		for _, a := range t.Aliases {
			p.byKey[resolve.Normalize(a)] = i
		}
	}

	return p
}

func TestProvider(t *testing.T) {
	p := newTestProvider()

	tests := []struct {
		query  string
		wantID string
		found  bool
	}{
		{"M42", "NGC1976", true},
		{"NGC 1976", "NGC1976", true},
		{"Orion Nebula", "NGC1976", true},
		{"m31", "NGC224", true},
		{"m 31", "NGC224", true},
		{"nonexistent", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got, ok := p.Resolve(context.Background(), tt.query)
			if ok != tt.found {
				t.Errorf("Resolve(%q) ok = %v, want %v", tt.query, ok, tt.found)
				return
			}

			if ok && got.ID != tt.wantID {
				t.Errorf("Resolve(%q) got ID = %v, want %v", tt.query, got.ID, tt.wantID)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	p := newTestProvider()

	results := p.Search(context.Background(), "orion")
	if len(results) == 0 {
		t.Fatalf("Search(%q) returned no results", "orion")
	}

	found := false

	for _, r := range results {
		if resolve.Normalize(r.Name) == "orionnebula" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Search(%q) did not find Orion Nebula", "orion")
	}
}

func BenchmarkSearch(b *testing.B) {
	p := newTestProvider()

	for range b.N {
		p.Search(context.Background(), "nebula")
	}
}

func TestProviderInterface(t *testing.T) {
	p := New()
	if p.Name() != "openngc" {
		t.Errorf("expected openngc, got %s", p.Name())
	}

	caps := p.Capabilities()
	if len(caps) != 2 || caps[0] != resolve.CapObjectResolution || caps[1] != resolve.CapMagnitudeBrowse {
		t.Errorf("expected CapObjectResolution and CapMagnitudeBrowse, got %v", caps)
	}
}

func TestSearchBright(t *testing.T) {
	p := newTestProvider()

	var got []resolve.Target

	iter := p.SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 5})
	iter(func(t resolve.Target, err error) bool {
		if err != nil {
			return false
		}

		got = append(got, t)

		return true
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 targets brighter than mag 5, got %d: %v", len(got), got)
	}

	// Brightest-first: Andromeda Galaxy (3.4) before Orion Nebula (4.0).
	if got[0].ID != "NGC224" || got[1].ID != "NGC1976" {
		t.Errorf("expected [NGC224, NGC1976] in brightest-first order, got [%s, %s]", got[0].ID, got[1].ID)
	}

	for _, tgt := range got {
		if tgt.ID == "NGC9999" {
			t.Error("expected the HasVMag=false fixture to be excluded, not just below threshold")
		}
	}
}

func TestSearchBright_RespectsLimit(t *testing.T) {
	p := newTestProvider()

	var got []resolve.Target

	iter := p.SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 5, Limit: 1})
	iter(func(t resolve.Target, _ error) bool {
		got = append(got, t)
		return true
	})

	if len(got) != 1 {
		t.Fatalf("expected Limit=1 to cap results at 1, got %d", len(got))
	}

	if got[0].ID != "NGC224" {
		t.Errorf("expected the single brightest result (NGC224), got %s", got[0].ID)
	}
}
