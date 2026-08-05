package atlas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"

	gofs "github.com/ungerik/go-fs"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/lpmap"
)

// Layer identifies one published light-pollution data source [Resolver]
// can answer from. Choosing a layer is the whole configuration surface
// most callers need — see [NewResolver]'s doc comment.
type Layer int

const (
	// LayerAuto tries the freshest available source automatically:
	// VIIRS (download, newest published year) -> World Atlas (download,
	// or WithAtlasFile) -> Light Pollution Map live API (if
	// WithLightPollutionMap was given) -> Bortle (if WithBortleClass was
	// given) -> a fixed scalar (if WithScalarSQM was given) ->
	// ErrNoTierAvailable. The default, and the recommended choice for
	// "just give me a floor."
	//
	// Freshness is preferred over modelling fidelity here — see
	// autoOrder for why, and use [WithLayer] to demand a specific
	// source instead.
	LayerAuto Layer = iota
	// LayerWorldAtlas resolves from Falchi et al. 2016's World Atlas
	// 2015 (propagated, mcd/m², CC BY-NC 4.0, static — frozen at 2015)
	// — the highest-fidelity source this package can reach. Downloads
	// and extracts the ~653 MB archive on first use unless
	// [WithAtlasFile] supplies an already-downloaded copy.
	LayerWorldAtlas
	// LayerVIIRS resolves from a VIIRS annual raw-radiance composite
	// (see [WithVIIRSYear]; default is the newest published year) —
	// fresher than World Atlas (data through the current year) but lower
	// fidelity: a raw-radiance empirical fit, not propagated through an
	// atmospheric model. Downloads the ~700 MB-1 GB archive on first
	// use for the requested year; no API key needed.
	//
	// BLIND AT DARK SITES. The composite stores a hard 0 wherever the
	// day-night band detected nothing, so Paranal, Mauna Kea and the
	// middle of the Pacific all read exactly 0 nW·cm⁻²·sr⁻¹ and resolve
	// to an infinite (zero-flux) artificial floor — see
	// [skybrightness.RadianceToArtificialSB]. That is honest for a
	// measurement, but it means VIIRS cannot rank one dark site against
	// another: everything below roughly 0.07 nW·cm⁻²·sr⁻¹ collapses to
	// "no artificial light". [LayerWorldAtlas], being a propagation
	// model rather than a measurement, still resolves gradations there.
	LayerVIIRS
	// LayerLightPollutionMap resolves via a live lightpollutionmap.info
	// point query (see [WithLightPollutionMap]) — needs a manually-
	// issued API key (see the lpmap package doc) and network access per
	// query, but no large local file.
	//
	// Which DATA this serves is the client's choice, not this package's:
	// lpmap defaults to World Atlas 2015, so unlike [LayerVIIRS] this rung
	// is not automatically on the newest composite. Build the client with
	// lpmap.WithLayer(lpmap.VIIRSLayer(year)) to put it there.
	LayerLightPollutionMap
	// LayerBortle resolves to a fixed Bortle-class brightness (see
	// [WithBortleClass]) — no geographic lookup, no data or network at
	// all.
	LayerBortle
	// LayerScalar resolves to a fixed, caller-supplied brightness (see
	// [WithScalarSQM]) — the simplest possible source.
	LayerScalar
)

// String names the layer, for logging/diagnostics.
func (l Layer) String() string {
	switch l {
	case LayerAuto:
		return "auto"
	case LayerWorldAtlas:
		return "world-atlas"
	case LayerVIIRS:
		return "viirs"
	case LayerLightPollutionMap:
		return "light-pollution-map"
	case LayerBortle:
		return "bortle"
	case LayerScalar:
		return "scalar"
	default:
		return fmt.Sprintf("Layer(%d)", int(l))
	}
}

// autoOrder is the fixed sequence LayerAuto tries — FRESHNESS first,
// coarse-but-always-configurable last.
//
// VIIRS leads deliberately, and it is a real tradeoff rather than an
// obvious win: VIIRS publishes through the current year (see
// NewestVIIRSYear) while the World Atlas is frozen at 2015, and a decade
// of change in artificial lighting usually outweighs the modelling gap.
// But VIIRS is raw satellite radiance put through an empirical fit,
// whereas the World Atlas is propagated through an atmospheric
// radiative-transfer model — so the World Atlas remains the
// higher-FIDELITY answer, and a caller who wants it should say
// WithLayer(LayerWorldAtlas) rather than rely on the ladder.
// Result.Layer always reports which one actually answered.
//
// Not caller-reorderable: this package trades that flexibility for a
// "choose a layer and it works" default (see the package doc). A layer
// with no corresponding requirement met (e.g. LayerLightPollutionMap
// with no client configured) is skipped rather than attempted and failed.
var autoOrder = []Layer{LayerVIIRS, LayerWorldAtlas, LayerLightPollutionMap, LayerBortle, LayerScalar}

