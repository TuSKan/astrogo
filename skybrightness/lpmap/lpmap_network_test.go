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
