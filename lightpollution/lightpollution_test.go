package lightpollution

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
)

// TestArtificialToSQM verifies the artificial-brightness → total SQM conversion
// at known anchor points. Zero artificial brightness must reproduce the natural
// zenith background (22.0 V mag/arcsec²), and a moderately bright urban value
// must map to a plausibly brighter (smaller) SQM.
func TestArtificialToSQM(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		art  float64 // mcd/m²
		want float64 // V mag/arcsec²
		tol  float64
	}{
		{"natural-only", 0, 22.0, 1e-6},
		{"urban", 6.64, 18.0, 0.05},
		{"negative-clamped", -5, 22.0, 1e-6},
	}

	for _, c := range cases {
		got := float64(artificialToSQM(c.art))
		testutil.AssertNear(t, c.name, got, c.want, c.tol)
	}
}

// TestArtificialToSQMMonotonic verifies that brighter artificial light yields a
// smaller (brighter) SQM magnitude.
func TestArtificialToSQMMonotonic(t *testing.T) {
	t.Parallel()

	prev := float64(artificialToSQM(0))

	for _, art := range []float64{0.1, 1, 10, 100, 1000} {
		got := float64(artificialToSQM(art))
		if got >= prev {
			t.Errorf("artificialToSQM(%g) = %g not brighter than previous %g", art, got, prev)
		}

		prev = got
	}
}

// TestParseBrightness covers the CSV point-query response shapes.
func TestParseBrightness(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		body    string
		want    float64
		wantErr bool
	}{
		{"single", "6.64", 6.64, false},
		{"lon-lat-value", "-46.6333,-23.5505,6.64", 6.64, false},
		{"trailing-newline", "-46.6333,-23.5505,6.64\n", 6.64, false},
		{"semicolon", "-46.6333;-23.5505;12.5", 12.5, false},
		{"no-data", "no data", 0, true},
		{"empty", "", 0, true},
	}

	for _, c := range cases {
		got, err := parseBrightness(c.body)
		if c.wantErr {
			if !errors.Is(err, ErrBadResponse) {
				t.Errorf("parseBrightness(%q): expected ErrBadResponse, got %v", c.body, err)
			}

			continue
		}

		if err != nil {
			t.Errorf("parseBrightness(%q): unexpected error %v", c.body, err)

			continue
		}

		testutil.AssertNear(t, c.name, got, c.want, 1e-9)
	}
}

// TestSQMNoAPIKey verifies that a client without an API key fails fast with
// ErrNoAPIKey and never touches the network.
func TestSQMNoAPIKey(t *testing.T) {
	t.Parallel()

	c := New(WithAPIKey(""))

	if _, err := c.SQM(context.Background(), -23.5505, -46.6333); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("SQM without key: expected ErrNoAPIKey, got %v", err)
	}

	if _, err := c.Floor(context.Background(), -23.5505, -46.6333); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("Floor without key: expected ErrNoAPIKey, got %v", err)
	}
}

// TestFloorIsArtificialOnly is a regression test: Floor used to build its
// skybrightness.Floor from SQM's TOTAL (artificial+natural) brightness,
// which silently double-counts the natural background when composed with
// Airglow/Zodiacal/Moonlight in a skybrightness.CompositeModel — the exact
// idiomatic composition pattern skybrightness/model_test.go demonstrates.
// Floor must instead match skybrightness/atlas's artificial-only contract.
func TestFloorIsArtificialOnly(t *testing.T) {
	t.Parallel()

	const artMcdM2 = 6.64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprintf(w, "-46.6333,-23.5505,%v", artMcdM2); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	c := New(WithAPIKey("test-key"), WithHTTPClient(server.Client()))
	c.baseURL = server.URL

	floor, err := c.Floor(context.Background(), -23.5505, -46.6333)
	testutil.AssertNoError(t, err)

	radiance, err := floor.Radiance(coord.AltAz{}, nil)
	testutil.AssertNoError(t, err)

	gotSB := float64(radiance.SurfaceBrightnessV())
	wantArtificialOnly := float64(skybrightness.SurfaceBrightnessFromMcdM2(artMcdM2))

	// Note: at this brightness level the artificial-only and total (SQM)
	// values happen to be numerically close (~0.03 mag apart) even though
	// they're conceptually different quantities — so this test checks an
	// exact match against the correct (artificial-only) formula rather than
	// asserting a "must differ from total" bound, which would be fragile.
	testutil.AssertNear(t, "Floor SB (artificial-only)", gotSB, wantArtificialOnly, 1e-9)
}