// Sentinel errors for a single, explicitly chosen Layer missing its
// required companion option.
var (
	// ErrLightPollutionMapNotConfigured is returned when LayerLightPollutionMap
	// is explicitly chosen but WithLightPollutionMap was not given.
	ErrLightPollutionMapNotConfigured = errors.New("atlas: LayerLightPollutionMap chosen but WithLightPollutionMap was not given")
	// ErrBortleClassNotConfigured is returned when LayerBortle is
	// explicitly chosen but WithBortleClass was not given.
	ErrBortleClassNotConfigured = errors.New("atlas: LayerBortle chosen but WithBortleClass was not given")
	// ErrScalarNotConfigured is returned when LayerScalar is explicitly
	// chosen but WithScalarSQM was not given.
	ErrScalarNotConfigured = errors.New("atlas: LayerScalar chosen but WithScalarSQM was not given")
	// ErrNilLocation is returned by [Resolver.Floor]/[FloorAt] for a nil
	// location.
	ErrNilLocation = errors.New("atlas: nil location")
)

// ErrNoTierAvailable is returned by [Resolver.Floor] when every attempted
// layer failed (or LayerAuto found nothing configured to even attempt).
// Use [errors.Is] against a more specific sentinel (e.g.
// remote.ErrDownloadDenied, lpmap.ErrNoAPIKey) to identify which
// underlying cause applies — this error joins every attempted layer's
// own error via [errors.Join], so all of them remain reachable.
var ErrNoTierAvailable = errors.New("atlas: no layer could resolve a light-pollution floor")

// errLayerAutoNotResolvable guards against resolveLayer ever being
// called with LayerAuto directly (Floor always expands it to autoOrder
// first) — a defensive, should-never-happen case, not a documented
// public error.
var errLayerAutoNotResolvable = errors.New("LayerAuto is not itself resolvable")

// errUnknownLayer guards against resolveLayer being called with a Layer
// value outside the documented enum — a defensive, should-never-happen
// case, not a documented public error.
var errUnknownLayer = errors.New("unknown layer")

// Attempt records one layer [Resolver.Floor] tried — see [Result.Attempts].
type Attempt struct {
	Layer Layer
	Err   error // nil for the layer that ultimately answered
}

// Result is the outcome of a successful [Resolver.Floor] call.
type Result struct {
	// Floor is the resolved artificial-only light-pollution floor,
	// composable with Airglow/Zodiacal/Moonlight in a
	// skybrightness.CompositeModel without double-counting the natural
	// background.
	Floor skybrightness.Floor
	// SQM is Floor's zenith artificial brightness as a single V-band
	// magnitude value, for display/logging convenience.
	SQM skybrightness.SurfaceBrightnessV
	// Layer is which layer ultimately answered.
	Layer Layer
	// Source is a short, human-readable description of where the value
	// came from (a file path, "downloaded", the API host, ...).
	Source string
	// Attempts records every layer that was tried before (and
	// including) Layer, each with its own error (nil for the last,
	// successful one) — so a caller can see not just that a
	// higher-fidelity layer was unavailable, but specifically why.
	Attempts []Attempt
}

// WithLayer chooses which data source [Resolver.Floor] resolves from.
// Default: [LayerAuto].
func WithLayer(l Layer) Option { return func(c *config) { c.layer = l } }

// WithAtlasFile supplies an already-downloaded World Atlas (or
// compatible) GeoTIFF at path for [LayerWorldAtlas]/[LayerAuto], skipping
// the automatic download. The file is opened lazily on first use and
// kept open for the [Resolver]'s lifetime (see [Resolver.Close]).
func WithAtlasFile(path string) Option {
	return func(c *config) { c.atlasFile = path; c.hasAtlasFile = true }
}

