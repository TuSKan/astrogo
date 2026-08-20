package starlight_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
)

// publishedSpec is the map astrogo releases with a bright-star cut applied.
func publishedSpec() starlight.GaiaBuild {
	return starlight.GaiaBuild{
		Order: 8,
		Bands: []starlight.GaiaBand{starlight.GaiaJohnsonV()},
	}
}

// tinyMap renders a complete order-1 map in the published format, so the whole
// fetch-decompress-parse path can run without a 5 MB asset.
func tinyMap(t *testing.T) []byte {
	t.Helper()

	spec := starlight.GaiaBuild{
		Order: 1,
		Bands: []starlight.GaiaBand{starlight.GaiaJohnsonV()},
	}

	var plain bytes.Buffer

	plain.WriteString(spec.Header())

	// A whole order-8 sky. It is 15 MB of text that gzips to a few kilobytes
	// because every value is the same, and it is generated in full rather than
	// shortened because Open checks that the content matches the order its name
	// promises — a shorter map is exactly what that check exists to reject.
	for pixel := range 786432 {
		if _, err := fmt.Fprintf(&plain, "%d 1.000000e-09\n", pixel); err != nil {
			t.Fatalf("write row: %v", err)
		}
	}

	var gz bytes.Buffer

	w := gzip.NewWriter(&gz)
	if _, err := w.Write(plain.Bytes()); err != nil {
		t.Fatalf("gzip: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	return gz.Bytes()
}

// The published path, end to end: consent, fetch, decompress, parse.
//
// The asset is served from a local server rather than GitHub, because what is
// under test is this package's handling of it — that it gunzips, that it reads
// the provenance header without choking, and that the frame it assigns is the
// one the map was built in.
func TestOpenFetchesAndParses(t *testing.T) {
	asset := tinyMap(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(asset)
	}))
	defer srv.Close()

	scope := remote.Capture(remote.GaiaStarMap)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(remote.GaiaStarMap, srv.URL+"/"); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	remote.EnableDownloads(16<<20, remote.GaiaStarMap)
	defer remote.DisableDownloads(remote.GaiaStarMap)

	m, err := starlight.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if m.Source != starlight.TotalStarlightMap {
		t.Errorf("Source = %q, want %q", m.Source, starlight.TotalStarlightMap)
	}

	if got := m.Grid().NumPixels(); got != 786432 {
		t.Errorf("map holds %d pixels, want 786432", got)
	}

	if m.Frame() != starlight.ICRS {
		t.Errorf("frame = %v, want ICRS — Gaia indexes HEALPix equatorially", m.Frame())
	}

	view, err := m.Band("V")
	if err != nil {
		t.Fatalf("Band: %v", err)
	}

	if view.Galactic() {
		t.Error("an ICRS map must not report itself as galactic")
	}
}

// The header has to say every input that changes the numbers. A hosted file
// that cannot state its own catalogue, grid, cut and zero points is an
// unattributable number, and the person holding it cannot tell whether it
// answers their question.
func TestPublishedHeaderCarriesProvenance(t *testing.T) {
	t.Parallel()

	header := publishedSpec().Header()

	for _, want := range []string{
		"gaiadr3.gaia_source",        // which catalogue release
		"HEALPix order 8",            // which grid
		"ICRS",                       // which frame
		"Gaia sees nothing brighter", // the limit the instrument itself imposes
		"W m^-2 sr^-1 nm^-1",         // which quantity, per nanometre
		"Table 5.9",                  // the colour transformation's source, by table
		"G - V polynomial in BP-RP",  // and which direction it is tabulated in
		"25.6874",                    // the Gaia G zero point
		"3.63e-11",                   // the Johnson V zero point
		"recovered, not dropped",     // what happens to sources with no BP-RP
		"flux-weighted mean BP-RP",   // and on what basis
	} {
		if !strings.Contains(header, want) {
			t.Errorf("header omits %q:\n%s", want, header)
		}
	}
}
