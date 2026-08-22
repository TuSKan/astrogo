package starlight

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/magnitude"
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
	// phot_g_mean_flux reports — into the passband-averaged spectral
	// flux density W m^-2 nm^-1, before the pixel solid angle is divided
	// out. It carries the band's zero point and the photometric system,
	// neither of which this package can supply — for Gaia G on the Vega
	// scale that is 3.63e-11 * 10^(-25.6874/2.5).
	//
	// Per nanometre, not integrated over the band: a zero point converts a
	// flux into a mean flux density, and [skybrightness.NewIntegratedStarlight]
	// consumes it as one. Mixing the two conventions changes the answer by
	// the band width and is not otherwise visible.
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

// colourPolynomial renders the G-to-band magnitude offset as an ADQL
// expression in bp_rp, and evaluates it in Go for a given colour.
//
// One definition serves both, because the correction for colourless sources
// has to use exactly the polynomial the query used. Two copies would drift.
func (b GaiaBand) colourPolynomial() string {
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

	return poly.String()
}

// colourValidLo and colourValidHi bound the interval Riello et al. (2021)
// fitted the transformation over.
const (
	colourValidLo = -0.5
	colourValidHi = 5.0
)

// colourFactor evaluates 10^(0.4*(G-band)) for one colour — the same factor
// the query applies per star, for use on the sources the query had to drop.
//
// The colour is clamped to the fitted interval first. This matters here and not
// in the query because the value passed in is a pixel's flux-weighted mean
// colour, and weighting by flux lets one dominant star carry the mean well past
// any individual population: measured over the order-9 sky the mean reaches
// BP-RP = 7.41, where a cubic fitted to 5.0 is no longer describing anything.
// Clamping refuses to extrapolate rather than inventing a value for it, and
// costs 0.0003 per cent of the whole-sky map across the 194 pixels that reach
// outside the interval.
func (b GaiaBand) colourFactor(bpRP float64) float64 {
	bpRP = math.Min(colourValidHi, math.Max(colourValidLo, bpRP))

	var offset, term float64 = 0, 1

	for _, c := range b.ColourTerm {
		offset += c * term
		term *= bpRP
	}

	return math.Pow(10, 0.4*offset)
}

// expression renders the band's per-star flux as ADQL.
//
// Sources without a BP-RP colour make the polynomial null, so SQL drops them
// from the sum. That is not a rounding error: across the whole order-8 build
// 14.95 per cent of sources lack a colour, rising above 50 per cent in the
// densest pixels of the Galactic plane, so the loss is both large and
// direction-dependent — the worst combination, because a deficit that varies
// across the sky cannot be absorbed into an overall calibration.
//
// They are recovered rather than excluded. The query returns two further
// sums — the total G flux and the G flux of coloured sources alone — whose
// difference is the flux the polynomial dropped, plus the mean colour of the
// pixel. [GaiaBuild.accumulate] then assigns that flux the pixel's own mean
// colour, which is what Masana et al. (2021) do. The counts still come back
// per pixel so a caller can see how much of a pixel rests on the assumption.
func (b GaiaBand) expression() string {
	if len(b.ColourTerm) == 0 {
		return "SUM(phot_g_mean_flux)"
	}

	return fmt.Sprintf("SUM(phot_g_mean_flux*POWER(10,0.4*(%s)))", b.colourPolynomial())
}

