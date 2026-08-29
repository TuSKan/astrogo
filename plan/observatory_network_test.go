//go:build network

package plan

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// requireGeocoding skips when either service the address lookup needs is
// unreachable. NewSiteEarthAddress calls two: Nominatim for coordinates
// and Open-Elevation for height, so checking only the first leaves the
// test failing on the second's downtime.
func requireGeocoding(t *testing.T) {
	t.Helper()

	for _, host := range []string{"nominatim.openstreetmap.org:443", "api.open-elevation.com:443"} {
		testutil.RequireReachable(t, host)
	}
}

func TestNewSiteEarthAddress_Live(t *testing.T) {
	requireGeocoding(t)

	site, err := NewSiteEarthAddress(context.Background(), "Greenwich", "Royal Observatory, Greenwich, London, UK")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
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
