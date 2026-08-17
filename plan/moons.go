package plan

import (
	"context"
	"fmt"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/parallel"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"
)

// PlanetaryMoon is a natural satellite of a planet other than Earth,
// observed via the same heliocentric H-G reflectance model plan.Asteroid
// already implements — see moonSpecs' doc comment for the per-moon H
// sourcing. It is distinct from Earth's own Moon (see NewMoon, which
// returns a *Planet): Earth's Moon uses a real lunar-phase magnitude model
// instead, completely different physics, so it deliberately keeps its own
// type rather than becoming a PlanetaryMoon.
type PlanetaryMoon struct {
	*Asteroid

	parent eph.ID
}

// NewPlanetaryMoon looks up name (matched against every moonSpecs entry's
// name, case- and space-insensitive) and builds a *PlanetaryMoon using
// provider for its ephemeris, or returns ErrUnknownPlanetaryMoon. The
// table's own H value (and the shared moonDefaultG slope) are used unless
// opts overrides them.
func NewPlanetaryMoon(name string, provider eph.Provider, opts ...AsteroidOption) (*PlanetaryMoon, error) {
	spec, ok := moonSpecs[normalizeSiteName(name)]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownPlanetaryMoon, name)
	}

	base := make([]AsteroidOption, 1, 1+len(opts))
	base[0] = WithHG(spec.h, moonDefaultG)

	return &PlanetaryMoon{
		Asteroid: NewAsteroid(spec.name, eph.ID(spec.naifID), provider, append(base, opts...)...), //nolint:gosec // naifID is a fixed, small, positive NAIF body ID (401-901)
		parent:   spec.parent,
	}, nil
}

// Parent returns the NAIF ID of the planet this moon orbits (e.g.
// eph.Jupiter for Io).
func (m *PlanetaryMoon) Parent() eph.ID { return m.parent }

// moonDefaultG is the phase-law slope parameter used for every planetary
// moon below — none of the sources this data was sourced from (Horizons'
// own OBJ_DATA physical-properties text, or Jewitt (2008) for Charon)
// publish a per-moon G, so this falls back to the same IAU default
// plan.NewAsteroid itself uses when a caller doesn't supply one.
const moonDefaultG = 0.15

// moonSpec describes one major, IAU-named natural satellite this library
// can compute apparent magnitude/position for — via the identical
// heliocentric H-G phase-law reflectance model plan.Asteroid already
// implements (a moon's brightness is sunlight reflected off an airless or
// icy body, the same physics, just with H sourced from lunar/planetary
// physical-parameter data instead of an SBDB query — there is no bulk
// "moon browse" API analogous to SBDB's asteroid/comet one).
//
// H is Horizons' own published V(1,0) absolute magnitude
// (https://ssd.jpl.nasa.gov/api/horizons.api, OBJ_DATA=YES against each
// NAIF ID below — confirmed live, not copied from a secondary source),
// with two documented exceptions:
//   - The four Galilean moons (Io/Europa/Ganymede/Callisto): Horizons
//     doesn't publish V(1,0) for these, so H is derived from their
//     well-documented, cross-checked mean opposition apparent magnitudes
//     (5.02/5.29/4.61/5.65 — matching both live Horizons ephemeris
//     photometry and independent secondary references) via
//     H = V_opposition - 5*log10(r*delta) at Jupiter's mean opposition
//     geometry (r=5.2044 AU, delta=r-1 AU), the same relation
//     magnitude.AsteroidHG reduces to at phase angle zero.
//   - Charon: Horizons doesn't publish V(1,0) for the Pluto system either;
//     H=1.0 is Jewitt (2008), "The 1000 km Scale KBOs" — cross-checked by
//     confirming it reproduces Charon's well-known real apparent magnitude
//     (~16.8) at Pluto's mean distance.
type moonSpec struct {
	name string
	// kernel is the SPK file stem under NAIF's
	// generic_kernels/spk/satellites/ directory (no base planetary kernel
	// is needed alongside it — unlike Asteroids/Comets/SmallBody, these
	// kernels already include the Sun, Earth, and the relevant planet
	// barycenter directly relative to the Solar System Barycenter,
	// confirmed against NAIF's own published segment summaries). Several
	// moons below share one kernel (e.g. all eight of Saturn's) —
	// gatherPlanetaryMoons fetches each distinct kernel only once.
	kernel string
	// parent is the NAIF ID of the planet this moon orbits (e.g.
	// eph.Jupiter for Io) — carried through to the constructed
	// *PlanetaryMoon's Parent() method.
	parent eph.ID
	naifID int32
	h      float64
}

