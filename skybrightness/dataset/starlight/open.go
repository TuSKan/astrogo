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
// counterpart, matched positionally with proper motion propagated to J2016.0.
// Seventy of the 74 are brighter than V = 3, which is where Gaia's limit
// actually sits, and they carry 9.4 per cent of the whole-sky flux in B falling
// to 4.7 in I — relatively more in the blue, because the diffuse Gaia
// background is the redder of the two.
//
// Six per cent in V rather than the twenty Masana et al. report, because theirs
// is a DR2 figure: DR2 lacked a counterpart for some 35,000 Hipparcos stars and
// DR3 recovered nearly all of them. The correction shrank because the catalogue
// improved, not because the physics changed.
//
// B, V and I hold all 74. R holds 66: four have no R-I in the Bright Star
// Catalogue, one is fainter than its completeness limit, and three are
// multiples where Hipparcos reports combined light while that catalogue reports
// components, so no colour on offer belongs to the same object. Those eight are
// absent from R rather than carrying an invented value.
//
// The name records the composition, for the same reason the order and bands are
// in it: each changes the numbers, and two files that differ must never share a
// name.
const TotalStarlightMap = "starmap-o8-BVRI-total.txt.gz"

// Open fetches the published integrated-starlight map.
//
// One download of about seventeen megabytes, cached afterwards and valid
// indefinitely because the order, bands and composition are all in the
// filename. [Fetch] queries the archive for named directions and is right when
// a finer grid is needed; [BuildFromGaia] aggregates a whole sky and is heavy
// use of a shared service.
//
// The map is HEALPix order 8 — 786,432 pixels of 1.5979e-5 sr, matching GAMBONS'
// own grid, so the two are directly comparable — in the four Johnson-Cousins
// bands Gaia can reach. There is no U: Gaia's bluest band starts near 330 nm
// and the photometric documentation publishes no G-to-U relation, so a U column
// would be a fit rather than a measurement.
//
// Every band comes back from one call; select one with [Map.Band]. Its V
// reproduces the V-only map published before it to 0.08 per cent, that residual
// being the difference between the passband service's Vega calibration and the
// rounded literal the earlier file rested on.
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

	names := make([]string, 0, len(g.Bands))
	for _, band := range g.Bands {
		names = append(names, band.Name)
	}

	if len(names) == 0 {
		names = append(names, "unknown")
	}

	fmt.Fprintf(&b, "# bands: %s\n", strings.Join(names, " "))
	fmt.Fprintf(&b, "# catalogue: gaiadr3.gaia_source\n")
	fmt.Fprintf(&b, "# grid: HEALPix order %d, NESTED, ICRS\n", g.Order)
	fmt.Fprintf(&b, "# composition: every source Gaia observes; Gaia sees nothing brighter than G = 5\n")
	fmt.Fprintf(&b, "# quantity: passband-averaged spectral radiance, W m^-2 sr^-1 nm^-1\n")

	// Per band, because each column rests on its own colour transformation and
	// its own zero point. A header naming only the first would describe one
	// column and mislabel the rest, which is worse than saying nothing.
	for _, band := range g.Bands {
		if len(band.ColourTerm) == 0 {
			continue
		}

		fmt.Fprintf(&b, "# colour transformation %s: G - %s polynomial in BP-RP, "+
			"Gaia DR3 photometric documentation, Sect. 5.5.1, Table 5.9\n",
			band.Name, band.Name)
		// Both zero points, and the factor they combine into. The factor is
		// what actually scales the data, and the two it came from are what let
		// a reader check it rather than take it.
		fmt.Fprintf(&b, "# zero points %s: Gaia DR3 G VEGAMAG %g; %s %.6g W m^-2 nm^-1\n",
			band.Name, GaiaGZeroPoint, band.Name,
			band.FluxToRadiance*math.Pow(10, GaiaGZeroPoint/2.5))
		fmt.Fprintf(&b, "# flux to radiance %s: %.6e W m^-2 nm^-1 per electron per second\n",
			band.Name, band.FluxToRadiance)
	}

	if len(g.Bands) > 0 && len(g.Bands[0].ColourTerm) > 0 {
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
// One line per pixel: the index, then one radiance per band in the order the
// header names them. A single-band map is the same format with one column,
// which is what the reader has always accepted.
//
// # Regenerating the published asset
//
// The whole sequence is committed code, so the published file can be rebuilt
// rather than remembered:
//
//	var bands []GaiaBand
//	var curves []magnitude.Passband
//
//	for _, name := range []string{"B", "V", "R", "I"} {
//		p, err := passband.Fetch(ctx, "Generic/Bessell."+name)
//		band, err := GaiaJohnsonCousins(name, p)
//		bands, curves = append(bands, band), append(curves, p)
//	}
//
//	build := GaiaBuild{Order: GaiaMapOrder, Bands: bands,
//		Endpoint: remote.GaiaAIPAsync}
//
//	m, _, err := BuildFromGaia(ctx, build)      // one query, about 27 minutes
//	stars, err := FetchBrightStars(ctx, BrightStarLimitV, BrightStarMatchRadius)
//	_, err = AddCousinsR(ctx, stars)            // R needs a second catalogue
//
//	for i, name := range []string{"B", "V", "R", "I"} {
//		err = AddBrightStars(m, name, curves[i], stars)
//	}
//
//	err = WriteMap(gzipWriter, build, m)
//
// gzip the result, name it for the bands it holds, and attach it to the
// release tag [github.com/TuSKan/astrogo/remote.GaiaStarMap] resolves against.
//
// Against a synchronous endpoint [BuildFromGaia] chunks the sky into 787 paced
// queries, because that is what politeness to a shared service costs when the
// whole aggregation cannot be one request. Against an asynchronous one it is a
// single query; the two return the same aggregate, cross-checked to 5e-7.
func WriteMap(w io.Writer, spec GaiaBuild, m *Map) error {
	if len(spec.Bands) == 0 {
		return fmt.Errorf("%w: the build names no bands", ErrGaiaBand)
	}

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

	row := make([]string, 0, len(spec.Bands))

	for pixel := range m.Grid().NumPixels() {
		row = row[:0]

		for _, band := range spec.Bands {
			v, err := m.Pixel(band.Name, pixel)
			if err != nil {
				return err
			}

			row = append(row, fmt.Sprintf("%.6e", v))
		}

		if _, err := fmt.Fprintf(w, "%d %s\n", pixel, strings.Join(row, " ")); err != nil {
			return fmt.Errorf("starlight: write pixel %d: %w", pixel, err)
		}
	}

	return nil
}
