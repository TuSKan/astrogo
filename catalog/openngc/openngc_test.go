package openngc

import (
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// newTestProvider builds a Provider directly from a fixed set of targets,
// exercising Resolve/Search logic without any network access — New()'s
// fetch pipeline is covered separately in fetch_test.go.
func newTestProvider() *Provider {
	targets := []resolve.Target{
		{ID: "NGC1976", Name: "Orion Nebula", Kind: resolve.KindNebula, Catalog: "openngc", Aliases: []string{"M42", "M 42", "Messier 42"}},
		{ID: "NGC224", Name: "Andromeda Galaxy", Kind: resolve.KindGalaxy, Catalog: "openngc", Aliases: []string{"M31", "M 31", "Messier 31"}},
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
			got, ok := p.Resolve(tt.query)
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

	results := p.Search("orion")
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
		p.Search("nebula")
	}
}

func TestProviderInterface(t *testing.T) {
	p := New()
	if p.Name() != "openngc" {
		t.Errorf("expected openngc, got %s", p.Name())
	}
}