// moonSpecs is the fixed table of natural satellites VisibleTonight
// evaluates, keyed by lowercase name, one real per-planet SPK kernel per
// group. Kernel sizes vary enormously — from ~64 MB (Mars, mar099s.bsp's
// 1995-2050 short span) to ~1.1 GB (Jupiter, jup365.bsp) — since NAIF's
// smaller per-planet kernels only cover obscure irregular moonlets, not
// these bright, named ones. Nothing in this package downloads any of these
// without the same explicit remote.EnableDownloads(maxSize, remote.NAIFSPK)
// consent every other kernel in this library requires. See NewPlanetaryMoon
// for name-based lookup.
var moonSpecs = map[string]moonSpec{
	"phobos": {name: "Phobos", kernel: "mar099s", parent: eph.Mars, naifID: 401, h: 11.8},
	"deimos": {name: "Deimos", kernel: "mar099s", parent: eph.Mars, naifID: 402, h: 12.89},

	"io":       {name: "Io", kernel: "jup365", parent: eph.Jupiter, naifID: 501, h: -1.68},
	"europa":   {name: "Europa", kernel: "jup365", parent: eph.Jupiter, naifID: 502, h: -1.41},
	"ganymede": {name: "Ganymede", kernel: "jup365", parent: eph.Jupiter, naifID: 503, h: -2.09},
	"callisto": {name: "Callisto", kernel: "jup365", parent: eph.Jupiter, naifID: 504, h: -1.05},

	"mimas":     {name: "Mimas", kernel: "sat441", parent: eph.Saturn, naifID: 601, h: 3.3},
	"enceladus": {name: "Enceladus", kernel: "sat441", parent: eph.Saturn, naifID: 602, h: 2.2},
	"tethys":    {name: "Tethys", kernel: "sat441", parent: eph.Saturn, naifID: 603, h: 0.7},
	"dione":     {name: "Dione", kernel: "sat441", parent: eph.Saturn, naifID: 604, h: 0.8},
	"rhea":      {name: "Rhea", kernel: "sat441", parent: eph.Saturn, naifID: 605, h: 0.1},
	"titan":     {name: "Titan", kernel: "sat441", parent: eph.Saturn, naifID: 606, h: -1.2},
	"hyperion":  {name: "Hyperion", kernel: "sat441", parent: eph.Saturn, naifID: 607, h: 4.8},
	"iapetus":   {name: "Iapetus", kernel: "sat441", parent: eph.Saturn, naifID: 608, h: 1.5},

	"ariel":   {name: "Ariel", kernel: "ura184_part-3", parent: eph.Uranus, naifID: 701, h: 1.45},
	"umbriel": {name: "Umbriel", kernel: "ura184_part-3", parent: eph.Uranus, naifID: 702, h: 2.10},
	"titania": {name: "Titania", kernel: "ura184_part-3", parent: eph.Uranus, naifID: 703, h: 1.02},
	"oberon":  {name: "Oberon", kernel: "ura184_part-3", parent: eph.Uranus, naifID: 704, h: 1.23},
	"miranda": {name: "Miranda", kernel: "ura184_part-3", parent: eph.Uranus, naifID: 705, h: 3.6},

	"triton": {name: "Triton", kernel: "nep097", parent: eph.Neptune, naifID: 801, h: -1.24},

	"charon": {name: "Charon", kernel: "plu060", parent: eph.Pluto, naifID: 901, h: 1.0},
}

// gatherPlanetaryMoons builds every moon in planetaryMoons, filtered by
// apparent magnitude at the night's midpoint like every other Solar System
// candidate (gatherSolarSystemCandidates). Moons sharing one SPK kernel
// (e.g. Saturn's eight) share one fetched eph.Provider, opened once per
// distinct kernel rather than once per moon — kernels this large are worth
// not fetching eight times over. Distinct kernels are fetched concurrently.
//
// The opened providers are returned separately rather than through
// visibleCandidate.closer: several candidates can share one of these
// providers, and VisibleTonight's per-candidate evaluation runs
// concurrently, so closing a shared provider the moment just one of its
// moons finishes evaluating would break the others still using it. The
// caller must Close every returned provider once ALL candidates (not just
// these) have finished evaluating.
func gatherPlanetaryMoons(ctx context.Context, at time.Time, magLimit float64) ([]visibleCandidate, []eph.Provider) {
	byKernel := make(map[string][]moonSpec)

	var kernels []string

	for _, m := range moonSpecs {
		if _, ok := byKernel[m.kernel]; !ok {
			kernels = append(kernels, m.kernel)
		}

		byKernel[m.kernel] = append(byKernel[m.kernel], m)
	}

	// parallel.Map's own error return is never non-nil here: a failed
	// kernel fetch yields a nil provider (kept in the slice, filtered
	// below) rather than a hard error, matching gatherCandidates' own
	// skip-on-fetch-failure convention -- this kernel's moons are simply
	// unavailable tonight, not fatal to the others.
	providers, _ := parallel.Map(kernels, 0, func(_ int, kernel string) (eph.Provider, error) {
		p, err := eph.NewProvider(ctx, eph.Moons, kernel)
		if err != nil {
			return nil, nil //nolint:nilerr,nilnil // see comment above: a failed kernel fetch is a documented skip, not an error
		}

		return p, nil
	})

	var candidates []visibleCandidate

	var opened []eph.Provider

	for i, p := range providers {
		if p == nil {
			continue
		}

		opened = append(opened, p)

		for _, m := range byKernel[kernels[i]] {
			moon, err := NewPlanetaryMoon(m.name, p)
			if err != nil {
				continue // m.name always resolves against moonSpecs — defense in depth only
			}

			mag, err := moon.ApparentMagnitude(at)
			if err == nil && mag < magLimit {
				candidates = append(candidates, visibleCandidate{
					obj:    moon,
					target: resolve.Target{Name: m.name, Kind: resolve.KindPlanetaryMoon, Catalog: "ephemeris"},
				})
			}
		}
	}

	return candidates, opened
}
