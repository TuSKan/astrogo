package openngc

import (
	"testing"

	"github.com/TuSKan/astrogo/catalog"
)

func TestProvider(t *testing.T) {
	p := New()
	if len(p.targets) == 0 {
		t.Skip("skipping OpenNGC standard tests: openngc.csv dataset is not present")
	}

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
	p := New()
	if len(p.targets) == 0 {
		t.Skip("skipping OpenNGC standard search tests: openngc.csv dataset is not present")
	}
	results := p.Search("orion")
	if len(results) == 0 {
		t.Errorf("Search(%q) returned no results", "orion")
	}
	found := false
	for _, r := range results {
		norm := catalog.Normalize(r.Name)
		if norm == "orionnebula" || norm == "greatorionnebula" || norm == "ngc1976" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Search(%q) did not find Orion Nebula", "orion")
	}
}

func BenchmarkSearch(b *testing.B) {
	p := New()
	if len(p.targets) == 0 {
		b.Skip("skipping OpenNGC standard benchmark: openngc.csv dataset is not present")
	}
	for i := 0; i < b.N; i++ {
		p.Search("nebula")
	}
}

func TestProviderInterface(t *testing.T) {
	p := New()
	if p.Name() != "openngc" {
		t.Errorf("expected openngc, got %s", p.Name())
	}
}
