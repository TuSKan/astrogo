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

// ErrNoPublishedMap is returned when no published map matches a request.
var ErrNoPublishedMap = errors.New("starlight: no published map for that order, band and cut")

// published lists the maps astrogo releases, keyed by their cache name.
//
// Each is its own build. A cut map cannot be derived from an uncut one —
// removing bright stars from an already-summed aggregate is impossible, the
// information is gone — so every cut published costs its own aggregation run
// and its own asset.
//
// Two are published rather than one because there is no universally correct
// cut, and baking this project's judgement into the only available file would
// undo the reason [GaiaBuild.FainterThan] is required in the first place.
var published = map[string]GaiaBuild{
	"starmap-o8-V-g6.txt.gz": {
		Order:       8,
		FainterThan: 6,
		Bands:       []GaiaBand{GaiaJohnsonV()},
	},
	"starmap-o8-V-all.txt.gz": {
		Order:       8,
		FainterThan: NoMagnitudeCut,
		Bands:       []GaiaBand{GaiaJohnsonV()},
	},
}

// Open fetches a published integrated-starlight map.
//
// This is the path a caller should normally take. It costs one download of
// about five megabytes, cached afterwards and valid indefinitely because the
// order and the cut are in the filename. [Fetch] queries the archive for named
// directions and is right when a finer grid or a different cut is needed;
// [BuildFromGaia] aggregates a whole sky and is heavy use of a shared service.
//
// The download is consent-gated like every other bulk fetch: call
// [remote.EnableDownloads] with [remote.GaiaStarMap] first, or this fails with
// [remote.ErrDownloadDenied].
//
// spec selects which published map to fetch. Its order, band name and
// magnitude cut must match one that exists, because a map is only meaningful
// against the cut it was built with — see [PublishedMaps] for the list.
func Open(ctx context.Context, spec GaiaBuild) (*Map, error) {
	spec, err := spec.withDefaults()
	if err != nil {
		return nil, err
	}

	if len(spec.Bands) != 1 {
		return nil, fmt.Errorf("%w: one band, got %d", ErrNoPublishedMap, len(spec.Bands))
	}

	name := spec.cacheKey() + ".gz"

	if _, ok := published[name]; !ok {
		return nil, fmt.Errorf("%w: %s; published maps are %s",
			ErrNoPublishedMap, name, strings.Join(PublishedMaps(), ", "))
	}

	bucket, key, err := remote.GetFile(ctx, remote.GaiaStarMap, name)
	if err != nil {
		return nil, fmt.Errorf("starlight: fetch %s: %w", name, err)
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

	// The name promises an order and the content has to keep that promise.
	// A map served under the wrong name is indistinguishable from the right
	// one at every later call site, and would put the sky at the wrong
	// resolution without any value looking out of place.
	if want := pixelsPerOrder(spec.Order); m.Grid().NumPixels() != want {
		return nil, fmt.Errorf("%w: %s holds %d pixels, order %d needs %d",
			ErrNoPublishedMap, name, m.Grid().NumPixels(), spec.Order, want)
	}

	m.Source = name

	return m, nil
}

// PublishedMaps names the maps [Open] can fetch.
func PublishedMaps() []string {
	out := make([]string, 0, len(published))
	for name := range published {
		out = append(out, name)
	}

	return out
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
	fmt.Fprintf(&b, "# cut: %s\n", g.cutDescription())
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
