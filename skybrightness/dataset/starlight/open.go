package starlight

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/TuSKan/astrogo/remote"
)

// ErrNoPublishedMap is returned when the published map is not what it claims.
var ErrNoPublishedMap = errors.New("starlight: the published map does not match its name")

// TotalStarlightMap is the asset astrogo publishes.
//
// "total" rather than "all", and the map has to earn it. Gaia saturates on the
// brightest sky, so a Gaia-only map is not the total integrated starlight and a
// file named "all" would promise what the data does not hold. This one closes
// that gap the way GAMBONS does, with Hipparcos: 74 stars with no Gaia DR3
// counterpart, matched positionally with proper motion propagated to J2016.0,
// carrying 6.4 per cent of the whole-sky flux. Seventy of the 74 are brighter
// than V = 3, which is where Gaia's limit actually sits.
//
// Six per cent rather than the twenty Masana et al. report, because theirs is a
// DR2 figure: DR2 lacked a counterpart for some 35,000 Hipparcos stars and DR3
// recovered nearly all of them. The correction shrank because the catalogue
// improved, not because the physics changed.
//
// The name records the composition, for the same reason the order and band are
// in it: each changes the numbers, and two files that differ must never share a
// name.
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

	if err := m.plausibleSky(); err != nil {
		return nil, err
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
			"Gaia DR3 photometric documentation, Sect. 5.5.1, Table 5.9\n", band)
		fmt.Fprintf(&b, "# zero points: Gaia DR3 G VEGAMAG 25.6874; Johnson V 3.63e-11 W m^-2 nm^-1\n")
		fmt.Fprintf(&b, "# colourless sources: recovered, not dropped - their G flux is scaled by "+
			"the polynomial at the pixel's flux-weighted mean BP-RP, clamped to [-0.5, 5.0]\n")
	}

	return b.String()
}

// MinPublishedSkyContrast is the smallest ratio between the brightest and
// faintest populated pixel that a published map has to show.
//
// The real order-8 map spans 13.1 to 30.0 mag arcsec^-2, a factor of six
// million, so ten is not a demanding threshold. It exists to catch a file that
// is the right shape and the wrong content.
const MinPublishedSkyContrast = 10

// plausibleSky rejects a map whose pixels carry no structure.
//
// # Why a pixel count is not enough
//
// This check was added after a placeholder fixture, written into the real cache
// directory by an earlier test run, was served by [Open] for weeks. It had the
// right filename and exactly 786,432 rows, so the order check passed; every row
// held 1.000000e-09, and its header claimed order 1 while the content said
// order 8. Downstream nothing looked wrong - integrated starlight simply became
// a constant, plausible, entirely fictional 22.97 mag arcsec^-2 in every
// direction, and it took an end-to-end comparison against another model to
// notice.
//
// A cached object for an immutable endpoint is reused on existence alone, so
// nothing else in the chain would ever have re-fetched it. The content is the
// only thing left that can tell a real sky from a stand-in, and the cheapest
// property that separates them is that a sky has structure.
func (m *Map) plausibleSky() error {
	for _, band := range m.Bands() {
		values, ok := m.bands[band]
		if !ok {
			continue
		}

		lo, hi := math.Inf(1), 0.0

		for _, v := range values {
			if v <= 0 {
				continue
			}

			lo = math.Min(lo, v)
			hi = math.Max(hi, v)
		}

		if math.IsInf(lo, 1) {
			return fmt.Errorf("%w: band %q has no positive pixel", ErrNoPublishedMap, band)
		}

		if hi/lo < MinPublishedSkyContrast {
			return fmt.Errorf("%w: band %q spans only %.3g to %.3g, a factor of %.2f; "+
				"a real sky spans millions, so this is a placeholder rather than a map",
				ErrNoPublishedMap, band, lo, hi, hi/lo)
		}
	}

	return nil
}

// WriteMap writes a map in the published format, provenance header first.
//
// # Regenerating the published asset
//
// This is the last step of four, and the whole sequence is committed code so
// the published file can be rebuilt rather than remembered:
//
//	build := GaiaBuild{Order: GaiaMapOrder, Bands: []GaiaBand{GaiaJohnsonV()}}
//
//	m, _, err := BuildFromGaia(ctx, build)                  // ~787 queries
//	stars, err := FetchBrightStars(ctx,                     // ~90 seconds
//		BrightStarLimitV, BrightStarMatchRadius)
//	err = AddBrightStars(m, "V", stars)
//	err = WriteMap(gzipWriter, build, m)
//
// gzip the result, name it [TotalStarlightMap], and attach it to the release
// tag [github.com/TuSKan/astrogo/remote.GaiaStarMap] resolves against.
//
// [BuildFromGaia] chunks the sky into 787 paced queries because that is polite
// to a shared service. One whole-sky query returns the same aggregate in about
// thirteen minutes — the two were cross-checked to 5e-7 — but it needs an
// asynchronous job, which this package does not implement; the chunked path is
// what is reproducible from here today.
func WriteMap(w io.Writer, spec GaiaBuild, m *Map) error {
	band := spec.Bands[0].Name

	if _, err := io.WriteString(w, spec.Header()); err != nil {
		return fmt.Errorf("starlight: write header: %w", err)
	}

	// Anything added after the aggregation records itself on the map rather
	// than on the build spec, because the spec describes one query and the map
	// can hold more than that query returned.
	if m.Source != "" {
		if _, err := fmt.Fprintf(w, "# added: %s\n", m.Source); err != nil {
			return fmt.Errorf("starlight: write source: %w", err)
		}
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
