package plan

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/constellation"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
)

// ErrNoTwilight is returned when site/night never reaches astronomical
// twilight within a 24-hour span (e.g. high-latitude summer) — "tonight"
// as this function defines it doesn't exist for that combination.
var ErrNoTwilight = errors.New("plan: no astronomical twilight found in the given night")

// VisibleObject is one object confirmed visible tonight, fully described.
// Target is reused wholesale (ID, Name, Kind, Catalog, Coord, VMag, H/G/M1/K1
// for minor bodies, Aliases, Provenance) rather than re-declaring fields
// this codebase already has a canonical home for.
type VisibleObject struct {
	Target resolve.Target
	// Constellation/ConstellationAbbr are computed at PeakTime.
	Constellation     string
	ConstellationAbbr string
	// ApparentMag is extinction-adjusted (via atmosphere.Airmass +
	// magnitude.StarApparent's generic linear-extinction model — the
	// physics doesn't care whether the photons came from a star, planet,
	// asteroid, or comet), evaluated at PeakTime.
	ApparentMag float64
	// RiseTime/TransitTime/SetTime are the real geometric event instants
	// within [start, end] — each is zero (time.Time{}) if that event
	// wasn't found tonight. A circumpolar object never sets; an object
	// already up at dusk has no Rise inside the window; TransitTime in
	// particular is zero whenever the object's true meridian crossing
	// falls outside [start, end] even though part of its arc doesn't — a
	// real, common case (e.g. an object rising just before dawn), not an
	// error. Use PeakTime for a value that's always populated whenever
	// the object is visible at all.
	RiseTime    time.Time
	TransitTime time.Time
	SetTime     time.Time
	// PeakTime/PeakAltitude/PeakAzimuth/Direction describe the best
	// moment to actually look for this object tonight: the real maximum
	// altitude reached within its first horizon-clearing window
	// (Windows[0]), found via TransitEstimate's Brent's-method numerical
	// optimum rather than an approximation. This coincides with
	// TransitTime whenever the true transit falls inside that window,
	// and stands in for it (the best available instant) otherwise —
	// PeakTime is never left at a zero value for a visible object.
	// ApparentMag/Constellation/SkyNote are all evaluated at PeakTime,
	// since that's the instant they're actually meaningful for. Direction
	// is PeakAzimuth rendered as a 16-point compass label (e.g. "SSW") —
	// see coord.CompassDirection.
	PeakTime     time.Time
	PeakAltitude angle.Angle
	PeakAzimuth  angle.Angle
	Direction    string
	// Windows is every interval tonight the object clears MinAltitude —
	// usually one, but real for circumpolar/multi-window edge cases. Peak*
	// above is always computed within Windows[0] specifically.
	Windows []Window
	// SkyNote is a Moon-proximity advisory (e.g. "Moon 87% illuminated,
	// 12° away — may be washed out"), empty otherwise. This is a
	// heuristic, not the full skybrightness-package light-pollution model
	// (which needs a caller-supplied light-pollution grid) — a caller
	// wanting that should layer plan.LimitingMagnitudeConstraint/
	// ScoreObservableSky themselves using this object's Target/Windows.
	SkyNote string
}

type visibleTonightConfig struct {
	minAltitude           angle.Angle
	step                  time.Duration
	includeMoons          bool
	forceSmallBodyKernels bool
}

// VisibleTonightOption configures VisibleTonight.
type VisibleTonightOption func(*visibleTonightConfig)

// WithMinAltitude overrides the minimum altitude an object must clear to
// count as "visible" tonight. Default: site.RiseSetThreshold() (geometric
// horizon plus standard refraction/dip — the same bar VisibilityEvents
// itself uses for rise/set).
func WithMinAltitude(alt angle.Angle) VisibleTonightOption {
	return func(c *visibleTonightConfig) { c.minAltitude = alt }
}

// WithStep overrides the sampling cadence ObservableWindows uses to find
// horizon-clearing intervals. Default: 10 minutes.
func WithStep(d time.Duration) VisibleTonightOption {
	return func(c *visibleTonightConfig) { c.step = d }
}

