package starlight

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
)

// Sentinel errors for the Gaia builder.
var (
	// ErrGaiaBand is returned for a band whose description cannot produce a
	// radiance.
	ErrGaiaBand = errors.New("starlight: invalid Gaia band description")

	// ErrGaiaOrder is returned for a HEALPix order outside what source_id
	// can address.
	ErrGaiaOrder = errors.New("starlight: HEALPix order must be between 1 and 12")

	// ErrGaiaResponse is returned when the archive's answer cannot be read.
	ErrGaiaResponse = errors.New("starlight: cannot read the Gaia archive response")
)

// Building an integrated-starlight map from Gaia.
//
// A Gaia source_id encodes the HEALPix index of the source in its high bits:
// source_id / 2^(59-2k) is the level-k nested pixel. That makes the whole
// aggregation a server-side GROUP BY, so the map is built from about 800
// queries rather than the 600 GB the bulk catalogue would cost, and each
// chunk is a source_id range that rides the primary-key index instead of
// scanning the table.
//
// This is data preparation, not evaluation. It performs network I/O, it takes
// tens of minutes, and its output is a [Map] that the sky-brightness engine
// then consumes offline.
const (
	// gaiaSourceIDBits is the 59 in source_id / 2^(59-2k).
	gaiaSourceIDBits = 59

	// defaultGaiaChunk is how many pixels one query aggregates. The archive
	// caps a synchronous result at a few thousand rows, and a thousand
	// returns comfortably inside that.
	defaultGaiaChunk = 1000

	// GaiaMapOrder is the HEALPix order GAMBONS publishes at: order 8, which
	// is nside 256 and 786432 pixels. Note that "resolution 8" in that paper
	// means the order, not nside.
	GaiaMapOrder = 8
)

// GaiaBand describes one output band of the map.
//
// # The photometric transformation is the caller's
//
// This package ships no band transformations and no zero points. Gaia's own
// G, BP and RP are the only photometry the archive holds; every other band is
// a fit, and Masana et al. recomputed theirs specifically for the EDR3 filter
// definitions. Shipping a set here would mean either copying one without
// being able to state which filter revision it belongs to, or inventing it.
//
// The Gaia G band itself needs no transformation, so a zero-value ColourTerm
// is the one case that works out of the box.
type GaiaBand struct {
	// Name labels the band in the resulting map.
	Name string

	// ColourTerm gives G minus the band magnitude as a polynomial in
	// (BP-RP), lowest order first:
	//
	//	G - m_band = c[0] + c[1]*x + c[2]*x^2 + ...
	//
	// Nil means the Gaia G band itself. The transformation is applied per
	// star inside the aggregate, which matters: transforming a summed flux
	// is not the same as summing transformed fluxes when the transformation
	// depends on colour.
	ColourTerm []float64

	// FluxToRadiance converts one unit of the archive's flux — e-/s, as
	// phot_g_mean_flux reports — into W m^-2, before the pixel solid angle
	// is divided out. It carries the band's zero point and the photometric
	// system, neither of which this package can supply.
	FluxToRadiance float64
}

// validate reports whether the band can produce a radiance.
func (b GaiaBand) validate() error {
	if b.Name == "" {
		return fmt.Errorf("%w: no name", ErrGaiaBand)
	}

	if b.FluxToRadiance <= 0 {
		return fmt.Errorf("%w: band %q has no flux-to-radiance factor", ErrGaiaBand, b.Name)
	}

	return nil
}

// expression renders the band's per-star flux as ADQL.
//
// Sources without a BP-RP colour make the polynomial null, so SQL drops them
// from the sum. That is deliberate rather than merely tolerated: the archive's
// ADQL parser rejects both COALESCE and CASE, so there is no way to substitute
// a default colour inside the aggregate, and inventing one outside it would be
// worse than excluding them.
//
// The cost is small and measurable. Colourless sources are the faintest few
// per cent of the catalogue — roughly 1 per cent of a typical pixel's count,
// and less of its flux, since flux is dominated by bright stars. Both counts
// come back per pixel so a caller can see exactly what was dropped.
func (b GaiaBand) expression() string {
	if len(b.ColourTerm) == 0 {
		return "SUM(phot_g_mean_flux)"
	}

	var poly strings.Builder

	for i, c := range b.ColourTerm {
		if i > 0 {
			poly.WriteString("+")
		}

		fmt.Fprintf(&poly, "%g", c)

		for range i {
			poly.WriteString("*bp_rp")
		}
	}

	// flux_band = flux_G * 10^(0.4*(G - m_band)), since a positive G - m
	// means the band magnitude is brighter and so carries more flux.
	return fmt.Sprintf("SUM(phot_g_mean_flux*POWER(10,0.4*(%s)))", poly.String())
}

