//go:build integration

package norad

import (
	"context"
	"testing"
	"time"
)

func TestFetchISS_Live(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p := New()
	gps, err := p.Fetch(ctx, QueryCatNr, "25544")
	if err != nil {
		t.Fatalf("Failed to fetch ISS data: %v", err)
	}

	if len(gps) == 0 {
		t.Fatal("Expected at least one GP element set for ISS")
	}

	gp := gps[0]
	t.Logf("ISS GP Data:")
	t.Logf("  Name:       %s", gp.ObjectName)
	t.Logf("  ID:         %s", gp.ObjectID)
	t.Logf("  Epoch:      %s", gp.Epoch)
	t.Logf("  Cat Nr:     %d", gp.NoradCatID)
	t.Logf("  Inclination: %.4f°", gp.Inclination)
	t.Logf("  MeanMotion: %.8f rev/day", gp.MeanMotion)
	t.Logf("  Eccentricity: %.7f", gp.Eccentricity)
	t.Logf("  BStar:      %.10f", gp.BStar)

	if gp.NoradCatID != 25544 {
		t.Errorf("NoradCatID = %d, want 25544", gp.NoradCatID)
	}

	// ISS orbit sanity checks.
	if gp.Inclination < 50 || gp.Inclination > 53 {
		t.Errorf("ISS inclination %.2f° outside expected 50-53° range", gp.Inclination)
	}
	if gp.MeanMotion < 15 || gp.MeanMotion > 16 {
		t.Errorf("ISS mean motion %.2f outside expected 15-16 rev/day", gp.MeanMotion)
	}

	// Verify TLE generation.
	line1, line2 := gp.ToTLE()
	t.Logf("  TLE Line 1: %s", line1)
	t.Logf("  TLE Line 2: %s", line2)
}

func TestFetchGroup_Live(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p := New()
	gps, err := p.Fetch(ctx, QueryGroup, GroupStations)
	if err != nil {
		t.Fatalf("Failed to fetch Stations group: %v", err)
	}

	if len(gps) < 2 {
		t.Fatalf("Expected at least 2 stations, got %d", len(gps))
	}

	t.Logf("Fetched %d space stations", len(gps))
	for i, gp := range gps {
		if i >= 5 {
			t.Logf("  ... and %d more", len(gps)-5)
			break
		}
		t.Logf("  [%d] %s (Cat %d)", i, gp.ObjectName, gp.NoradCatID)
	}
}

func TestResolve_Live(t *testing.T) {
	p := New()
	target, ok := p.Resolve("ISS")
	if !ok {
		t.Fatal("Failed to resolve ISS")
	}

	t.Logf("Resolved: %s (ID=%s, Catalog=%s)", target.Name, target.ID, target.Catalog)

	if target.Catalog != "norad" {
		t.Errorf("Catalog = %q, want %q", target.Catalog, "norad")
	}
}