// WithPlanetaryMoons enables the 21 major, IAU-named natural satellites in
// planetaryMoons (Io, Titan, Triton, ...) as candidates, off by default.
// This is an explicit opt-in, not a size-based default, because the SPK
// kernels these moons need are far larger than everything else
// VisibleTonight downloads — from ~64 MB (Mars) to ~1.1 GB (Jupiter's
// Galilean moons), since NAIF's only kernels covering these bright, named
// moons also carry very long high-precision integration spans; there is no
// smaller official alternative. Each kernel still requires the same
// remote.EnableDownloads(remote.NAIFSPK, maxSize) consent as any other —
// this option only controls whether VisibleTonight asks for them at all.
func WithPlanetaryMoons() VisibleTonightOption {
	return func(c *visibleTonightConfig) { c.includeMoons = true }
}

// WithSmallBodyKernels forces every asteroid/comet/dwarf-planet/
// interstellar candidate to use a real JPL-Horizons-generated SPK
// kernel, instead of the default: two-body Keplerian propagation
// (ephemeris/kepler) from the candidate's own published elements when
// SBDB provided them (resolve.Target.HasElements).
//
// The default is Kepler because it is free — no network round trip, no
// remote.EnableDownloads(remote.JPLHorizons, ...) consent, no file
// handle — and, for a single night's visibility-window search, accurate
// well beyond what that search itself resolves (~0.04″ near the
// elements' own epoch, ~0.56″ at 30 days out — see CHANGELOG for the
// live 433 Eros validation). A candidate with no published elements, or
// whose orbit is hyperbolic/parabolic (every KindInterstellar object,
// and near-parabolic comets — two-body propagation cannot represent
// either), still takes the kernel path regardless of this option.
//
// Use this when real, perturbed, kernel-backed positions matter more
// than the network/consent cost: astrometry, occultation prediction,
// close-approach work, or any epoch far from the elements' own.
func WithSmallBodyKernels() VisibleTonightOption {
	return func(c *visibleTonightConfig) { c.forceSmallBodyKernels = true }
}

// visibleCandidate pairs a constructed Observable with the resolve.Target
// describing it — Observable alone loses the catalog fields (Coord, VMag,
// H/G/M1/K1, Aliases, Provenance) VisibleObject.Target is supposed to
// carry, so this keeps both wired together through the pipeline instead of
// trying to recover the Target from the Observable afterward.
type visibleCandidate struct {
	obj    Observable
	target resolve.Target
	// closer is non-nil only for asteroid/comet candidates — each owns a
	// real per-body JPL Horizons SPK kernel (opened by candidateFromTarget)
	// that must be released once this candidate has been evaluated, rather
	// than held open (and, on Windows, locked on disk) for the rest of the
	// process's lifetime.
	closer eph.Provider
}

// naked-eye planets, Mercury through Neptune, plus Pluto — Uranus/Neptune/
// Pluto are included on the same footing as everything else (never
// hardcoded out): at typical magLimit values they're excluded by their own
// faintness (~5.7/7.8/~14-16), not by a special case here. Pluto's Kind is
// reported as resolve.KindDwarfPlanet, not resolve.KindPlanet — see
// gatherSolarSystemCandidates.
var planetConstructors = []func(eph.Provider) *Planet{
	NewMercury, NewVenus, NewMars, NewJupiter, NewSaturn, NewUranus, NewNeptune, NewPluto,
}

