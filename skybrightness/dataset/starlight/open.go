package starlight

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/TuSKan/astrogo/remote"
)

// ErrNoPublishedMap is returned when the published map is not what it claims.
var ErrNoPublishedMap = errors.New("starlight: the published map does not match its name")

// TotalStarlightMap is the asset astrogo publishes.
//
// "total" rather than "all" because the distinction matters and cost this
// project a wasted build to learn. A Gaia-only map is not the total integrated
// starlight: Gaia cannot observe objects brighter than G = 5, and Masana et al.
// (2021) put those stars at around 20 per cent of the flux, which is why
// GAMBONS reaches the bright end with Hipparcos. A file named "all" that is
// missing a fifth of the light is a promise the data does not keep.
//
// The name therefore records the composition, for the same reason the cut and
// the order are in it: each changes the numbers, and two files that differ by
// twenty per cent must never share a name.
const TotalStarlightMap = "starmap-o8-V-total.txt.gz"

// Open fetches the published integrated-starlight map.
//
// One download of about five megabytes, cached afterwards and valid
// indefinitely because the order, band and composition are all in the filename.
// [Fetch] queries the archive for named directions and is right when a finer
// grid or a magnitude cut is needed; [BuildFromGaia] aggregates a whole sky and
// is heavy use of a shared service.
//
// The map is HEALPix order 8 in Johnson V, matching GAMBONS' own grid — 786,432
// pixels of 1.5979e-5 sr — so the two are directly comparable.
//
// The download is consent-gated like every other bulk fetch: call
// [remote.EnableDownloads] with [remote.GaiaStarMap] first, or this fails with
// [remote.ErrDownloadDenied].
func Open(ctx context.Context) (*Map, error) {
	bucket, key, err := remote.GetFile(ctx, remote.GaiaStarMap, TotalStarlightMap)
	if err != nil {
		return nil, fmt.Errorf("starlight: fetch %s: %w", TotalStarlightMap, err)
	}

	r, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("starlight: open %s: %w", key, err)
	}
	defer func() { _ = r.Close() }()

	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("starlight: decompress %s: %w", key, err)
	}
	defer func() { _ = gz.Close() }()

	m, err := Load(gz, ICRS)
	if err != nil {
		return nil, err
	}

	// The name promises an order and the content has to keep that promise. A
	// map served under the wrong name is indistinguishable from the right one
	// at every later call site, and would put the sky at the wrong resolution
	// without any value looking out of place.
	if want := pixelsPerOrder(GaiaMapOrder); m.Grid().NumPixels() != want {
		return nil, fmt.Errorf("%w: %s holds %d pixels, order %d needs %d",
			ErrNoPublishedMap, TotalStarlightMap, m.Grid().NumPixels(), GaiaMapOrder, want)
	}

	m.Source = TotalStarlightMap

	return m, nil
}

// Header renders the provenance block that belongs at the top of a published
// map, as comment lines.
//
// A hosted artifact that cannot say how it was made is an unattributable
// number. Every input that changes the values is named: the catalogue release,
// the grid, the magnitude cut, the colour transformation and both zero points
// the band conversion rests on. Someone holding only the file must be able to
// tell whether it answers their question.
func (g GaiaBuild) Header() string {
	var b strings.Builder

	band := "unknown"
	if len(g.Bands) > 0 {
		band = g.Bands[0].Name
	}

	fmt.Fprintf(&b, "# bands: %s\n", band)
	fmt.Fprintf(&b, "# catalogue: gaiadr3.gaia_source\n")
	fmt.Fprintf(&b, "# grid: HEALPix order %d, NESTED, ICRS\n", g.Order)
	fmt.Fprintf(&b, "# composition: every source Gaia observes; Gaia sees nothing brighter than G = 5\n")
	fmt.Fprintf(&b, "# quantity: passband-averaged spectral radiance, W m^-2 sr^-1 nm^-1\n")

	if len(g.Bands) > 0 && len(g.Bands[0].ColourTerm) > 0 {
		fmt.Fprintf(&b, "# colour transformation: G - %s polynomial in BP-RP, "+
			"Riello et al. (2021), Gaia DR3 photometry, Table 5.7\n", band)
		fmt.Fprintf(&b, "# zero points: Gaia DR3 G VEGAMAG 25.6874; Johnson V 3.63e-11 W m^-2 nm^-1\n")
		fmt.Fprintf(&b, "# excluded: sources with no BP-RP, which the transformation cannot reach\n")
	}

	return b.String()
}

// WriteMap writes a map in the published format, provenance header first.
func WriteMap(w io.Writer, spec GaiaBuild, m *Map) error {
	band := spec.Bands[0].Name

	if _, err := io.WriteString(w, spec.Header()); err != nil {
		return fmt.Errorf("starlight: write header: %w", err)
	}

	for pixel := range m.Grid().NumPixels() {
		v, err := m.Pixel(band, pixel)
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintf(w, "%d %.6e\n", pixel, v); err != nil {
			return fmt.Errorf("starlight: write pixel %d: %w", pixel, err)
		}
	}

	return nil
}
