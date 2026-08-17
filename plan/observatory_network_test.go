//go:build network

package plan

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// requireGeocoding skips when either service the address lookup needs is
// unreachable. NewSiteEarthAddress calls two: Nominatim for coordinates
// and Open-Elevation for height, so checking only the first leaves the
// test failing on the second's downtime.
func requireGeocoding(t *testing.T) {
	t.Helper()

	for _, host := range []string{"nominatim.openstreetmap.org:443", "api.open-elevation.com:443"} {
		conn, err := net.DialTimeout("tcp", host, 5*time.Second)
		if err != nil {
			t.Skipf("%s unreachable, skipping live test: %v", host, err)
		}

		_ = conn.Close()
	}
}

// skipIfUnresponsive turns a timed-out request into a skip. A TCP
// pre-check is not sufficient on its own: these public endpoints routinely
// accept a connection and then fail to answer, which is downtime rather
// than wrong data.
func skipIfUnresponsive(t *testing.T, err error) {
	t.Helper()

	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		t.Skipf("geocoding service accepted the connection but did not answer, skipping: %v", err)
	}
}

// TestNewSiteEarthAddress_Live confirms a real geocoding + elevation round
// trip against the live Nominatim and Open-Elevation APIs — Greenwich
// Observatory should resolve to approximately its well-known coordinates
// (51.48°N, 0°) and elevation (~45m).
func TestNewSiteEarthAddress_Live(t *testing.T) {
	requireGeocoding(t)

	site, err := NewSiteEarthAddress(context.Background(), "Greenwich", "Royal Observatory, Greenwich, London, UK")
	if err != nil {
		skipIfUnresponsive(t, err)
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