// VisibleTonight finds every known object brighter than magLimit that
// clears the horizon at some point tonight (astronomical dusk to the
// following astronomical dawn) from site.
//
// night should be an instant earlier in the same calendar day than dusk
// (local noon or midnight both work) — VisibleTonight searches
// [night, night+24h] for tonight's dusk, then independently searches
// starting at that dusk for the following dawn (not the same window both
// searches share), so the result is always correctly ordered regardless of
// whether night falls before or after that morning's own dawn.
//
// Stars and deep-sky objects come from brightSources — any
// resolve.BrightObjectSearcher (catalog/simbad, catalog/openngc, and
// catalog/sbdb's Provider all implement it structurally; pass whichever
// combination you want included). The Moon and naked-eye planets come from
// planetProvider — a nil planetProvider falls back to ephemeris.Default()
// (SOFA-analytic, offline, covers Sun/Moon/Mercury-Neptune/Pluto). For
// higher-fidelity, perturbation-aware planet positions over longer spans,
// pass a real kernel-backed provider instead — ephemeris.NewProvider(ctx,
// ephemeris.Planets, "de440s") or similar.
//
// magLimit governs every category uniformly, including asteroids/comets
// surfaced by an SBDB-backed brightSources entry: at a tight limit (e.g. 2)
// essentially none qualify (no known asteroid has ever been observed
// brighter than ~mag 5.1), but at a looser one (e.g. 5) real candidates
// near a favorable opposition legitimately appear — a property of the
// physics, not a special case in this function. Each such candidate is
// resolved via two-body Keplerian propagation of its own published
// elements by default (ephemeris/kepler) — free, no network round trip,
// no download consent — falling back to a real JPL-Horizons-generated
// SPK kernel (gated by remote.EnableDownloads(remote.JPLHorizons, ...))
// only when the candidate has no published elements or an orbit two-body
// propagation can't represent; see WithSmallBodyKernels to force the
// kernel path unconditionally. A candidate whose ephemeris (either path)
// can't be obtained is skipped, not treated as fatal.
func VisibleTonight(
	ctx context.Context,
	site *Site,
	night time.Time,
	magLimit float64,
	brightSources []resolve.BrightObjectSearcher,
	planetProvider eph.Provider,
	opts ...VisibleTonightOption,
) ([]VisibleObject, error) {
	if planetProvider == nil {
		planetProvider = eph.Default()
	}

	cfg := visibleTonightConfig{
		minAltitude: site.RiseSetThreshold(),
		step:        10 * time.Minute,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	// AstronomicalDawnDusk finds the first dawn and first dusk independently
	// within [start, end) — not "tonight's dusk paired with the dawn that
	// follows it". Searching [night, night+24h) directly for both, as an
	// earlier version of this function did, breaks the moment night is
	// midnight (a documented valid choice): the first dawn in that window
	// is that same morning's, which is BEFORE dusk, not after it — an
	// inverted [start, end) that silently makes every candidate evaluate
	// as never visible (confirmed via live testing: every one of ~150 real
	// candidates, including Sirius, was rejected). Two searches fix this
	// structurally regardless of which instant night is: first find dusk
	// anywhere in the caller's 24h window, then search for dawn starting
	// exactly at dusk.Time — guaranteeing dawn is always after dusk.
	_, dusk, err := AstronomicalDawnDusk(night, night.AddDate(0, 0, 1), site, planetProvider)
	if err != nil {
		return nil, fmt.Errorf("plan: visible tonight: twilight: %w", err)
	}

	if dusk == nil {
		return nil, ErrNoTwilight
	}

	dawn, _, err := AstronomicalDawnDusk(dusk.Time, dusk.Time.AddDate(0, 0, 1), site, planetProvider)
	if err != nil {
		return nil, fmt.Errorf("plan: visible tonight: twilight: %w", err)
	}

	if dawn == nil {
		return nil, ErrNoTwilight
	}

	start, end := dusk.Time, dawn.Time
	mid := start.Add(end.Sub(start) / 2)

	candidates := gatherCandidates(ctx, gatherBrightTargets(ctx, brightSources, magLimit), start, end, cfg)
	candidates = append(candidates, gatherSolarSystemCandidates(planetProvider, mid, magLimit)...)

	if cfg.includeMoons {
		moonCandidates, moonProviders := gatherPlanetaryMoons(ctx, mid, magLimit)
		candidates = append(candidates, moonCandidates...)

		defer func() {
			for _, p := range moonProviders {
				_ = p.Close()
			}
		}()
	}

	// evaluateCandidate is pure CPU/in-memory work by this point (every
	// network fetch already happened in gatherCandidates) — window search,
	// a TransitEstimate solve, and a handful of coordinate transforms per
	// candidate, fully independent across candidates. Unlike
	// gatherCandidates' network fetches, there's no external server to be
	// considerate of here, so this uses every core rather than a small
	// fixed bound.
	evaluated := make([]VisibleObject, len(candidates))
	visible := make([]bool, len(candidates))

	g := new(errgroup.Group)
	g.SetLimit(runtime.GOMAXPROCS(0))

	for i, c := range candidates {
		g.Go(func() error {
			vo, ok := evaluateCandidate(c, start, end, site, planetProvider, magLimit, cfg)
			evaluated[i], visible[i] = vo, ok

			if c.closer != nil {
				_ = c.closer.Close()
			}

			return nil // evaluateCandidate reports failure via ok, not an error — never fails the group
		})
	}

	_ = g.Wait() // never returns a non-nil error — see the comment above

	results := make([]VisibleObject, 0, len(candidates))

	for i, vo := range evaluated {
		if visible[i] {
			results = append(results, vo)
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].ApparentMag < results[j].ApparentMag })

	return results, nil
}

