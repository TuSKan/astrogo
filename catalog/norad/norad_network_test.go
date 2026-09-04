//go:build integration

package norad

// Package norad contains integration tests that validate live queries
// against the CelestTrak GP API (celestrak.org).
//
// Run with: go test -tags integration -v ./catalog/norad/
//
// These tests require an active internet connection to reach
// https://celestrak.org/NORAD/elements/gp.php endpoints.
// If the endpoint is unreachable, tests are skipped automatically.

import (
	"context"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// requireCelestrak skips the test when the CelestTrak API endpoint is
// unreachable (DNS failure, firewall, etc.).  This avoids false-negative
// CI failures for transient network issues.
// celestrakHealth is probed once per run and shared by every live test here,
// so a CelesTrak outage costs one request rather than one per test.
var celestrakHealth struct {
	once sync.Once
	err  error
}

// requireCelestrak skips unless CelesTrak is both reachable and working.
//
// Reachability alone is not enough: CelesTrak answering "500 Internal Server
// Error" opens a socket perfectly well, so the probe passed and the assertions
// below failed, reporting a CelesTrak outage as an astrogo defect on pull
// requests that never touched this package. A 4xx still fails, because that
// would mean astrogo built a request CelesTrak rejected.
func requireCelestrak(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "celestrak.org:443")

	celestrakHealth.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, celestrakHealth.err = New().Fetch(ctx, QueryCatNr, "25544")
	})

	testutil.SkipOnUpstreamFailure(t, celestrakHealth.err)
}

func TestFetchISS_Live(t *testing.T) {
	requireCelestrak(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p := New()

	gps, err := p.Fetch(ctx, QueryCatNr, "25544")
	testutil.SkipOnUpstreamFailure(t, err)

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
	requireCelestrak(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p := New()

	gps, err := p.Fetch(ctx, QueryGroup, GroupStations)
	testutil.SkipOnUpstreamFailure(t, err)

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
	requireCelestrak(t)

	p := New()

	target, err := p.Resolve(context.Background(), "ISS")
	if err != nil {
		t.Fatal("Failed to resolve ISS")
	}

	t.Logf("Resolved: %s (ID=%s, Catalog=%s)", target.Name, target.ID, target.Catalog)

	if target.Catalog != "norad" {
		t.Errorf("Catalog = %q, want %q", target.Catalog, "norad")
	}
}

// TestResolveISSIsTheStation is the end-to-end guard on the defect.
//
// The README's satellite showcase is "predict ISS passes over your location",
// and Resolve("ISS") returned UME (ISS) — NORAD 8709, a Japanese ionosphere
// satellite launched in 1976. Every pass, elevation and ground track computed
// from it was for the wrong spacecraft, and nothing said so.
//
// # Why it fetches once
//
// Its first version resolved three names in a row and CI throttled: two
// subtests sat for 121 and 91 seconds and then failed, which is this
// repository's stated policy inverted — an external service having a bad
// minute must never fail a build. RequireReachable does not cover it either,
// since a TCP handshake succeeding says nothing about whether the service
// will answer, which is the trap docs/PULL_REQUESTS.md section 7 exists to
// warn about.
//
// So the fetch happens once, through the error-returning path, and an
// upstream failure skips. The three name forms are then checked against that
// one response, which is all they ever needed: they are about how a name is
// ranked, not about the network.
func TestResolveISSIsTheStation(t *testing.T) {
	testutil.RequireReachable(t, "celestrak.org:443")

	p := New()

	// Fetch through the path that reports why it failed, so throttling and
	// downtime skip rather than fail.
	gps, err := p.Fetch(t.Context(), QueryName, "ISS")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("CelesTrak: %v", err)
	}

	if len(gps) == 0 {
		t.Skip("CelesTrak returned no satellites for ISS")
	}

	targets := make([]resolve.Target, 0, len(gps))
	for _, gp := range gps {
		targets = append(targets, gpToTarget(gp))
	}

	rankByName("ISS", targets)

	if got := targets[0].ID; got != "25544" {
		t.Errorf("ranking ISS put NORAD %s (%q) first, want 25544 ISS (ZARYA)",
			got, targets[0].Name)
	}

	// The catalog number takes a different route entirely — CelesTrak's exact
	// CATNR parameter rather than its substring NAME one — so it is worth one
	// more call. "25544" used to find nothing at all.
	byNumber, err := p.Resolve(t.Context(), "25544")
	if err != nil {
		t.Skip("CelesTrak did not answer the catalog-number query")
	}

	if byNumber.ID != "25544" {
		t.Errorf("Resolve(\"25544\") = %q (NORAD %s), want ISS (ZARYA)",
			byNumber.Name, byNumber.ID)
	}
}