// WithVIIRSYear pins which annual composite [LayerVIIRS]/[LayerAuto]
// downloads. Without it the layer resolves the newest published year
// automatically via [NewestVIIRSYear] — pin one for reproducibility, or
// to compare years. Anything from [EarliestVIIRSYear] up is accepted;
// there is no upper bound, so a year newer than this build knows about
// still works (upstream decides what exists — see [EnsureVIIRSAnnual]).
func WithVIIRSYear(year int) Option {
	return func(c *config) { c.viirsYear = year; c.viirsYearPinned = true }
}

// WithLightPollutionMap enables [LayerLightPollutionMap]/[LayerAuto],
// resolving via client's live lightpollutionmap.info query. Build one
// with lpmap.New(lpmap.WithAPIKey(key)) — a nil client leaves the layer
// unconfigured ([LayerAuto] skips it; [LayerLightPollutionMap] then
// returns [ErrLightPollutionMapNotConfigured]).
func WithLightPollutionMap(client *lpmap.Client) Option {
	return func(c *config) { c.lpmapClient = client }
}

// WithBortleClass enables [LayerBortle]/[LayerAuto], resolving to a
// fixed brightness for the given Bortle class (1-9; see
// [skybrightness.FloorFromBortle]) — no geographic lookup.
func WithBortleClass(class int) Option {
	return func(c *config) { c.bortleClass = class; c.hasBortle = true }
}

// WithScalarSQM enables [LayerScalar]/[LayerAuto], resolving to a fixed,
// caller-supplied artificial-only brightness — the simplest possible
// source, and [LayerAuto]'s final fallback.
func WithScalarSQM(sqm skybrightness.SurfaceBrightnessV) Option {
	return func(c *config) { c.scalarSQM = sqm; c.hasScalar = true }
}

// WithQuiet disables the default download-progress logging a Resolver
// otherwise applies to [LayerWorldAtlas]/[LayerVIIRS] (see
// [WithDownloadProgress]).
func WithQuiet() Option { return func(c *config) { c.quiet = true } }

// openResult is the cached outcome of lazily opening a download-backed
// layer's provider.
type openResult struct {
	provider skybrightness.SQMProvider
	err      error
}

// Resolver resolves a site's light-pollution floor per the [Layer]
// [NewResolver] was configured with. Build one with [NewResolver]; safe
// for concurrent use.
type Resolver struct {
	cfg config

	atlasFileOnce   sync.Once
	atlasFileResult openResult

	worldAtlasOnce   sync.Once
	worldAtlasResult openResult

	viirsOnce   sync.Once
	viirsResult openResult

	viirsYearOnce sync.Once
	viirsYear     int

	mu      sync.Mutex
	closers []io.Closer
}

// NewResolver builds a Resolver from the same [Option] type
// EnsureWorldAtlas/OpenWorldAtlas/EnsureVIIRSAnnual/OpenVIIRSAnnual take —
// a download-related option (WithDownloadProgress/WithCacheDir/
// WithKeepArchive) applies here exactly as it does there, with no
// separate option type or translation layer to wire up.
//
// With no options at all it defaults to [LayerAuto] with only the
// always-attempted download-backed layers (World Atlas, VIIRS) in play —
// configure at least one of [WithLightPollutionMap]/[WithBortleClass]/
// [WithScalarSQM] for [Resolver.Floor] to have a fallback once download
// consent isn't granted (see remote.EnableDownloads).
//
// Choosing a single explicit [Layer] via [WithLayer] that's missing its
// required companion option (e.g. [LayerBortle] with no
// [WithBortleClass]) is a configuration error surfaced by
// [Resolver.Floor], not by NewResolver — NewResolver always succeeds.
func NewResolver(opts ...Option) *Resolver {
	cfg := config{layer: LayerAuto, viirsYear: LatestVIIRSYear}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Resolver{cfg: cfg}
}

// FloorAt resolves loc's artificial-only light-pollution floor in one
// call — it builds a [Resolver] from opts, queries it, and releases it:
//
//	result, err := atlas.FloorAt(ctx, site.Location(), atlas.WithBortleClass(4))
//
// This is the whole API most callers need. Use [NewResolver] instead
// when querying MANY locations: a Resolver keeps its (multi-gigabyte)
// atlas file open across calls, whereas FloorAt reopens and re-parses it
// every time.
//
// The returned [Result.Floor] stays valid after FloorAt returns: every
// layer yields a scalar floor ([skybrightness.NewFloorSQM]) holding no
// reference to the closed file.
func FloorAt(ctx context.Context, loc *coord.Geodetic, opts ...Option) (Result, error) {
	resolver := NewResolver(opts...)
	defer func() { _ = resolver.Close() }()

	return resolver.Floor(ctx, loc)
}

