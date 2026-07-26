package plan

import (
	"context"

	"golang.org/x/sync/errgroup"

	eph "github.com/TuSKan/astrogo/ephemeris"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"
)

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
	naifID int32
	h      float64
}

// planetaryMoons is the fixed list of natural satellites VisibleTonight
// evaluates, one real per-planet SPK kernel per group. Kernel sizes vary
// enormously — from ~64 MB (Mars, mar099s.bsp's 1995-2050 short span) to
// ~1.1 GB (Jupiter, jup365.bsp) — since NAIF's smaller per-planet kernels
// only cover obscure irregular moonlets, not these bright, named ones.
// Nothing in this package downloads any of these without the same explicit
// remote.EnableDownloads(remote.NAIFSPK, maxSize) consent every other
// kernel in this library requires.
var planetaryMoons = []moonSpec{
	{"Phobos", "mar099s", 401, 11.8},
	{"Deimos", "mar099s", 402, 12.89},

	{"Io", "jup365", 501, -1.68},
	{"Europa", "jup365", 502, -1.41},
	{"Ganymede", "jup365", 503, -2.09},
	{"Callisto", "jup365", 504, -1.05},

	{"Mimas", "sat441", 601, 3.3},
	{"Enceladus", "sat441", 602, 2.2},
	{"Tethys", "sat441", 603, 0.7},
	{"Dione", "sat441", 604, 0.8},
	{"Rhea", "sat441", 605, 0.1},
	{"Titan", "sat441", 606, -1.2},
	{"Hyperion", "sat441", 607, 4.8},
	{"Iapetus", "sat441", 608, 1.5},

	{"Ariel", "ura184_part-3", 701, 1.45},
	{"Umbriel", "ura184_part-3", 702, 2.10},
	{"Titania", "ura184_part-3", 703, 1.02},
	{"Oberon", "ura184_part-3", 704, 1.23},
	{"Miranda", "ura184_part-3", 705, 3.6},

	{"Triton", "nep097", 801, -1.24},

	{"Charon", "plu060", 901, 1.0},
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

	for _, m := range planetaryMoons {
		if _, ok := byKernel[m.kernel]; !ok {
			kernels = append(kernels, m.kernel)
		}

		byKernel[m.kernel] = append(byKernel[m.kernel], m)
	}

	providers := make([]eph.Provider, len(kernels))

	g := new(errgroup.Group)

	for i, kernel := range kernels {
		g.Go(func() error {
			p, err := eph.NewProvider(ctx, eph.Moons, kernel)
			if err != nil {
				return nil //nolint:nilerr // this kernel's moons are simply unavailable tonight — not fatal, matching gatherCandidates' skip-on-fetch-failure convention
			}

			providers[i] = p

			return nil
		})
	}

	_ = g.Wait() // never returns a non-nil error — see the comment above

	var candidates []visibleCandidate

	var opened []eph.Provider

	for i, p := range providers {
		if p == nil {
			continue
		}

		opened = append(opened, p)

		for _, m := range byKernel[kernels[i]] {
			moon := NewAsteroid(m.name, eph.ID(m.naifID), p, WithHG(m.h, moonDefaultG)) //nolint:gosec // naifID is a fixed, small, positive NAIF body ID (401-901)

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
