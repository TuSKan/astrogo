package plan

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/constellation"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/parallel"
	"github.com/TuSKan/astrogo/logging"
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
	// 12° away — may be washed out"), empty otherwise.
	//
	// A heuristic on Moon separation and illumination, not a sky-brightness
	// model: it does not know the observer's air, where the Milky Way is, or
	// whether there is a city over the hill. A caller wanting the real
	// number evaluates
	// [github.com/TuSKan/astrogo/skybrightness/dataset.Sky] at this object's
	// Target and Windows and compares surface brightness themselves. There is
	// no constraint or scorer wired to it here, deliberately: turning a sky
	// radiance into a limiting magnitude needs a defensible detection model,
	// and that is gated behind the same phases the skybrightness roadmap
	// gates it behind.
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
// remote.EnableDownloads(maxSize, remote.NAIFSPK) consent as any other —
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
// remote.EnableDownloads(..., remote.JPLHorizonsSPK) consent, no file
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
// SPK kernel (gated by remote.EnableDownloads(..., remote.JPLHorizonsSPK))
// only when the candidate has no published elements or an orbit two-body
// propagation can't represent; see WithSmallBodyKernels to force the
// kernel path unconditionally. A candidate whose ephemeris (either path)
// can't be obtained is skipped, not treated as fatal.
//
// # The result can be incomplete, and says so
//
// That skip covers a network failure as well as a missing orbit. A kernel
// fetch that times out, or JPL being unreachable, removes that body's moons or
// that small body from the result — so a short list can mean "nothing
// qualified" or "not everything could be checked", and those are different
// answers.
//
// Skipping is deliberate: a partial sky is more useful than an error for a
// planning query, and one unreachable kernel should not cost the caller the
// other forty candidates. Staying quiet about it was not. So both values are
// returned together: the candidates that did qualify, and an error wrapping
// [ErrIncomplete] naming everything that could not be evaluated and why.
//
//	objects, err := plan.VisibleTonight(...)
//	if errors.Is(err, plan.ErrIncomplete) {
//		// objects is usable but not exhaustive; err says what is missing.
//	} else if err != nil {
//		return err
//	}
//
// A caller who wants the old behaviour ignores an [ErrIncomplete] error; one
// who needs certainty treats it as fatal. Both are now possible, which is the
// point — a Warn log line, which is all this used to emit, is not something a
// program can branch on. Every other error return is fatal and comes with no
// results, so ignoring [ErrIncomplete] specifically is safe.
//
// To avoid the situation rather than detect it, pre-seed the kernels the query
// depends on; remote.SetOffline(true) makes the degradation deterministic
// rather than dependent on the network.
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

	// Everything dropped because it could not be evaluated, rather than because
	// it was evaluated and rejected. See ErrIncomplete.
	var dropped skips

	candidates := gatherCandidates(ctx, gatherBrightTargets(ctx, brightSources, magLimit, &dropped), start, end, cfg, &dropped)
	candidates = append(candidates, gatherSolarSystemCandidates(planetProvider, mid, magLimit)...)

	if cfg.includeMoons {
		moonCandidates, moonProviders := gatherPlanetaryMoons(ctx, mid, magLimit, &dropped)
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
	type evalResult struct {
		vo   VisibleObject
		name string
		ok   bool
		err  error
	}

	// The skip reason travels in the result rather than through parallel.Map's
	// own error return: that is an errgroup, so it keeps only the first error
	// and cancels the rest, and one candidate failing must not stop the others.
	evaluated, _ := parallel.Map(candidates, 0, func(_ int, c visibleCandidate) (evalResult, error) {
		vo, ok, err := evaluateCandidate(ctx, c, start, end, site, planetProvider, magLimit, cfg)

		if c.closer != nil {
			_ = c.closer.Close()
		}

		return evalResult{vo: vo, name: c.obj.Name(), ok: ok, err: err}, nil
	})

	results := make([]VisibleObject, 0, len(candidates))

	for _, r := range evaluated {
		if r.ok {
			results = append(results, r.vo)
		}

		dropped.add("candidate", r.name, r.err)
	}

	slices.SortFunc(results, func(a, b VisibleObject) int {
		return cmp.Compare(a.ApparentMag, b.ApparentMag)
	})

	return results, dropped.err()
}