// Close releases every resource opened by a download-backed layer (the
// GeoTIFF file handle for World Atlas/VIIRS). Safe to call even if no
// such layer was ever used or configured. Safe to call once; further
// Floor calls after Close will re-fail every layer that needed a now-
// closed handle, since Close does not reset a layer's cached open
// attempt.
func (r *Resolver) Close() error {
	r.mu.Lock()
	closers := r.closers
	r.closers = nil
	r.mu.Unlock()

	errs := make([]error, 0, len(closers))
	for _, c := range closers {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Floor resolves loc's artificial-only light-pollution floor — either
// from the single [Layer] [NewResolver] was configured with, or, under
// [LayerAuto], by trying each configured layer in [autoOrder] and
// returning the first one that succeeds. See [Result] and [Attempt] for
// how to inspect what was tried, and [FloorAt] for the one-shot form
// that needs no Resolver of its own.
//
// loc supplies the geodetic position to sample; a nil loc returns
// [ErrNilLocation]. Only latitude and longitude are used — a
// light-pollution atlas is a ground-level 2-D grid, so loc's height is
// ignored.
func (r *Resolver) Floor(ctx context.Context, loc *coord.Geodetic) (Result, error) {
	if loc == nil {
		return Result{}, ErrNilLocation
	}

	latDeg, lonDeg := loc.Lat().Degrees(), loc.Lon().Degrees()

	layers := []Layer{r.cfg.layer}
	if r.cfg.layer == LayerAuto {
		layers = autoOrder
	}

	var (
		attempts []Attempt
		joined   error
	)

	for _, layer := range layers {
		if r.cfg.layer == LayerAuto && !r.autoConfigured(layer) {
			continue
		}

		floor, sqm, source, err := r.resolveLayer(ctx, layer, latDeg, lonDeg)
		if err != nil {
			attempts = append(attempts, Attempt{Layer: layer, Err: err})
			joined = errors.Join(joined, err)

			continue
		}

		attempts = append(attempts, Attempt{Layer: layer})

		return Result{Floor: floor, SQM: sqm, Layer: layer, Source: source, Attempts: attempts}, nil
	}

	if joined == nil {
		return Result{Attempts: attempts}, ErrNoTierAvailable
	}

	return Result{Attempts: attempts}, fmt.Errorf("%w: %w", ErrNoTierAvailable, joined)
}

// autoConfigured reports whether layer has enough configuration to be
// worth attempting under LayerAuto. World Atlas/VIIRS are always
// attempted — a missing download consent is itself a clean, informative
// failure, not a reason to skip trying. Light Pollution Map/Bortle/Scalar
// are skipped (rather than attempted and failed) when their own option
// was never given, since there is nothing meaningful to try.
func (r *Resolver) autoConfigured(layer Layer) bool {
	switch layer {
	case LayerLightPollutionMap:
		return r.cfg.lpmapClient != nil
	case LayerBortle:
		return r.cfg.hasBortle
	case LayerScalar:
		return r.cfg.hasScalar
	case LayerAuto, LayerWorldAtlas, LayerVIIRS:
		return true
	default:
		return true
	}
}

// resolveLayer attempts exactly one layer.
func (r *Resolver) resolveLayer(
	ctx context.Context, layer Layer, latDeg, lonDeg float64,
) (skybrightness.Floor, skybrightness.SurfaceBrightnessV, string, error) {
	switch layer {
	case LayerWorldAtlas:
		return r.resolveWorldAtlas(ctx, latDeg, lonDeg)
	case LayerVIIRS:
		return r.resolveVIIRS(ctx, latDeg, lonDeg)
	case LayerLightPollutionMap:
		return r.resolveLightPollutionMap(ctx, latDeg, lonDeg)
	case LayerBortle:
		return r.resolveBortle()
	case LayerScalar:
		return r.resolveScalar()
	case LayerAuto:
		return skybrightness.Floor{}, 0, "", fmt.Errorf("atlas: %w", errLayerAutoNotResolvable)
	default:
		return skybrightness.Floor{}, 0, "", fmt.Errorf("atlas: %w: %s", errUnknownLayer, layer)
	}
}

func (r *Resolver) resolveWorldAtlas(
	ctx context.Context, latDeg, lonDeg float64,
) (skybrightness.Floor, skybrightness.SurfaceBrightnessV, string, error) {
	res := r.worldAtlasProvider(ctx)
	if res.err != nil {
		return skybrightness.Floor{}, 0, "", res.err
	}

	sqm, err := res.provider.ZenithBrightness(latDeg, lonDeg)
	if err != nil {
		return skybrightness.Floor{}, 0, "", fmt.Errorf("atlas: sample World Atlas: %w", err)
	}

	source := "World Atlas 2015 (downloaded)"
	if r.cfg.hasAtlasFile {
		source = "World Atlas file: " + r.cfg.atlasFile
	}

	return skybrightness.NewFloorSQM(sqm), sqm, source, nil
}

func (r *Resolver) resolveVIIRS(
	ctx context.Context, latDeg, lonDeg float64,
) (skybrightness.Floor, skybrightness.SurfaceBrightnessV, string, error) {
	year := r.viirsYearFor(ctx)

	res := r.viirsProvider(ctx)
	if res.err != nil {
		return skybrightness.Floor{}, 0, "", res.err
	}

	sqm, err := res.provider.ZenithBrightness(latDeg, lonDeg)
	if err != nil {
		return skybrightness.Floor{}, 0, "", fmt.Errorf("atlas: sample VIIRS %d: %w", year, err)
	}

	return skybrightness.NewFloorSQM(sqm), sqm, fmt.Sprintf("VIIRS %d annual composite (downloaded)", year), nil
}

// viirsYearFor resolves which VIIRS year this Resolver uses, at most once:
// the pinned year from WithVIIRSYear, otherwise the newest one upstream
// actually publishes. A failed probe (offline, unreachable) is swallowed
// deliberately — NewestVIIRSYear still returns the best-known year, and
// falling back to LatestVIIRSYear is far better than failing the whole
// layer over a version check.
func (r *Resolver) viirsYearFor(ctx context.Context) int {
	if r.cfg.viirsYearPinned {
		return r.cfg.viirsYear
	}

	r.viirsYearOnce.Do(func() {
		year, err := NewestVIIRSYear(ctx)
		if err != nil {
			log.Printf("atlas: could not confirm the newest VIIRS year (%v); using %d", err, year)
		}

		r.viirsYear = year
	})

	return r.viirsYear
}

func (r *Resolver) resolveLightPollutionMap(
	ctx context.Context, latDeg, lonDeg float64,
) (skybrightness.Floor, skybrightness.SurfaceBrightnessV, string, error) {
	if r.cfg.lpmapClient == nil {
		return skybrightness.Floor{}, 0, "", ErrLightPollutionMapNotConfigured
	}

	floor, err := r.cfg.lpmapClient.Floor(ctx, latDeg, lonDeg)
	if err != nil {
		return skybrightness.Floor{}, 0, "", fmt.Errorf("atlas: lpmap: %w", err)
	}

	sqm, err := zenithSQM(floor)
	if err != nil {
		return skybrightness.Floor{}, 0, "", err
	}

	return floor, sqm, "lightpollutionmap.info", nil
}

func (r *Resolver) resolveBortle() (skybrightness.Floor, skybrightness.SurfaceBrightnessV, string, error) {
	if !r.cfg.hasBortle {
		return skybrightness.Floor{}, 0, "", ErrBortleClassNotConfigured
	}

	floor, err := skybrightness.FloorFromBortle(r.cfg.bortleClass)
	if err != nil {
		return skybrightness.Floor{}, 0, "", fmt.Errorf("atlas: %w", err)
	}

	sqm, err := zenithSQM(floor)
	if err != nil {
		return skybrightness.Floor{}, 0, "", err
	}

	return floor, sqm, fmt.Sprintf("Bortle class %d (fixed, no geographic lookup)", r.cfg.bortleClass), nil
}

func (r *Resolver) resolveScalar() (skybrightness.Floor, skybrightness.SurfaceBrightnessV, string, error) {
	if !r.cfg.hasScalar {
		return skybrightness.Floor{}, 0, "", ErrScalarNotConfigured
	}

	return skybrightness.NewFloorSQM(r.cfg.scalarSQM), r.cfg.scalarSQM, "fixed scalar fallback", nil
}

func (r *Resolver) registerCloser(c io.Closer) {
	r.mu.Lock()
	r.closers = append(r.closers, c)
	r.mu.Unlock()
}

// downloadOpts builds the []Option a World-Atlas/VIIRS download inside
// this Resolver uses: the Resolver's own download-related settings,
// defaulting to ProgressLogger unless WithDownloadProgress or WithQuiet
// was given.
func (r *Resolver) downloadOpts(progressLabel string) []Option {
	opts := make([]Option, 0, 3)

	switch {
	case r.cfg.hasProgress:
		opts = append(opts, WithDownloadProgress(r.cfg.progress))
	case !r.cfg.quiet:
		opts = append(opts, WithDownloadProgress(ProgressLogger(progressLabel)))
	}

	if r.cfg.cacheDir != "" {
		opts = append(opts, WithCacheDir(r.cfg.cacheDir))
	}

	opts = append(opts, WithKeepArchive(r.cfg.keepArchive))

	return opts
}

// atlasFileProvider lazily opens WithAtlasFile's file at most once,
// caching the result (success or failure) for the Resolver's lifetime.
func (r *Resolver) atlasFileProvider() openResult {
	r.atlasFileOnce.Do(func() {
		rs, err := gofs.File(r.cfg.atlasFile).OpenReadSeeker()
		if err != nil {
			r.atlasFileResult = openResult{err: fmt.Errorf("atlas: open atlas file %s: %w", r.cfg.atlasFile, err)}
			return
		}

		p, err := NewFalchiProvider(rs)
		if err != nil {
			_ = rs.Close()

			r.atlasFileResult = openResult{err: fmt.Errorf("atlas: open atlas file %s: %w", r.cfg.atlasFile, err)}

			return
		}

		r.registerCloser(rs)

		r.atlasFileResult = openResult{provider: p}
	})

	return r.atlasFileResult
}

// worldAtlasProvider lazily opens WithAtlasFile's file if given,
// otherwise downloads/extracts/opens the World Atlas archive — at most
// once either way, caching the result (success or failure) for the
// Resolver's lifetime. The context of whichever Floor call triggers the
// (first) open governs a download; subsequent calls reuse the cached
// provider (or cached failure) regardless of their own context.
func (r *Resolver) worldAtlasProvider(ctx context.Context) openResult {
	if r.cfg.hasAtlasFile {
		return r.atlasFileProvider()
	}

	r.worldAtlasOnce.Do(func() {
		p, closer, err := OpenWorldAtlas(ctx, r.downloadOpts("World Atlas 2015")...)
		if err != nil {
			// Stored unwrapped: OpenWorldAtlas already prefixes "atlas: ".
			r.worldAtlasResult = openResult{err: err}
			return
		}

		r.registerCloser(closer)

		r.worldAtlasResult = openResult{provider: p}
	})

	return r.worldAtlasResult
}

// viirsProvider lazily downloads/extracts/opens the configured VIIRS
// year's archive at most once, caching the result (success or failure)
// for the Resolver's lifetime — same context-ownership note as
// worldAtlasProvider.
func (r *Resolver) viirsProvider(ctx context.Context) openResult {
	r.viirsOnce.Do(func() {
		year := r.viirsYearFor(ctx)
		label := fmt.Sprintf("VIIRS %d", year)

		p, closer, err := OpenVIIRSAnnual(ctx, year, r.downloadOpts(label)...)
		if err != nil {
			// Stored unwrapped: OpenVIIRSAnnual already prefixes "atlas: ".
			r.viirsResult = openResult{err: err}
			return
		}

		r.registerCloser(closer)

		r.viirsResult = openResult{provider: p}
	})

	return r.viirsResult
}

// zenithSQM samples floor at zenith and converts back to a
// SurfaceBrightnessV, for Result.SQM — every layer here builds a scalar
// (altitude/azimuth-independent) Floor, so zenith is as good a sample
// point as any.
func zenithSQM(floor skybrightness.Floor) (skybrightness.SurfaceBrightnessV, error) {
	nl, err := floor.Radiance(coord.NewAltAz(angle.Deg(90), angle.Zero()), nil)
	if err != nil {
		return 0, fmt.Errorf("atlas: sample resolved floor: %w", err)
	}

	return nl.SurfaceBrightnessV(), nil
}
