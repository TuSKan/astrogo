//go:build network

//go test -tags network ./skybrightness/lpmap

// These tests reach the live lightpollutionmap.info QueryRaster service and
// require a free API key in the LIGHTPOLLUTIONMAP_KEY environment variable.
// They skip automatically when the key is absent or the endpoint is unreachable
// (DNS failure, firewall, transient outage) to avoid false-negative CI failures.
package lpmap

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/skybrightness"
)

// requireService skips when no API key is configured or the QueryRaster
// endpoint cannot be reached.
func requireService(t *testing.T) {
	t.Helper()

	if os.Getenv(apiKeyEnv) == "" {
		t.Skipf("%s not set, skipping live light-pollution test", apiKeyEnv)
	}

	conn, err := net.DialTimeout("tcp", "www.lightpollutionmap.info:443", 5*time.Second)
	if err != nil {
		t.Skipf("lightpollutionmap.info unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
}

// TestSQMSaoPaulo verifies that querying central São Paulo — one of the largest
// cities on Earth — returns a plausibly bright urban sky. This is an empirical
// guard on the artificial-brightness unit assumption (mcd/m²): a unit error of
// 10³ would push the result far outside this range.
func TestSQMSaoPaulo(t *testing.T) {
	requireService(t)

	c := New()

	sqm, err := c.SQM(context.Background(), -23.5505, -46.6333)
	if err != nil {
		t.Fatalf("SQM(São Paulo): %v", err)
	}

	t.Logf("São Paulo zenith SQM = %.2f V mag/arcsec²", float64(sqm))

	if sqm < 15 || sqm > 20 {
		t.Errorf("São Paulo SQM = %.2f outside plausible urban range [15,20]", float64(sqm))
	}
}

// TestSQMDarkVsCity verifies the model orders a remote dark site darker than a
// major city (larger SQM magnitude). The dark point is in the Atacama region.
func TestSQMDarkVsCity(t *testing.T) {
	requireService(t)

	ctx := context.Background()
	c := New()

	city, err := c.SQM(ctx, -23.5505, -46.6333) // São Paulo
	if err != nil {
		t.Fatalf("SQM(city): %v", err)
	}

	dark, err := c.SQM(ctx, -24.6275, -70.4044) // Cerro Paranal
	if err != nil {
		t.Fatalf("SQM(dark): %v", err)
	}

	t.Logf("city=%.2f dark=%.2f", float64(city), float64(dark))

	if dark <= city {
		t.Errorf("expected dark site darker (larger SQM) than city: dark=%.2f city=%.2f", float64(dark), float64(city))
	}
}

// TestFloorWA2015_MatchesFrozenReference is a live regression check against
// the default "wa_2015" layer, which — because the World Atlas 2015 archive
// is DOI-frozen and never revised — should return the same value every time
// this test runs. The reference artificial-brightness values (mcd/m²) were
// live-verified against the real API on 2026-08-06 and cross-checked against
// this repo's own downloaded-and-decoded World Atlas GeoTIFF for the same
// coordinates (skybrightness/atlas.LayerWorldAtlas), which agreed within ~1%
// — this test exists to catch a future regression in either this client's
// unit handling or an unexpected upstream change to a source that is
// supposed to never change, not to re-derive the reference values from
// scratch each run. Tolerance is generous (20%) to absorb minor
// interpolation/rounding differences, not to hide a real unit-scale bug — a
// unit-scale regression (e.g. the historical mcd/m² vs. raw-radiance mixup
// this package's WithLayer dispatch exists to prevent) would be off by
// orders of magnitude and fail this bound easily.
func TestFloorWA2015_MatchesFrozenReference(t *testing.T) {
	requireService(t)

	ctx := context.Background()
	c := New() // default layer: wa_2015

	cases := []struct {
		name           string
		lat, lon       float64
		wantArtificial float64 // mcd/m², live-verified 2026-08-06
	}{
		{"Sao Paulo (centre)", -23.5505, -46.6333, 10.44},
		{"Paranal (VLT)", -24.6275, -70.4044, 0.0002149},
	}

	for _, c2 := range cases {
		total, err := c.SQM(ctx, c2.lat, c2.lon)
		if err != nil {
			t.Fatalf("SQM(%s): %v", c2.name, err)
		}

		artificial := total.McdM2() - skybrightness.NaturalZenithMcdM2
		if artificial < 0 {
			artificial = 0
		}

		t.Logf("%s: artificial = %.4g mcd/m² (want ~%.4g)", c2.name, artificial, c2.wantArtificial)

		lo, hi := c2.wantArtificial*0.8, c2.wantArtificial*1.2
		if artificial < lo || artificial > hi {
			t.Errorf("%s: artificial = %.4g mcd/m², want within 20%% of %.4g (frozen World Atlas 2015 reference)",
				c2.name, artificial, c2.wantArtificial)
		}
	}
}

// TestSQMViirs2025_SaoPauloBrighterThanDarkSite is TestSQMDarkVsCity's
// counterpart for the "viirs_<year>" layer family — the raw-radiance unit
// path WithLayer's dispatch logic exists for (see
// TestRadianceLayerUnitDispatch's offline synthetic-server coverage). This
// exercises the identical dispatch against the real live API and a real
// current-year composite, not a fixture.
func TestSQMViirs2025_SaoPauloBrighterThanDarkSite(t *testing.T) {
	requireService(t)

	ctx := context.Background()
	c := New(WithLayer(VIIRSLayer(2025)))

	city, err := c.SQM(ctx, -23.5505, -46.6333) // São Paulo
	if err != nil {
		t.Fatalf("SQM(city): %v", err)
	}

	dark, err := c.SQM(ctx, -24.6275, -70.4044) // Cerro Paranal
	if err != nil {
		t.Fatalf("SQM(dark): %v", err)
	}

	t.Logf("viirs_2025: city=%.2f dark=%.2f", float64(city), float64(dark))

	if dark <= city {
		t.Errorf("expected dark site darker (larger SQM) than city on viirs_2025: dark=%.2f city=%.2f", float64(dark), float64(city))
	}
}