// gatherBrightTargets calls SearchBright on every source concurrently and
// flattens the results — a source whose query errors is skipped, not
// treated as fatal, since the other sources' results don't depend on it.
// SIMBAD/OpenNGC/SBDB (or however many resolve.BrightObjectSearcher a
// caller registers) are otherwise independent network round trips with no
// reason to wait on each other; each source writes into its own slice
// index, so no mutex is needed to combine them afterward.
func gatherBrightTargets(ctx context.Context, sources []resolve.BrightObjectSearcher, magLimit float64) []resolve.Target {
	perSource := make([][]resolve.Target, len(sources))

	g := new(errgroup.Group)

	for i, src := range sources {
		g.Go(func() error {
			var targets []resolve.Target

			iter := src.SearchBright(ctx, resolve.BrightRequest{MaxVMag: magLimit})
			iter(func(tgt resolve.Target, err error) bool {
				if err == nil {
					targets = append(targets, tgt)
				}

				return true
			})

			perSource[i] = targets

			return nil // a source's own query error is already handled above — never fails the group
		})
	}

	_ = g.Wait() // never returns a non-nil error — see the comment above

	var targets []resolve.Target
	for _, ts := range perSource {
		targets = append(targets, ts...)
	}

	return targets
}

// maxConcurrentEphemerisFetches bounds how many asteroid/comet Stage-2
// SPK-kernel fetches (candidateFromTarget's network call) run at once.
// Stage 1 can legitimately surface on the order of 100 candidates (see
// catalog/sbdb.SearchBright's default Limit) — fetching those strictly
// sequentially made VisibleTonight impractically slow end-to-end (confirmed
// via live testing); firing all of them at once instead would hammer JPL
// Horizons with that many simultaneous requests, which is neither
// considerate nor reliable. 8 is a modest, bounded middle ground.
const maxConcurrentEphemerisFetches = 8

// coverageMargin pads the Horizons SPK coverage window
// (candidateFromTarget's eph.WithTimeInterval) beyond [start, end] — the
// event solver's bisection refinement can evaluate an instant just outside
// the nominal window while converging on a boundary rise/set, and this
// keeps that from tripping "no coverage for target at requested epoch".
const coverageMargin = 24 * time.Hour

// gatherCandidates converts every bright-search target into a
// visibleCandidate. Star/deep-sky targets need no I/O and are converted
// inline; asteroid/comet targets each need a real network fetch
// (candidateFromTarget's Stage 2), so those run concurrently, bounded by
// maxConcurrentEphemerisFetches — the dominant cost of a magLimit=2-style
// query is this fetch, not the CPU-bound evaluation that follows, so this
// is where concurrency actually pays off. Result order matches the input
// target order (each goroutine writes to its own slice index), independent
// of which fetch happens to finish first.
//
// start/end bound the Horizons SPK coverage window each asteroid/comet's
// Stage-2 provider is fetched for (see candidateFromTarget) — without
// this, eph.NewProvider defaults to a zero time.Time interval, which
// Horizons interprets literally (formats to a year-1 date) and so never
// covers the actual query night, a real bug confirmed via live testing:
// every asteroid/comet candidate's Stage 2 silently failed
// ("no coverage for target at requested epoch"), making the entire
// category permanently absent from every result regardless of real
// brightness.
func gatherCandidates(ctx context.Context, targets []resolve.Target, start, end time.Time, cfg visibleTonightConfig) []visibleCandidate {
	slots := make([]visibleCandidate, len(targets))

	g := new(errgroup.Group)
	g.SetLimit(maxConcurrentEphemerisFetches)

	for i, tgt := range targets {
		if !needsSmallBodyEphemeris(tgt.Kind) {
			if obj, closer := candidateFromTarget(ctx, tgt, start, end, cfg); obj != nil {
				slots[i] = visibleCandidate{obj: obj, target: tgt, closer: closer}
			}

			continue
		}

		g.Go(func() error {
			if obj, closer := candidateFromTarget(ctx, tgt, start, end, cfg); obj != nil {
				slots[i] = visibleCandidate{obj: obj, target: tgt, closer: closer}
			}

			// A candidate's own fetch failure is a skip, not a group-wide
			// failure — candidateFromTarget already reports that by
			// returning a nil Observable rather than an error, so this
			// goroutine never actually fails the group; SetLimit is the
			// only errgroup feature in play here.
			return nil
		})
	}

	_ = g.Wait() // never returns a non-nil error — see the comment above

	candidates := make([]visibleCandidate, 0, len(slots))

	for _, c := range slots {
		if c.obj != nil {
			candidates = append(candidates, c)
		}
	}

	return candidates
}