// colourRecoveryColumns renders the extra aggregates that make the colourless
// sources recoverable.
//
// The trick is NULL propagation rather than CASE or FILTER, both of which one
// archive or the other rejects: adding 0*bp_rp to a flux makes the whole term
// null exactly when the colour is missing, so the sum covers coloured sources
// alone. Subtracting it from the unconditional sum leaves the dropped flux.
// Plain arithmetic like this parses everywhere, which CASE and FILTER do not —
// ESA rejects CASE, and Gaia@AIP rejects FILTER.
func (b GaiaBand) colourRecoveryColumns() string {
	if len(b.ColourTerm) == 0 {
		return ""
	}

	// The mean colour is weighted by flux, not by count.
	//
	// AVG(bp_rp) is dominated by the numerous faint red stars, while the flux
	// being recovered is dominated by bright ones, which are systematically
	// bluer. Measured on the worst pixel in the sky, the count-weighted mean is
	// 1.452 against a flux-weighted 0.924, and using the former over-corrects
	// by 19 per cent. Since what is being scaled is flux, the colour has to
	// represent the light rather than the population.
	name := columnName(b.Name)

	return fmt.Sprintf(", SUM(phot_g_mean_flux) AS %s_all, "+
		"SUM(phot_g_mean_flux+0*bp_rp) AS %s_col, "+
		"SUM(phot_g_mean_flux*bp_rp)/SUM(phot_g_mean_flux+0*bp_rp) AS %s_mc",
		name, name, name)
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
		fmt.Fprintf(&columns, ", %s AS %s%s", b.expression(), columnName(b.Name),
			b.colourRecoveryColumns())
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
//
// Prefer [Fetch] unless a whole sky is genuinely needed.
//
// # This is heavy use of a shared service
//
// A build at order 8 sends 787 queries over about forty minutes to a free,
// anonymous research service. Doing that repeatedly is enough to stop being
// served: after three builds and some probing in one afternoon, submissions to
// the query endpoint returned nothing at all — not an error, silence — while
// ordinary GETs to the same host kept answering. A malformed query that should
// have failed at the parser in milliseconds got the same silence, which is how
// a defending service looks from outside and not how a broken one does.
//
// The queries are spaced and serialised (see [aggregationPace]) for that
// reason. If a full sky is what you want rather than the directions you intend
// to observe, consider the bulk release instead: ESA publishes gaia_source as
// files keyed by HEALPix level-8 range, which is this aggregation's own grid,
// and it exists so that heavy users do not have to ask the query service 787
// times. It costs far more bandwidth and far less of somebody else's service.
//
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

	client, err := aggregationClient()
	if err != nil {
		return nil, nil, err
	}

	// The build checkpoints into the same accumulating cache Fetch uses, so a
	// run that dies partway keeps what it had. Before this, a throttle at
	// chunk 360 of 787 discarded 360 chunks of somebody else's service time
	// as well as fourteen minutes of ours, and the only recovery was to ask
	// for all of it again — which is precisely the behaviour that gets a
	// client throttled in the first place.
	band := build.Bands[0].Name
	values := bands[band]

	bucket, prefix, cacheErr := remote.CacheDir(ctx, remote.GaiaTAP)

	key := ""
	if cacheErr == nil {
		key = path.Join(prefix, build.cacheKey())
		_ = readCache(ctx, bucket, key, values, counts)
	}

	checkpoint := func() {
		if cacheErr == nil {
			_ = writeCache(ctx, bucket, key, band, values, counts)
		}
	}
	defer checkpoint() // a failed build still keeps its completed chunks

	chunks := int((npix + int64(build.Chunk) - 1) / int64(build.Chunk))

	for c := range chunks {
		first := int64(c) * int64(build.Chunk)

		last := first + int64(build.Chunk) - 1
		if last >= npix {
			last = npix - 1
		}

		if build.chunkIsCached(values, first, last) {
			if build.Progress != nil {
				build.Progress(c+1, chunks)
			}

			continue
		}

		if err := build.fetchChunkWithRetry(ctx, client, first, last, bands, counts, solidAngle); err != nil {
			return nil, nil, err
		}

		if (c+1)%checkpointEvery == 0 {
			checkpoint()
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

	client, err := aggregationClient()
	if err != nil {
		return nil, nil, err
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

// aggregationTimeout is the per-request budget for one chunk.
//
// The registered Gaia endpoint allows 30 seconds, which suits a catalogue
// lookup. One chunk here is a GROUP BY over a few million sources and the
// archive's own queueing, and 30 seconds is not reliably enough: a run of 787
// chunks meets a slow one sooner or later.
const aggregationTimeout = 3 * time.Minute

// aggregationRetries is how many times a chunk is re-attempted.
//
// A whole-sky build is hundreds of queries over half an hour, so the chance
// that none of them times out is not the chance any single one succeeds.
// Losing the completed chunks because query 360 was slow is the difference
// between a tool that finishes and one that has to be babysat.
const aggregationRetries = 4

// aggregationBudget bounds the total time one chunk may spend across all its
// attempts.
//
// Without it the retry multiplies rather than absorbs: the Gaia archive
// answers a given query in three seconds when it is quiet and times out its
// own SQL statement when it is not, and four retries of the latter is twelve
// minutes of waiting to arrive at the same failure. Measured against the live
// service in one afternoon, the same forty-pixel query took 18 seconds, then
// 182, then stopped responding entirely.
const aggregationBudget = 6 * time.Minute

// aggregationPace is the minimum gap between queries this package sends.
//
// The Gaia archive is free, anonymous, and under no obligation to serve
// anyone. A whole-sky build asks it 787 questions in a row; three such builds
// and a handful of probes in one afternoon was enough that submissions stopped
// being answered at all — not with an error, but with silence, including for a
// deliberately malformed query that should have failed at the parser in
// milliseconds. GETs to the same host kept working throughout, which is what
// distinguishes a defending service from a broken one.
//
// So the queries are spaced and serialised. It makes a whole-sky build slower
// and that is the correct trade: the cost of being wrong here is borne by
// every other user of a shared research instrument, not by us.
const aggregationPace = 2 * time.Second

// checkpointEvery is how many chunks are aggregated between cache writes.
//
// Writing after every chunk would rewrite the whole partial map 787 times for
// one order-8 build. Writing only at the end is what made a failure cost
// everything. Twenty-five chunks is about a minute of work at the paced rate,
// which is the most a stumble should cost.
const checkpointEvery = 25

// chunkIsCached reports whether every pixel in the range already has a value,
// so a resumed build can skip the query rather than repeat it.
//
// A pixel with genuinely no flux is indistinguishable from an unfetched one
// here, so such a pixel is queried again on every resume. At order 8 the real
// map has none — every pixel holds sources — and paying one redundant chunk
// beats inventing a separate presence bitmap to avoid it.
func (g GaiaBuild) chunkIsCached(values []float64, first, last int64) bool {
	for pixel := first; pixel <= last && pixel < int64(len(values)); pixel++ {
		if values[pixel] <= 0 {
			return false
		}
	}

	return true
}

// aggregationClient builds the TAP client this package's queries go through.
func aggregationClient() (*api.Client, error) {
	client, err := api.NewClient(remote.GaiaTAP,
		api.WithTimeout(aggregationTimeout),
		api.WithMinInterval(aggregationPace))
	if err != nil {
		return nil, fmt.Errorf("starlight: gaia client: %w", err)
	}

	return client, nil
}

// fetchChunkWithRetry runs one chunk, re-attempting a transient failure.
//
// The client already retries the statuses resty classifies as retriable; what
// it cannot retry is a request whose own timeout expired, because by then the
// context is done. That is the failure a long build actually hits, so it is
// retried here with a fresh deadline and a backoff.
//
// A cancelled parent context is not a transient failure and stops immediately.
//
// Retries are not reported through Progress. Overloading a (done, total)
// callback with a sentinel would make every existing caller's arithmetic
// wrong; a caller that wants to see the stumbles can wrap the client.
func (g GaiaBuild) fetchChunkWithRetry(
	ctx context.Context,
	client *api.Client,
	first, last int64,
	bands map[string][]float64,
	counts []int64,
	solidAngle float64,
) error {
	var err error

	deadline := time.Now().Add(aggregationBudget)

	for attempt := range aggregationRetries {
		if attempt > 0 {
			// A server-side timeout consumes the whole per-request budget, so
			// retrying four of them costs four times three minutes and turns a
			// slow archive into a twelve-minute failure. Retries are worth
			// having for a transient stumble and not for a congested service,
			// so they stop when the chunk has had long enough overall.
			if time.Now().After(deadline) {
				break
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("starlight: pixels %d-%d: %w", first, last, ctx.Err())
			case <-time.After(time.Duration(attempt) * 5 * time.Second):
			}
		}

		err = g.fetchChunk(ctx, client, first, last, bands, counts, solidAngle)
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return err
		}
	}

	return fmt.Errorf("starlight: pixels %d-%d after %d attempts: %w",
		first, last, aggregationRetries, err)
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

	return g.runQuery(ctx, client, adql, bands, counts, solidAngle,
		fmt.Sprintf("pixels %d-%d", first, last))
}

// runQuery posts one ADQL statement and accumulates its rows.
//
// The query goes in a POST body rather than a URL: asking about several
// hundred scattered pixels renders tens of kilobytes of disjunction, which no
// query string will carry.
//
// what names the pixels for the error message, since a caller looking at a
// failure needs to know which part of the sky it was about.
func (g GaiaBuild) runQuery(
	ctx context.Context,
	client *api.Client,
	adql string,
	bands map[string][]float64,
	counts []int64,
	solidAngle float64,
	what string,
) error {
	v := url.Values{}
	v.Set("REQUEST", "doQuery")
	v.Set("LANG", "ADQL")
	v.Set("FORMAT", "csv")
	v.Set("QUERY", adql)

	body, err := client.PostForm(ctx, remote.GaiaTAP, "", v)
	if err != nil {
		return fmt.Errorf("starlight: %s: %w", what, err)
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

			flux += g.recoverColourless(b, index, row)

			bands[b.Name][pixel] = flux * b.FluxToRadiance / solidAngle
		}
	}
}

// recoverColourless returns the flux the colour polynomial dropped, scaled as
// though those sources carried the pixel's mean colour.
//
// Sources without BP-RP are 15 per cent of the sky and over half of the
// densest pixels, so dropping them underestimates the Galactic plane
// specifically. Masana et al. (2021) assign such stars the local mean colour;
// this does the same, per HEALPix pixel, which is as local as the aggregate
// allows.
//
// It returns zero — leaving the uncorrected sum — when the response predates
// these columns, when nothing was dropped, or when a pixel has no coloured
// source at all to average. That last case cannot be corrected by any local
// mean, and inventing a global one would be exactly the fabrication this
// package refuses elsewhere.
func (g GaiaBuild) recoverColourless(b GaiaBand, index map[string]int, row []string) float64 {
	if len(b.ColourTerm) == 0 {
		return 0
	}

	col := strings.ToLower(columnName(b.Name))

	value := func(suffix string) (float64, bool) {
		i, ok := index[col+suffix]
		if !ok || i >= len(row) {
			return 0, false
		}

		v, err := strconv.ParseFloat(strings.TrimSpace(row[i]), 64)
		if err != nil {
			return 0, false
		}

		return v, true
	}

	all, okAll := value("_all")
	coloured, okCol := value("_col")
	mean, okMean := value("_mc")

	if !okAll || !okCol || !okMean {
		return 0
	}

	dropped := all - coloured
	if dropped <= 0 {
		return 0
	}

	return dropped * b.colourFactor(mean)
}

// johnsonVZeroFlux is Johnson V's Vega zero point: the spectral flux density
// of a V = 0 star, in W m^-2 nm^-1.
//
// Both paths into a V-band map rest on it — the Gaia conversion in
// [GaiaJohnsonV] and the Hipparcos one in [AddBrightStars] — so it is declared
// once. Two copies of a zero point are two chances for them to drift apart,
// and a map built half on each would be wrong by the difference with nothing
// to show it.
const johnsonVZeroFlux = 3.63e-11

// GaiaJohnsonV returns the Gaia G to Johnson V band, ready for [GaiaBuild].
//
// It exists because the conversion needs three published numbers that come
// from three different places, and mixing them is invisible in the result:
//
//   - Gaia DR3's G-band VEGAMAG zero point, 25.6874, which turns
//     phot_g_mean_flux in e-/s into a G magnitude.
//   - The G to V colour transformation of Riello et al. (2021), Gaia DR3
//     photometric documentation, Section 5.5.1, Table 5.9. The table is
//     tabulated as G minus
//     the target band, so its polynomial is G - V directly and is entered
//     here unchanged, which is what the query's +0.4 exponent needs: the
//     factor is 10^(0.4*(G-V)) = F_V/F_G. G - V is negative for ordinary
//     stars, so a red star loses flux in V, which is the physical direction
//   - G spans 330-1050 nm against V's 500-600 nm, so a star is always
//     brighter in G. See [magnitude.GaiaGToJohnsonV] for how the direction
//     was established.
//   - Johnson V's Vega zero point, 3.63e-11 W m^-2 nm^-1, which turns that V
//     magnitude into a spectral flux density.
//
// Using G's zero point with V's flux density and no colour term — the obvious
// mistake — produces a map that is neither a G map nor a V map, and is wrong
// by the colour term of whatever mix of spectral types the pixel holds. The
// result is plausible everywhere and badly wrong along the Galactic plane,
// where the ensemble is reddest.
//
// The transformation is fitted for -0.5 < BP-RP < 5.0. Sources with no colour
// at all make the polynomial null and SQL drops them from the sum; their count
// comes back per pixel so the caller can see how much of a pixel that is.
func GaiaJohnsonV() GaiaBand {
	const gZeroPoint = 25.6874 // Gaia DR3 G VEGAMAG zero point

	return GaiaBand{
		Name:           "V",
		ColourTerm:     []float64{-0.02704, 0.01424, -0.2156, 0.01426},
		FluxToRadiance: johnsonVZeroFlux / math.Pow(10, gZeroPoint/2.5),
	}
}

// JohnsonCousinsColourTerm returns the published Gaia G-to-band polynomial for
// one of the Johnson-Cousins bands B, V, R or I.
//
// The coefficients are the Gaia DR3 photometric documentation, Section 5.5.1,
// Table 5.9, tabulated as G minus the target band and in ascending powers of
// BP-RP. They are the same relations [github.com/TuSKan/astrogo/magnitude]
// exposes as functions; here they are needed as data, because the aggregation
// renders them into ADQL and evaluates them server-side inside the SUM.
//
// U is absent and cannot be added: Gaia's bluest band starts around 330 nm and
// Table 5.9 publishes no G-to-U relation, because Gaia does not constrain the
// Balmer jump well enough to support one. A four-band map is what this
// catalogue can produce.
func JohnsonCousinsColourTerm(band string) ([]float64, error) {
	switch band {
	case "B":
		return []float64{0.01448, -0.6874, -0.3604, 0.06718, -0.006061}, nil
	case "V":
		return []float64{-0.02704, 0.01424, -0.2156, 0.01426}, nil
	case "R":
		return []float64{-0.02275, 0.3961, -0.1243, -0.01396, 0.003775}, nil
	case "I":
		return []float64{0.01753, 0.76, -0.0991}, nil
	default:
		return nil, fmt.Errorf("%w: no published Gaia relation for band %q; Table 5.9 has "+
			"B, V, R and I, and no U", ErrGaiaBand, band)
	}
}

// VegaZeroFlux returns a passband's Vega zero point as a spectral flux
// density in W m^-2 nm^-1: the flux of a zero-magnitude star.
//
// Derived from the passband rather than tabulated, because a zero point and
// the curve it belongs to are one calibration and transcribing half of it
// invites the two to drift. The service publishes the zero point in janskys,
// which is per unit frequency; converting to per unit wavelength needs the
// band's pivot wavelength, which is the wavelength at which that conversion is
// exact for any spectrum:
//
//	F_lambda = F_nu * c / lambda_pivot^2
//
// Checked against the value this package used to carry as a literal: Bessell V
// at 3630.22 Jy and a pivot of 547.77 nm gives 3.627e-11, against the 3.63e-11
// that was written down.
func VegaZeroFlux(band magnitude.Passband) (float64, error) {
	if band.VegaZeroPointJy <= 0 {
		return 0, fmt.Errorf("%w: passband %q carries no Vega zero point",
			ErrGaiaBand, band.Name)
	}

	pivot, err := band.PivotWavelength()
	if err != nil {
		return 0, fmt.Errorf("%w: passband %q: %w", ErrGaiaBand, band.Name, err)
	}

	const (
		jansky      = 1e-26 // W m^-2 Hz^-1
		metrePerNM  = 1e-9
		perMToPerNM = 1e-9
	)

	lambdaM := float64(pivot) * metrePerNM

	return band.VegaZeroPointJy * jansky *
		constants.SI2019.SpeedOfLight.Value / (lambdaM * lambdaM) * perMToPerNM, nil
}

// GaiaJohnsonCousins describes one output band of the map, given the passband
// it is calibrated on.
//
// One constructor rather than a GaiaJohnsonB/V/R/I apiece: the band differs
// only in its colour polynomial and its zero point, and the first comes from
// [JohnsonCousinsColourTerm] while the second comes from the passband the
// caller already has. A caller resolves that passband from
// [github.com/TuSKan/astrogo/skybrightness/dataset/passband], so the curve,
// the detector convention and the zero point all arrive together from one
// published calibration.
//
// name selects the published relation and labels the band in the map; it is
// one of "B", "V", "R", "I".
func GaiaJohnsonCousins(name string, band magnitude.Passband) (GaiaBand, error) {
	colour, err := JohnsonCousinsColourTerm(name)
	if err != nil {
		return GaiaBand{}, err
	}

	zero, err := VegaZeroFlux(band)
	if err != nil {
		return GaiaBand{}, err
	}

	// The Gaia DR3 G VEGAMAG zero point, which turns a catalogue flux in
	// electrons per second into a magnitude and so has to be undone before the
	// target band's zero point is applied.
	const gZeroPoint = 25.6874

	return GaiaBand{
		Name:           name,
		ColourTerm:     colour,
		FluxToRadiance: zero / math.Pow(10, gZeroPoint/2.5),
	}, nil
}