// GaiaBuild configures a map build.
type GaiaBuild struct {
	// Order is the HEALPix order. Defaults to [GaiaMapOrder].
	Order int

	// Chunk is how many pixels one query aggregates. Defaults to 1000.
	Chunk int

	// Bands are the output bands, at least one.
	Bands []GaiaBand

	// Progress, if set, is called after each chunk.
	Progress func(done, total int)
}

// pixelsPerOrder returns 12*4^order.
func pixelsPerOrder(order int) int64 { return 12 << (2 * order) }

// sourceIDDivisor is the constant that turns a source_id into a level-order
// nested HEALPix index.
func sourceIDDivisor(order int) int64 { return 1 << (gaiaSourceIDBits - 2*order) }

// ADQL renders the aggregation query for one chunk of pixels.
//
// It is exported so a caller can inspect, run or adapt the query without
// going through [BuildFromGaia] — an all-sky build is a long operation, and
// being able to see exactly what is sent matters more here than in a
// request-response API.
func (g GaiaBuild) ADQL(firstPixel, lastPixel int64) (string, error) {
	g, err := g.withDefaults()
	if err != nil {
		return "", err
	}

	divisor := sourceIDDivisor(g.Order)

	var columns strings.Builder

	for _, b := range g.Bands {
		fmt.Fprintf(&columns, ", %s AS %s", b.expression(), columnName(b.Name))
	}

	// Two things here are dictated by what the archive's ADQL parser accepts,
	// both learned by having a query rejected:
	//
	//   - GROUP BY takes the select-list alias, not the expression. Repeating
	//     source_id/N there is a parse error, not merely redundant.
	//   - COUNT(bp_rp) counts the non-null colours, so COUNT(*) minus it gives
	//     the sources a transformed band drops. A CASE expression would say
	//     that more directly and is rejected, as is COALESCE.
	return fmt.Sprintf(
		"SELECT source_id/%d AS hpx, COUNT(*) AS n, COUNT(bp_rp) AS ncolour%s "+
			"FROM gaiadr3.gaia_source "+
			"WHERE source_id BETWEEN %d AND %d GROUP BY hpx",
		divisor, columns.String(),
		firstPixel*divisor, (lastPixel+1)*divisor-1,
	), nil
}

// withDefaults fills in and validates a build.
func (g GaiaBuild) withDefaults() (GaiaBuild, error) {
	if g.Order == 0 {
		g.Order = GaiaMapOrder
	}

	if g.Order < 1 || g.Order > 12 {
		return g, fmt.Errorf("%w: got %d", ErrGaiaOrder, g.Order)
	}

	if g.Chunk <= 0 {
		g.Chunk = defaultGaiaChunk
	}

	if len(g.Bands) == 0 {
		return g, fmt.Errorf("%w: no bands", ErrGaiaBand)
	}

	for _, b := range g.Bands {
		if err := b.validate(); err != nil {
			return g, err
		}
	}

	return g, nil
}

// columnName makes a band name safe as an ADQL identifier.
func columnName(name string) string {
	var b strings.Builder

	b.WriteString("b_")

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}

	return b.String()
}

// BuildFromGaia aggregates integrated starlight over the whole sky from the
// ESA Gaia archive, returning a map in the ICRS frame.
//
// # What this reproduces, and what it does not
//
// It sums the flux of every Gaia source into its HEALPix pixel, applying the
// caller's colour transformation per star. That is the core of the GAMBONS
// method and the part that needs the catalogue.
//
// It does not add the bright stars Gaia omits, which Masana et al. take from
// Hipparcos and which carry disproportionate weight; it drops sources with no
// BP-RP colour from transformed bands rather than imputing one, where Masana
// et al. use the local mean; and it does not add the faint-star completion
// below G = 20 that Masana et al. draw from the Besancon model, worth under
// 3 per cent away from the galactic plane. A map built here is therefore a floor, not a replacement,
// and the per-pixel counts are returned so those gaps can be assessed.
//
// Masana et al. shipped a bug in their own DR2 aggregation that
// underestimated this quantity for months, worst in the Milky Way. Anything
// built here should be checked against their published tool before being
// trusted, which docs/skybrightness.md section 13 describes.
func BuildFromGaia(ctx context.Context, build GaiaBuild) (*Map, []int64, error) {
	build, err := build.withDefaults()
	if err != nil {
		return nil, nil, err
	}

	npix := pixelsPerOrder(build.Order)
	solidAngle := 4 * math.Pi / float64(npix)

	bands := make(map[string][]float64, len(build.Bands))
	for _, b := range build.Bands {
		bands[b.Name] = make([]float64, npix)
	}

	counts := make([]int64, npix)

	client, err := api.NewClient(remote.GaiaTAP)
	if err != nil {
		return nil, nil, fmt.Errorf("starlight: gaia client: %w", err)
	}

	chunks := int((npix + int64(build.Chunk) - 1) / int64(build.Chunk))

	for c := range chunks {
		first := int64(c) * int64(build.Chunk)

		last := first + int64(build.Chunk) - 1
		if last >= npix {
			last = npix - 1
		}

		if err := build.fetchChunk(ctx, client, first, last, bands, counts, solidAngle); err != nil {
			return nil, nil, err
		}

		if build.Progress != nil {
			build.Progress(c+1, chunks)
		}
	}

	m, err := NewMap(ICRS, bands)
	if err != nil {
		return nil, nil, err
	}

	m.Source = fmt.Sprintf("gaiadr3.gaia_source aggregated at HEALPix order %d", build.Order)

	return m, counts, nil
}