// gatherBrightTargets calls SearchBright on every source concurrently and
// flattens the results — a source whose query errors is skipped, not
// treated as fatal, since the other sources' results don't depend on it.
// SIMBAD/OpenNGC/SBDB (or however many resolve.BrightObjectSearcher a
// caller registers) are otherwise independent network round trips with no
// reason to wait on each other; each source writes into its own slice
// index, so no mutex is needed to combine them afterward.
func gatherBrightTargets(ctx context.Context, sources []resolve.BrightObjectSearcher, magLimit float64, dropped *skips) []resolve.Target {
	// A source's own query error is handled per-item below (skipped, not
	// propagated), so parallel.Map's own error return is never non-nil.
	perSource, _ := parallel.Map(sources, 0, func(_ int, src resolve.BrightObjectSearcher) ([]resolve.Target, error) {
		var targets []resolve.Target

		iter := src.SearchBright(ctx, resolve.BrightRequest{MaxVMag: magLimit})
		iter(func(tgt resolve.Target, err error) bool {
			if err != nil {
				// One bad row does not end the source, but the caller is told
				// their list is short by however many of these there were.
				dropped.add("bright source", fmt.Sprintf("%T", src), err)

				return true
			}

			targets = append(targets, tgt)

			return true
		})

		return targets, nil
	})

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
func gatherCandidates(ctx context.Context, targets []resolve.Target, start, end time.Time, cfg visibleTonightConfig, dropped *skips) []visibleCandidate {
	slots := make([]visibleCandidate, len(targets))

	var smallBodyIdx []int

	for i, tgt := range targets {
		if !needsSmallBodyEphemeris(tgt.Kind) {
			obj, closer, err := candidateFromTarget(ctx, tgt, start, end, cfg)
			dropped.add("target", tgt.Name, err)

			if obj != nil {
				slots[i] = visibleCandidate{obj: obj, target: tgt, closer: closer}
			}

			continue
		}

		smallBodyIdx = append(smallBodyIdx, i)
	}

	// Only the small-body subset goes through parallel.Map, bounded by
	// maxConcurrentEphemerisFetches — the fast local candidates above need
	// no such cap and stay fully synchronous, avoiding goroutine dispatch
	// overhead for the common case. A candidate's own fetch failure is a
	// skip, not a group-wide failure, so it goes to dropped rather than out
	// through Map's error return — that is an errgroup's, which would cancel
	// the other seven fetches over one unreachable body.
	results, _ := parallel.Map(smallBodyIdx, maxConcurrentEphemerisFetches, func(_ int, idx int) (visibleCandidate, error) {
		tgt := targets[idx]
		obj, closer, err := candidateFromTarget(ctx, tgt, start, end, cfg)
		dropped.add("small body", tgt.Name, err)

		if obj != nil {
			return visibleCandidate{obj: obj, target: tgt, closer: closer}, nil
		}

		return visibleCandidate{}, nil
	})

	for j, idx := range smallBodyIdx {
		slots[idx] = results[j]
	}

	candidates := make([]visibleCandidate, 0, len(slots))

	for _, c := range slots {
		if c.obj != nil {
			candidates = append(candidates, c)
		}
	}

	return candidates
}

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
//
// The error says why no candidate came back, so a caller can tell a target
// that could not be fetched from one that simply has no ephemeris to fetch.
// A nil Observable with a nil error is the latter: FromCatalog produced a
// fixed target where a small body was expected, which is a property of the
// catalogue row, not a failure.
func candidateFromTarget(ctx context.Context, tgt resolve.Target, start, end time.Time, cfg visibleTonightConfig) (Observable, eph.Provider, error) {
	if !needsSmallBodyEphemeris(tgt.Kind) {
		// A target the resolver returned with no position at all is
		// dropped rather than placed at RA 0, Dec 0. Both callers already
		// treat a nil Observable as "no candidate".
		obj, err := FromCatalog(tgt, nil)
		if err != nil {
			return nil, nil, err
		}

		return obj, nil, nil
	}

	// Kepler first: try FromCatalog's own elements-based construction
	// (no provider passed) before ever reaching for a kernel — see
	// WithSmallBodyKernels' doc comment for the full rationale. Falls
	// through to the kernel path below when elements weren't published,
	// are hyperbolic/parabolic, or the caller forced kernels via
	// WithSmallBodyKernels.
	if !cfg.forceSmallBodyKernels && tgt.HasElements {
		if obj, err := FromCatalog(tgt, nil); err == nil && !isFixedTarget(obj) {
			return obj, nil, nil
		}
	}

	minorProvider, err := eph.NewProvider(ctx, eph.SmallBody, tgt.SPKID,
		eph.WithTimeInterval(start.Add(-coverageMargin), end.Add(coverageMargin)))
	if err != nil {
		return nil, nil, fmt.Errorf("plan: small-body ephemeris for %q: %w", tgt.Name, err)
	}

	obj, err := FromCatalog(tgt, minorProvider)
	if err != nil {
		_ = minorProvider.Close()

		return nil, nil, err
	}

	return obj, minorProvider, nil
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
func evaluateCandidate(ctx context.Context, c visibleCandidate, start, end time.Time, site *Site, planetProvider eph.Provider, magLimit float64, cfg visibleTonightConfig) (VisibleObject, bool, error) {
	obj := c.obj

	// skipped reports a candidate dropped because it could not be evaluated,
	// as distinct from one evaluated and found wanting.
	//
	// VisibleTonight's contract is to skip rather than fail — one unreachable
	// kernel should not cost the caller the other forty candidates, and its
	// doc comment says so. But every one of these used to return the same
	// bare false as "too faint" or "never rises", so a night's list could come
	// back short because JPL was down and nothing said which it was.
	//
	// Warn, not Info: the result is quietly less complete than it looks, which
	// is exactly what that level is for. See the logging package.
	skipped := func(stage string, err error) (VisibleObject, bool, error) {
		logging.WarnContext(ctx, "candidate skipped: could not evaluate",
			"target", obj.Name(), "stage", stage, "err", err)

		return VisibleObject{}, false, fmt.Errorf("%s: %w", stage, err)
	}

	windows, err := ObservableWindows(obj, start, end, cfg.step, site, Altitude{Threshold: cfg.minAltitude})
	if err != nil {
		return skipped("observable windows", err)
	}

	if len(windows) == 0 {
		return VisibleObject{}, false, nil // evaluated: never clears the horizon
	}

	events, err := VisibilityEvents(start, end, obj, site)
	if err != nil {
		return skipped("visibility events", err)
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
	if err != nil {
		return skipped("transit estimate", err)
	}

	if peakTime.IsZero() {
		return VisibleObject{}, false, nil // evaluated: no maximum inside the window
	}

	pos, err := obj.Position(peakTime)
	if err != nil {
		return skipped("position at peak", err)
	}

	astroCtx := coord.NewContext(peakTime, site.Location(), site.Refraction())

	aa, err := observedAltAz(obj, peakTime, astroCtx, pos)
	if err != nil {
		return skipped("observed alt/az", err)
	}

	vo.PeakTime = peakTime
	vo.PeakAltitude = aa.Alt()
	vo.PeakAzimuth = aa.Az()
	vo.Direction = coord.CompassDirection(aa.Az())

	rawMag, ok, err := rawMagnitude(obj, peakTime)
	if err != nil {
		return skipped("apparent magnitude", err)
	}

	if !ok {
		return VisibleObject{}, false, nil // evaluated: no published magnitude
	}

	airmass, err := atmosphere.Airmass(aa.Alt())
	if err != nil {
		return skipped("airmass", err)
	}

	vo.ApparentMag = magnitude.StarApparent(rawMag, airmass)
	if vo.ApparentMag >= magLimit {
		return VisibleObject{}, false, nil
	}

	if full, abbr, err := constellation.Lookup(pos); err == nil {
		vo.Constellation, vo.ConstellationAbbr = full, abbr
	}

	if obj.Name() != "Moon" {
		vo.SkyNote = moonNote(planetProvider, peakTime, pos)
	}

	return vo, true, nil
}

// rawMagnitude returns obj's magnitude before atmospheric extinction —
// via MagnitudeComputer (Planet/Asteroid/Comet, dynamic photometry) or
// StaticMagnitude (Star/DeepSkyObject, fixed catalog VMag).
//
// The three outcomes are distinct and stay distinct: a magnitude; ok false
// with a nil error, meaning obj publishes none (it exposes neither interface,
// or its catalog row carried no VMag); and a non-nil error, meaning it has one
// but the photometry could not be computed — an ephemeris lookup that failed
// under it. Returning that last case as a bare false would drop the object
// exactly like one too faint to make the cut, which is how it read before.
func rawMagnitude(obj Observable, t time.Time) (mag float64, ok bool, err error) {
	if mc, isMC := obj.(MagnitudeComputer); isMC {
		m, err := mc.ApparentMagnitude(t)
		if err != nil {
			return 0, false, fmt.Errorf("plan: apparent magnitude of %q: %w", obj.Name(), err)
		}

		return m, true, nil
	}

	if sm, isSM := obj.(StaticMagnitude); isSM {
		m, has := sm.StaticMagnitude()

		return m, has, nil
	}

	return 0, false, nil
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