// candidateFromTarget converts one bright-search result into an Observable.
// Stars/deep-sky objects (FromCatalog's fixed-target path) need no
// ephemeris and are always converted, with a nil closer. Asteroids/comets
// need a real per-body SPK kernel fetched first — Stage 2 of the
// two-stage minor-body design (see catalog/sbdb.SearchBright's doc
// comment for Stage 1) — covering [start-coverageMargin, end+coverageMargin]
// so the fetched kernel actually spans the night being evaluated; a
// candidate whose kernel can't be fetched here returns a nil Observable,
// skipped rather than failing the whole night's query. The returned
// eph.Provider is the caller's to Close once this candidate has been
// evaluated — FromCatalog's Comet/Asteroid wrapper holds it for the
// lifetime of the evaluation, but the underlying SPK/LSK file handles must
// not outlive that.
// needsSmallBodyEphemeris reports whether kind requires a real per-body
// Horizons-generated SPK ephemeris fetch (Stage 2) rather than the plain
// FromCatalog(tgt, nil) path — asteroids, comets, dwarf planets, and
// interstellar objects are all resolved by name/designation through SBDB
// with no coordinate of their own, so all four need this. Kept as one
// function rather than repeating the Kind list at both call sites below.
func needsSmallBodyEphemeris(kind resolve.Kind) bool {
	switch kind { //nolint:exhaustive // every other Kind takes the FromCatalog(tgt, nil) path via default
	case resolve.KindAsteroid, resolve.KindComet, resolve.KindDwarfPlanet, resolve.KindInterstellar:
		return true
	default:
		return false
	}
}

// isFixedTarget reports whether obs is one of the two types
// FromCatalog's fixed-target fall-through can produce (*Star,
// *DeepSkyObject) — the signal that a preceding elements-based
// construction attempt inside FromCatalog fell through rather than
// succeeding, since a real small-body Observable is always
// *Asteroid/*Comet/*GenericBody, never one of these two.
func isFixedTarget(obs Observable) bool {
	switch obs.(type) {
	case *Star, *DeepSkyObject:
		return true
	default:
		return false
	}
}

func candidateFromTarget(ctx context.Context, tgt resolve.Target, start, end time.Time, cfg visibleTonightConfig) (Observable, eph.Provider) {
	if !needsSmallBodyEphemeris(tgt.Kind) {
		return FromCatalog(tgt, nil), nil
	}

	// Kepler first: try FromCatalog's own elements-based construction
	// (no provider passed) before ever reaching for a kernel — see
	// WithSmallBodyKernels' doc comment for the full rationale. Falls
	// through to the kernel path below when elements weren't published,
	// are hyperbolic/parabolic, or the caller forced kernels via
	// WithSmallBodyKernels.
	if !cfg.forceSmallBodyKernels && tgt.HasElements {
		if obj := FromCatalog(tgt, nil); !isFixedTarget(obj) {
			return obj, nil
		}
	}

	minorProvider, err := eph.NewProvider(ctx, eph.SmallBody, tgt.SPKID,
		eph.WithTimeInterval(start.Add(-coverageMargin), end.Add(coverageMargin)))
	if err != nil {
		return nil, nil
	}

	return FromCatalog(tgt, minorProvider), minorProvider
}

