package fits

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/provider"
	"github.com/TuSKan/astrogo/coord"
)

func TestNew_FileNotFound(t *testing.T) {
	// Provide a non-existent path to verify error bubbling from fits.Open
	_, err := New("non_existent_fake_file.fits")
	if err == nil {
		t.Fatal("expected error on non-existent file, got nil")
	}
}

func TestProvider_ResolveAndSearch(t *testing.T) {
	// Mock a provider directly to test the interface logic,
	// bypassing the physical file extraction which relies on actual FITS file schemas.
	provider := &Provider{
		name: "MockCatalog",
		targets: []provider.Target{
			{ID: "ID-1", Name: "Sirius", Coord: coord.NewICRS(angle.Deg(101.287), angle.Deg(-16.716))},
			{ID: "ID-2", Name: "Vega", Coord: coord.NewICRS(angle.Deg(279.234), angle.Deg(38.783))},
		},
	}

	// Test Interface Name wrapper
	if provider.Name() != "MockCatalog" {
		t.Errorf("expected MockCatalog, got %s", provider.Name())
	}

	// Test Resolver (perfect match mapping)
	tgt, found := provider.Resolve("SIRIUS")
	if !found {
		t.Fatal("expected to find Sirius")
	}
	if tgt.ID != "ID-1" {
		t.Errorf("expected ID-1, got %s", tgt.ID)
	}

	tgt, found = provider.Resolve("unknown")
	if found {
		t.Errorf("expected not to find unknown provider.Target, got %s", tgt.Name)
	}

	// Test Search (partial match mapping for substring discovery)
	results := provider.Search("Siri")
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(results))
	}
	if results[0].Name != "Sirius" {
		t.Errorf("expected Sirius, got %s", results[0].Name)
	}

	results = provider.Search("ID-")
	if len(results) != 2 {
		t.Fatalf("expected 2 search results discovering common substring ID-, got %d", len(results))
	}
}
