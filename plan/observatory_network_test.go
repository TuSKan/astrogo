//go:build network

package plan

import (
	"context"
	"net"
	"testing"
	"time"
)

func requireNominatim(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "nominatim.openstreetmap.org:443", 5*time.Second)
	if err != nil {
		t.Skipf("Nominatim unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
}

// TestNewSiteEarthAddress_Live confirms a real geocoding + elevation round
// trip against the live Nominatim and Open-Elevation APIs — Greenwich
// Observatory should resolve to approximately its well-known coordinates
// (51.48°N, 0°) and elevation (~45m).
func TestNewSiteEarthAddress_Live(t *testing.T) {
	requireNominatim(t)

	site, err := NewSiteEarthAddress(context.Background(), "Greenwich", "Royal Observatory, Greenwich, London, UK")
	if err != nil {
		t.Fatalf("NewSiteEarthAddress: %v", err)
	}

	if lat := site.Latitude().Degrees(); lat < 51 || lat > 52 {
		t.Errorf("Latitude = %.4f, want ~51.48 (Greenwich)", lat)
	}

	if lon := site.Longitude().Degrees(); lon < -1 || lon > 1 {
		t.Errorf("Longitude = %.4f, want ~0 (Greenwich)", lon)
	}

	if h := site.HeightMeters(); h < 0 || h > 120 {
		t.Errorf("HeightMeters = %.1f, want ~45 (Greenwich Observatory)", h)
	}
}