// RunChunk aggregates a single range of pixels, for callers who want to
// inspect one chunk rather than build a whole sky — and for the network test
// that proves the archive accepts the query this package generates.
//
// The returned map covers the full sphere at the build's order with only the
// requested pixels populated, so it is a probe rather than a usable sky.
func RunChunk(ctx context.Context, build GaiaBuild, first, last int64) (*Map, []int64, error) {
	build, err := build.withDefaults()
	if err != nil {
		return nil, nil, err
	}

	npix := pixelsPerOrder(build.Order)
	solidAngle := 4 * math.Pi / float64(npix)

	bands := make(map[string][]float64, len(build.Bands))
	for _, b := range build.Bands {
		bands[b.Name] = make([]float64, npix)
	}

	counts := make([]int64, npix)

	client, err := api.NewClient(remote.GaiaTAP)
	if err != nil {
		return nil, nil, fmt.Errorf("starlight: gaia client: %w", err)
	}

	if err := build.fetchChunk(ctx, client, first, last, bands, counts, solidAngle); err != nil {
		return nil, nil, err
	}

	m, err := NewMap(ICRS, bands)
	if err != nil {
		return nil, nil, err
	}

	return m, counts, nil
}

// fetchChunk runs one chunk's query and accumulates it.
func (g GaiaBuild) fetchChunk(
	ctx context.Context,
	client *api.Client,
	first, last int64,
	bands map[string][]float64,
	counts []int64,
	solidAngle float64,
) error {
	adql, err := g.ADQL(first, last)
	if err != nil {
		return err
	}

	v := url.Values{}
	v.Set("REQUEST", "doQuery")
	v.Set("LANG", "ADQL")
	v.Set("FORMAT", "csv")
	v.Set("QUERY", adql)

	body, err := client.PostForm(ctx, remote.GaiaTAP, "", v)
	if err != nil {
		return fmt.Errorf("starlight: pixels %d-%d: %w", first, last, err)
	}
	defer func() { _ = body.Close() }()

	return g.accumulate(body, bands, counts, solidAngle)
}

// accumulate reads one chunk's CSV into the map under construction.
func (g GaiaBuild) accumulate(
	r io.Reader,
	bands map[string][]float64,
	counts []int64,
	solidAngle float64,
) error {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrGaiaResponse, err)
	}

	index := map[string]int{}
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(name))] = i
	}

	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("%w: %w", ErrGaiaResponse, err)
		}

		pixel, err := strconv.ParseInt(strings.TrimSpace(row[index["hpx"]]), 10, 64)
		if err != nil || pixel < 0 || pixel >= int64(len(counts)) {
			return fmt.Errorf("%w: pixel index %q", ErrGaiaResponse, row[index["hpx"]])
		}

		if i, ok := index["n"]; ok {
			counts[pixel], _ = strconv.ParseInt(strings.TrimSpace(row[i]), 10, 64)
		}

		for _, b := range g.Bands {
			// The archive returns its column names lowercased, and the index
			// is built that way, so the lookup has to match.
			i, ok := index[strings.ToLower(columnName(b.Name))]
			if !ok {
				return fmt.Errorf("%w: no column for band %q", ErrGaiaResponse, b.Name)
			}

			flux, err := strconv.ParseFloat(strings.TrimSpace(row[i]), 64)
			if err != nil || flux < 0 {
				continue // a pixel with no usable sources contributes nothing
			}

			bands[b.Name][pixel] = flux * b.FluxToRadiance / solidAngle
		}
	}
}
