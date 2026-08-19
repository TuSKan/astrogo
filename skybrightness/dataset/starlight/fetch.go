package starlight

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/remote/file"
)

// ErrFetchSpec is returned when a fetch is described incompletely.
var ErrFetchSpec = errors.New("starlight: incomplete fetch specification")

// fetchBatch is how many pixels one query asks for.
//
// The archive's cost is dominated by its own overhead and the index scan
// rather than by how many pixels are requested: measured at order 8, one pixel
// takes about three seconds, a hundred four, and five hundred seven. Past that
// the query text itself grows past what is comfortable to send, and the
// marginal pixel is nearly free anyway, so this is where the batching stops.
const fetchBatch = 400

// Fetch returns a map covering the given directions, querying the archive for
// what is not already cached.
//
// # Why this rather than a whole sky
//
// A full order-8 map is 786,432 pixels and 38 minutes of querying. Nobody
// observes the whole sky at once, and the archive charges almost nothing for
// asking narrowly: a night's target list is one query and about four seconds.
// So the unit of work here is the directions a caller actually intends to look
// at, not the sphere.
//
// The returned map is sparse. Directions outside it read as zero, and
// [github.com/TuSKan/astrogo/skybrightness.IntegratedStarlight] already treats
// that as missing data rather than as a dark sightline — it contributes
// nothing and raises the estimate's UnknownCloud flag. A partial map is a
// first-class input, not a degraded one.
//
// # The cache accumulates
//
// Everything fetched is written back under a key naming the order and band,
// and later calls read it first. A second night on the same
// targets costs nothing, and a regular observer's cache grows into a partial
// map of the sky they actually use rather than the sky they do not.
//
// The archive is only contacted for pixels the cache lacks, so a fully cached
// call performs no network access at all.
func Fetch(ctx context.Context, spec GaiaBuild, directions ...coord.ICRS) (*Map, error) {
	spec, err := spec.withDefaults()
	if err != nil {
		return nil, err
	}

	if len(spec.Bands) != 1 {
		return nil, fmt.Errorf("%w: fetch takes exactly one band, got %d",
			ErrFetchSpec, len(spec.Bands))
	}

	if len(directions) == 0 {
		return nil, fmt.Errorf("%w: no directions", ErrFetchSpec)
	}

	grid, err := coord.NewHEALPix(1 << spec.Order)
	if err != nil {
		return nil, fmt.Errorf("starlight: grid: %w", err)
	}

	npix := grid.NumPixels()
	band := spec.Bands[0].Name

	values := make([]float64, npix)
	counts := make([]int64, npix)

	bucket, prefix, cacheErr := remote.CacheDir(ctx, remote.GaiaTAP)

	key := ""
	if cacheErr == nil {
		key = path.Join(prefix, spec.cacheKey())
		// A cache that cannot be read is not a failure: it means fetching
		// what it would have supplied.
		_ = readCache(ctx, bucket, key, values, counts)
	}

	wanted := wantedPixels(grid, directions, values)
	if len(wanted) == 0 {
		return assembleFetch(spec, values, counts, band)
	}

	client, err := aggregationClient()
	if err != nil {
		return nil, err
	}

	solidAngle := 4 * math.Pi / float64(npix)

	for start := 0; start < len(wanted); start += fetchBatch {
		end := min(start+fetchBatch, len(wanted))

		if err := spec.fetchPixels(ctx, client, wanted[start:end], values, counts, solidAngle); err != nil {
			return nil, err
		}

		if spec.Progress != nil {
			spec.Progress(end, len(wanted))
		}
	}

	if cacheErr == nil {
		// A cache that cannot be written costs the next call its time, not
		// this one its answer.
		_ = writeCache(ctx, bucket, key, band, values, counts)
	}

	return assembleFetch(spec, values, counts, band)
}

// assembleFetch wraps the accumulated values in a Map.
func assembleFetch(spec GaiaBuild, values []float64, _ []int64, band string) (*Map, error) {
	m, err := NewMap(ICRS, map[string][]float64{band: values})
	if err != nil {
		return nil, err
	}

	m.Source = fmt.Sprintf("gaiadr3.gaia_source at HEALPix order %d",
		spec.Order)

	return m, nil
}

// wantedPixels reduces the directions to the distinct pixels not yet held.
func wantedPixels(grid coord.HEALPix, directions []coord.ICRS, have []float64) []int64 {
	seen := make(map[int64]struct{}, len(directions))
	out := make([]int64, 0, len(directions))

	for _, dir := range directions {
		pixel := grid.PixelOf(dir.RA(), dir.Dec())

		if pixel < 0 || pixel >= int64(len(have)) {
			continue
		}

		if have[pixel] > 0 {
			continue // already cached
		}

		if _, dup := seen[pixel]; dup {
			continue
		}

		seen[pixel] = struct{}{}

		out = append(out, pixel)
	}

	// Sorted so the query's ranges ascend, which is how the archive's index
	// wants to walk them, and so a run of adjacent directions stays adjacent.
	slices.Sort(out)

	return out
}

