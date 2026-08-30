// Package ephemeris provides celestial body positions and velocities for astronomical computing.
//
// # Architecture
//
// The top-level [NewProvider] factory creates an [Ephemeris] for a given
// [Source] and kernel identifier, routing internally to specialised
// implementations:
//
//   - [Planets], [SmallBody], [Asteroids], [Comets] — JPL SPK/LSK kernels
//   - [Satellites] — NORAD TLE/GP element sets with SGP4 propagation
//
// This mirrors the catalog package's unified [catalog.Resolver] pattern:
// users rarely need to import subpackages directly.
//
// # Quick Start
//
//	// JPL planetary ephemeris (DE442)
//	p, err := eph.NewProvider(ctx, eph.Planets, "de442")
//	if err != nil { log.Fatal(err) }
//	defer p.Close()
//	state, _ := p.State(eph.Mars, t)
//
//	// Multi-kernel (deep historical + modern)
//	p, err := eph.NewProvider(ctx, eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
//
//	// NORAD satellite (ISS)
//	sat, err := eph.NewProvider(ctx, eph.Satellites, "ISS",
//	    eph.WithTLE(line1, line2))
//
// # Default Provider
//
// The [Default] provider needs no kernel and no network. It uses algorithms
// from the IAU SOFA library (via gofaext), plus one that is not SOFA at all:
//   - Sun: IAU 2000 Earth ephemeris (Epv00). Measured worst case 0.013″.
//   - Moon: Meeus 1998 (Moon98), geocentric GCRS-like state. Worst case 10.2″.
//   - Planets: Simon et al. truncated series (Plan94). Worst case 1.5″ for
//     Mercury to 88.6″ for Saturn.
//   - Pluto: no SOFA model exists, so two-body propagation of Standish mean
//     elements. Worst case 197″ — three arcminutes, and a different order of
//     accuracy from everything above it.
//
// See "What Default is worth" below before relying on any of them.
//
// # Choosing a Provider
//
// [Default] needs no download and no consent step, at the cost of accuracy. A
// JPL kernel via [NewProvider] agrees with JPL Horizons to 2e-13 AU — 33 mm —
// but requires a one-time, consent-gated download (see the remote package doc
// for the consent gate). de440s/de440/de442/de441 do not differ from each
// other in accuracy, only in time-span coverage and file size:
//
//	Provider                     Accuracy                     Size          Download
//	Default()                    arcseconds to arcminutes,    built-in      not needed
//	                              per body — see below
//	NewProvider(…, "de440s")     2e-13 AU vs. Horizons        ~32 MB        one-time
//	NewProvider(…, "de440")      same fidelity, wider span    ~115 MB       one-time
//	NewProvider(…, "de442")      same fidelity, wider span    ~115 MB       one-time
//	NewProvider(…, "de441_…")    same fidelity, millennia     multi-GB/part one-time
//
// Default to de440s once JPL-grade positions are needed; reach for de440/de442
// only when a request spans centuries beyond de440s's coverage, and de441
// only for millennia-scale work.
//
// # What Default is worth
//
// Measured against DE440, quarterly over 1972-2100, 516 samples per body. The
// last column is the body's own apparent diameter, which is the number that
// decides whether an error matters:
//
//	Body       typical     worst     apparent disc
//	Sun          0.004″     0.013″      1920″
//	Mercury      0.21″      1.5″        5-13″
//	Venus        0.78″     10.2″        10-66″
//	Moon         2.1″      10.2″        1865″
//	Neptune      4.8″      10.7″        2.3″
//	Mars         3.8″      53.8″        4-25″
//	Jupiter     13.2″      52.7″        40″
//	Uranus      25.1″      71.9″        3.5″
//	Saturn      18.9″      88.6″        18″
//	Pluto      142″       197″          0.1″
//
// Read the last two columns together. Saturn's worst case is five times its
// own disc and Uranus's is twenty times, so [Default] cannot be relied on to
// place a planet inside a narrow field. Against a 30-60′ eyepiece, a GoTo
// pointing model, or any visibility question — rise and set, altitude,
// airmass, twilight, Moon separation — none of this is visible: 90″ moves a
// rise time by about six seconds.
//
// So: [Default] is a planning-grade provider. Use a kernel for astrometry,
// occultation timing, photometric aperture placement, or anything that has to
// land inside a slit.
//
// Two bodies deserve singling out.
//
// Pluto is a different category, roughly thirty times worse than the worst
// planet, because SOFA has no Pluto at all: it is two-body propagation of
// Standish mean elements, not an analytical series. At 3′ it will tell you the
// constellation and will not let you find the object.
//
// The Moon's 10″ is a two-hundredth of its own disc — fine for phase,
// illumination, separation and rise/set, and not fine for occultations, where
// 10″ is about twenty seconds of lunar motion.
//
// Small bodies propagated from published osculating elements (see [Elements]
// and [NewMovingBodyProvider]) hold roughly 1-4″ over a month either side of
// the elements' epoch of osculation, degrading as t² further out.
//
// Every figure here is generated, not asserted: see the ephemeris.sofa.* and
// ephemeris.kepler.* rows in docs/VALIDATION.md, each with its distribution,
// its contract, and the commit it was last verified at.
//
// # Coordinates and Frames
//
// Providers return [State] vectors (position and velocity) in astronomical units
// (AU) and AU/day. These are geocentric inertial states, typically in an
// ICRS-compatible frame.
//
// Use the [ToICRS] helper to convert Cartesian geocentric positions into
// spherical [coord.ICRS] coordinates.
//
// # Concurrency
//
// A [Provider] is expected to be safe for concurrent use, and the ones here
// are: the JPL provider guards its kernel set with a sync.RWMutex, and the
// analytical provider is an empty struct. A third-party Provider is under the
// same expectation, since a planner may call one from several goroutines.
//
// # Failure and degradation
//
// A missing kernel is an error rather than a silent gap: constructing a JPL
// provider fails with remote.ErrDownloadDenied unless the caller has granted
// download consent, and asking for a body or an epoch that a loaded kernel
// does not cover fails with jpl.ErrNoSegment. Nothing here substitutes an
// approximation for data it does not have.
package ephemeris
