package catalog

import (
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

type mockProvider struct {
	name    string
	targets map[string]Target
}

func (p *mockProvider) Name() string { return p.name }
func (p *mockProvider) Resolve(query string) (Target, bool) {
	t, ok := p.targets[resolve.Normalize(query)]
	return t, ok
}
func (p *mockProvider) Search(query string) []Target {
	var res []Target
	q := resolve.Normalize(query)
	for _, t := range p.targets {
		if resolve.Normalize(t.Name) == q || resolve.Normalize(t.ID) == q {
			res = append(res, t)
		}
	}
	return res
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
		query   string
		wantID  string
		wantErr error
	}{
		{"M42", "m42", nil},
		{"m 42", "m42", nil},
		{"Messier 42", "m42", nil},
		{"M31", "m31", nil},
		{"M43", "", ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got, err := r.Resolve(tt.query)
			if err != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && got.ID != tt.wantID {
				t.Errorf("Resolve() got ID = %v, want %v", got.ID, tt.wantID)
			}
		})
	}
}

func TestResolver_ProviderPriority(t *testing.T) {
	// When multiple providers resolve the same query,
	// the first provider in priority order wins.
	p1 := &mockProvider{
		name: "p1",
		targets: map[string]Target{
			"target": {ID: "target", Name: "Target 1", Catalog: "p1"},
		},
	}
	p2 := &mockProvider{
		name: "p2",
		targets: map[string]Target{
			"target": {ID: "target", Name: "Target 2", Catalog: "p2"},
		},
	}

	r := &Resolver{providers: []resolve.Provider{p1, p2}}
	got, err := r.Resolve("target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Catalog != "p1" {
		t.Errorf("expected p1 (first provider), got catalog=%s", got.Catalog)
	}
	if got.Name != "Target 1" {
		t.Errorf("expected 'Target 1', got %q", got.Name)
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
	for i := 0; i < b.N; i++ {
		_, _ = r.Resolve("M42")
	}
}