// fetchPixels runs one query for a set of pixels and accumulates it.
func (g GaiaBuild) fetchPixels(
	ctx context.Context,
	client *api.Client,
	pixels []int64,
	values []float64,
	counts []int64,
	solidAngle float64,
) error {
	adql, err := g.adqlForPixels(pixels)
	if err != nil {
		return err
	}

	bands := map[string][]float64{g.Bands[0].Name: values}

	return g.runQuery(ctx, client, adql, bands, counts, solidAngle,
		fmt.Sprintf("pixels %d-%d", pixels[0], pixels[len(pixels)-1]))
}

// adqlForPixels builds the query for an arbitrary set of pixels.
//
// The pixels are not contiguous, so the source_id range is a disjunction of
// one BETWEEN per pixel rather than a single span. That is what lets a caller
// ask about targets scattered across the sky in one request instead of
// aggregating everything between them.
func (g GaiaBuild) adqlForPixels(pixels []int64) (string, error) {
	g, err := g.withDefaults()
	if err != nil {
		return "", err
	}

	if len(pixels) == 0 {
		return "", fmt.Errorf("%w: no pixels", ErrFetchSpec)
	}

	divisor := sourceIDDivisor(g.Order)

	var (
		columns strings.Builder
		ranges  strings.Builder
	)

	for _, b := range g.Bands {
		fmt.Fprintf(&columns, ", %s AS %s", b.expression(), columnName(b.Name))
	}

	for i, pixel := range pixels {
		if i > 0 {
			ranges.WriteString(" OR ")
		}

		fmt.Fprintf(&ranges, "source_id BETWEEN %d AND %d",
			pixel*divisor, (pixel+1)*divisor-1)
	}

	return fmt.Sprintf(
		"SELECT source_id/%d AS hpx, COUNT(*) AS n, COUNT(bp_rp) AS ncolour%s "+
			"FROM gaiadr3.gaia_source "+
			"WHERE (%s) GROUP BY hpx",
		divisor, columns.String(), ranges.String(),
	), nil
}

// cacheKey names the dataset this build produces.
//
// The order and the band are both in it because both change the numbers, and
// two files whose values differ must never share a name.
func (g GaiaBuild) cacheKey() string {
	return fmt.Sprintf("starmap-o%d-%s.txt", g.Order, g.Bands[0].Name)
}

// readCache fills values from a previously written partial map.
//
// It tolerates gaps, which is what separates it from [Load]: a published map
// missing a pixel is malformed, while a cache missing a pixel simply has not
// been asked about it yet.
func readCache(ctx context.Context, bucket *file.Bucket, key string, values []float64, counts []int64) error {
	r, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		return err //nolint:wrapcheck // the caller treats any failure as a cold cache
	}
	defer func() { _ = r.Close() }()

	return parseCache(r, values, counts)
}

// parseCache reads the cache format, skipping anything it cannot use.
//
// A malformed line costs the pixels on it, not the whole file: a truncated
// write should leave the rest of the cache usable rather than forcing a
// re-fetch of everything.
func parseCache(r io.Reader, values []float64, counts []int64) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}

		fields := strings.Fields(text)
		if len(fields) < 2 {
			continue
		}

		pixel, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || pixel < 0 || pixel >= int64(len(values)) {
			continue
		}

		v, err := strconv.ParseFloat(fields[1], 64)
		if err != nil || v <= 0 {
			continue
		}

		values[pixel] = v

		// The source count is optional: a cache written before counts were
		// stored is still usable, it just cannot report the total.
		if counts != nil && len(fields) > 2 {
			if n, err := strconv.ParseInt(fields[2], 10, 64); err == nil && n >= 0 {
				counts[pixel] = n
			}
		}
	}

	return scanner.Err() //nolint:wrapcheck // the caller treats any failure as a cold cache
}

// writeCache stores every pixel held so far.
func writeCache(ctx context.Context, bucket *file.Bucket, key, band string, values []float64, counts []int64) error {
	var buf strings.Builder

	fmt.Fprintf(&buf, "# bands: %s\n# partial map, fetched on demand\n", band)

	// The source count rides alongside each value so a resumed build can still
	// report the total that proves it tiled the sky. Without it, a build that
	// resumed would count only the chunks its final run happened to fetch.
	for pixel, v := range values {
		if v <= 0 {
			continue
		}

		if counts != nil {
			fmt.Fprintf(&buf, "%d %.6e %d\n", pixel, v, counts[pixel])
		} else {
			fmt.Fprintf(&buf, "%d %.6e\n", pixel, v)
		}
	}

	if err := file.Save(ctx, bucket, key, strings.NewReader(buf.String())); err != nil {
		return fmt.Errorf("starlight: write cache %s: %w", key, err)
	}

	return nil
}