// gatherSolarSystemCandidates builds the Moon and every naked-eye planet,
// filtered by apparent magnitude at the night's midpoint (a reasonable
// single-instant approximation — these bodies' brightness changes slowly
// over one night, unlike their position). Each gets a synthetic
// resolve.Target (no catalog backs these, so there's nothing to carry
// through except identity/kind).
func gatherSolarSystemCandidates(provider eph.Provider, at time.Time, magLimit float64) []visibleCandidate {
	var out []visibleCandidate

	moon := NewMoon(provider)
	if m, err := moon.ApparentMagnitude(at); err == nil && m < magLimit {
		out = append(out, visibleCandidate{
			obj:    moon,
			target: resolve.Target{Name: moon.Name(), Kind: resolve.KindMoon, Catalog: "ephemeris"},
		})
	}

	for _, ctor := range planetConstructors {
		p := ctor(provider)

		kind := resolve.KindPlanet
		if p.Name() == "Pluto" {
			kind = resolve.KindDwarfPlanet
		}

		m, err := p.ApparentMagnitude(at)
		if err == nil && m < magLimit {
			out = append(out, visibleCandidate{
				obj:    p,
				target: resolve.Target{Name: p.Name(), Kind: kind, Catalog: "ephemeris"},
			})
		}
	}

	return out
}

// observableObject adapts an Observable to coord.Object, which
// TransitEstimate requires — Observable.Position(t) and coord.Object.ICRS(t)
// are the identical operation under different names, so this is a pure
// rename, not a behavior change.
type observableObject struct{ Observable }

func (o observableObject) ICRS(t time.Time) (coord.ICRS, error) { return o.Position(t) }

// evaluateCandidate runs the shared downstream pipeline every category goes
// through identically: horizon windows, rise/transit/set, the real best-
// observed Peak instant, extinction-adjusted magnitude at Peak, a final
// magLimit check against THAT extinction-adjusted magnitude (not the
// pre-extinction catalog/computed value), constellation, and a
// Moon-proximity note. Returns ok=false if the candidate never clears the
// horizon tonight, or its real, as-observed brightness turns out fainter
// than magLimit (not an error — most candidates in a wide magnitude search
// won't clear either bar).
//
// Checking the extinction-adjusted magnitude, not the raw one, matters for
// every category, not just asteroids/comets: a star cataloged well within
// magLimit (or a planet/Moon whose upstream ApparentMagnitude(mid) check
// in gatherSolarSystemCandidates used the night's midpoint, not this
// candidate's actual evaluation instant) can still be evaluated here near
// the horizon, where atmospheric extinction can add several magnitudes —
// confirmed via live testing: at magLimit=2, an earlier version of this
// check (against the pre-extinction value) let real results as faint as
// mag +8.5 through, since their catalog brightness alone passed even
// though their reported, as-observed ApparentMag plainly didn't. Star/
// deep-sky/planet/Moon candidates are still filtered loosely upstream
// (SearchBright's own contract, gatherSolarSystemCandidates) to avoid
// fetching/evaluating obviously-hopeless candidates at all, but this is
// the only place the bound is enforced against what's actually reported.
//
// The same reasoning applies with even more force to asteroid/comet
// candidates: catalog/sbdb.SearchBright's Stage-1 prefilter only bounds
// H/M1 loosely (several magnitudes fainter than magLimit can still pass,
// on purpose — see its doc comment), so this check, against the real
// computed-and-extinguished magnitude, is the only place that bound is
// enforced for real.
func evaluateCandidate(c visibleCandidate, start, end time.Time, site *Site, planetProvider eph.Provider, magLimit float64, cfg visibleTonightConfig) (VisibleObject, bool) {
	obj := c.obj

	windows, err := ObservableWindows(obj, start, end, cfg.step, site, Altitude{Threshold: cfg.minAltitude})
	if err != nil || len(windows) == 0 {
		return VisibleObject{}, false
	}

	events, err := VisibilityEvents(start, end, obj, site)
	if err != nil {
		return VisibleObject{}, false
	}

	vo := VisibleObject{Target: c.target, Windows: windows}

	for _, e := range events {
		switch e.Kind { //nolint:exhaustive // only rise/transit/set are relevant here
		case EventRise:
			if vo.RiseTime.IsZero() {
				vo.RiseTime = e.Time
			}
		case EventSet:
			if vo.SetTime.IsZero() {
				vo.SetTime = e.Time
			}
		case EventTransit:
			if vo.TransitTime.IsZero() {
				vo.TransitTime = e.Time
			}
		}
	}

	// PeakTime is the real maximum-altitude instant within the first
	// horizon-clearing window — a genuine numerical optimum (Brent's
	// method via TransitEstimate), not the crude window-midpoint
	// approximation an earlier version of this function used. It's
	// always populated for a visible candidate, unlike TransitTime,
	// which is legitimately zero whenever the true meridian crossing
	// falls outside [start, end].
	w := windows[0]

	peakTime, _, err := TransitEstimate(observableObject{obj}, site, w.Start, w.End)
	if err != nil || peakTime.IsZero() {
		return VisibleObject{}, false
	}

	pos, err := obj.Position(peakTime)
	if err != nil {
		return VisibleObject{}, false
	}

	astroCtx := coord.NewContext(peakTime, site.Location(), site.Atmosphere())

	aa, err := astroCtx.ICRSToAltAz(pos)
	if err != nil {
		return VisibleObject{}, false
	}

	vo.PeakTime = peakTime
	vo.PeakAltitude = aa.Alt()
	vo.PeakAzimuth = aa.Az()
	vo.Direction = coord.CompassDirection(aa.Az())

	rawMag, ok := rawMagnitude(obj, peakTime)
	if !ok {
		return VisibleObject{}, false
	}

	airmass, err := atmosphere.Airmass(aa.Alt())
	if err != nil {
		return VisibleObject{}, false
	}

	vo.ApparentMag = magnitude.StarApparent(rawMag, airmass)
	if vo.ApparentMag >= magLimit {
		return VisibleObject{}, false
	}

	if full, abbr, err := constellation.Lookup(pos); err == nil {
		vo.Constellation, vo.ConstellationAbbr = full, abbr
	}

	if obj.Name() != "Moon" {
		vo.SkyNote = moonNote(planetProvider, peakTime, pos)
	}

	return vo, true
}

// rawMagnitude returns obj's magnitude before atmospheric extinction —
// via MagnitudeComputer (Planet/Asteroid/Comet, dynamic photometry) or
// StaticMagnitude (Star/DeepSkyObject, fixed catalog VMag). ok is false if
// obj exposes neither (no known magnitude).
func rawMagnitude(obj Observable, t time.Time) (mag float64, ok bool) {
	if mc, isMC := obj.(MagnitudeComputer); isMC {
		m, err := mc.ApparentMagnitude(t)
		if err != nil {
			return 0, false
		}

		return m, true
	}

	if sm, isSM := obj.(StaticMagnitude); isSM {
		return sm.StaticMagnitude()
	}

	return 0, false
}

// moonNote returns a Moon-proximity advisory when the Moon is a
// significant fraction illuminated and angularly close to pos, empty
// otherwise. This is a heuristic capturing the single dominant real-world
// "will moonlight wash this out" factor, not the full light-pollution-
// aware skybrightness model (see VisibleObject.SkyNote's doc comment).
func moonNote(provider eph.Provider, t time.Time, pos coord.ICRS) string {
	fraction, _, err := MoonIllumination(t, provider)
	if err != nil || fraction < 0.5 {
		return ""
	}

	moonVec, err := eph.Position(provider, eph.Moon, t)
	if err != nil {
		return ""
	}

	moonICRS, err := eph.ToICRS(moonVec)
	if err != nil {
		return ""
	}

	sep := coord.Separation(pos, moonICRS)
	if sep.Degrees() > 30 {
		return ""
	}

	return fmt.Sprintf("Moon %.0f%% illuminated, %.0f° away — may be washed out", fraction*100, sep.Degrees())
}
