package plan

import (
	"fmt"

	"strconv"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// starAtJ2000 returns c's position at J2000, carrying c's own proper
// motion, parallax and radial velocity, as a [Star] takes it.
//
// A Star has no epoch: its position goes to SOFA's catalog-place transform,
// which reads it as the position at J2000.0 and moves it from there. A
// catalog's position is at the catalog's own epoch, J2016.0 for Gaia DR3,
// and was handed over as it stood until #635, so a Gaia-built star was moved
// as though its 2016 position were its 2000 one: Barnard's star 166″ off.
// A zero Epoch means J2000, as everywhere else.
func starAtJ2000(c catalog.Target) (coord.ICRS, error) {
	pos := coord.NewICRSWithKinematics(c.Coord.RA(), c.Coord.Dec(), c.PmRA, c.PmDec, c.Parallax, c.RadialVelocity)

	moved, err := coord.PropagateEpoch(pos, c.Epoch, time.J2000())
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("position at J2000: %w", err)
	}

	return moved, nil
}

// FromCatalog converts a catalog.Target (wire format from resolvers) and an
// optional ephemeris provider into the appropriate concrete Observable type.
//
// Routing logic:
//   - Satellite TLE → *Satellite (needs a real TLE-backed provider; p == nil never matches)
//   - Sun/Moon/planets (Planet/Moon/Star Kind with a major-body NAIF ID) → *Planet, always —
//     falls back to eph.Default() when p is nil, so this path never needs a caller-supplied
//     provider to produce a real, moving Observable
//   - PlanetaryMoon (a name plan.NewPlanetaryMoon recognizes) with provider → *PlanetaryMoon
//   - HasM1 (comet photometry) with provider → *Comet
//   - HasH (asteroid photometry) with provider → *Asteroid
//   - HasElements (published orbital elements) and no provider → *Comet/*Asteroid via a
//     Kepler-propagated provider built from those elements (see eph.NewFromElements) —
//     preferred over eph.Default() here, which has no data for an arbitrary small body.
//     An open orbit (e >= 1) is built from its perihelion form (eph.ElementsFromPerihelion)
//   - Star kind → *Star
//   - Everything else → *DeepSkyObject
func FromCatalog(c catalog.Target, p eph.Provider) (Observable, error) {
	// ── A NAIF major body, as catalog/jpl reports one ──
	//
	// Its SPK-ID is a NAIF code, as an eph.ID is (#809), and is translated
	// only to the point plan observes for that body: a planet's system
	// barycenter from Mars out (see majorBodyForNAIF).
	if naif, ok := naifMajorRangeID(c.SPKID); ok {
		if body, ok := majorBodyForNAIF(naif); ok {
			return NewPlanet(c.Name, body, p), nil
		}

		if p != nil {
			if m, err := NewPlanetaryMoon(c.Name, p); err == nil {
				return m, nil
			}
		}

		if !c.HasCoord {
			return nil, fmt.Errorf("%w: %s is NAIF body %d, which astrogo has no ephemeris for", ErrNoCoordinates, c.Name, naif)
		}
	}

	id, idOK := ephIDOf(c)

	// ── Satellite ──
	//
	// Its ID is a NORAD catalog number, which names it as NAIF does,
	// −100000 − n (#809). Read as an eph.ID, 25544 was a body no satellite
	// provider answers for, so every position of the ISS failed.
	if c.Kind == "Satellite" && p != nil {
		if norad, err := strconv.Atoi(c.ID); err == nil {
			if sid, ok := eph.SatelliteID(norad); ok {
				return NewSatellite(c.Name, sid, p), nil
			}
		}
	}

	// ── Sun/Moon/planets — always have a real ephemeris, provider or not ──
	//
	// majorBodyForNAIF(id) means this target really is a major named body (the
	// resolver tags the Sun with KindStar, not KindPlanet, hence checking
	// all three Kinds here). NewPlanet itself defaults a nil provider to
	// eph.Default() — which answers every one of these bodies (Sun, Moon,
	// Mercury-Neptune, Pluto, the Solar System Barycenter) — so a caller
	// who didn't supply a provider still gets a real, moving *Planet
	// instead of silently degrading to a static, non-moving fixed-target
	// Observable built from whatever (or no) fixed coordinate happened to
	// be on c.Coord. A caller-supplied provider (a real kernel, for
	// perturbation-aware accuracy) always takes precedence, unchanged.
	if c.Kind == resolve.KindPlanet || c.Kind == resolve.KindMoon || c.Kind == resolve.KindStar {
		if body, ok := majorBodyForNAIF(int(id)); idOK && ok {
			return NewPlanet(c.Name, body, p), nil
		}
	}

	// ── Moving body with provider ──
	if p != nil {
		// PlanetaryMoon — resolve.KindPlanetaryMoon-tagged candidates (e.g.
		// from plan/moons.go's gatherPlanetaryMoons) round-tripped through
		// FromCatalog used to silently degrade to *GenericBody here, losing
		// the H-G photometry NewPlanetaryMoon already knows how to build.
		// An unrecognized name (not in plan's fixed moon table) falls
		// through to the generic path below rather than failing, since
		// FromCatalog has no error return.
		if c.Kind == resolve.KindPlanetaryMoon {
			if m, err := NewPlanetaryMoon(c.Name, p); err == nil {
				return m, nil
			}
		}

		// Comet
		if c.HasM1 {
			return newCometFromTarget(c, id, p), nil
		}

		// Asteroid
		if c.HasH {
			return NewAsteroid(c.Name, id, p, asteroidOptsFrom(c)...), nil
		}

		// Generic moving body (unknown sub-type) — no photometric model.
		return NewGenericBody(c.Name, id, p), nil
	}

	// ── Elements-only moving body (no provider supplied) ──
	//
	// A small body resolved from SBDB carries real published osculating
	// elements (c.HasElements). With no caller-supplied provider,
	// propagating those via two-body Keplerian motion
	// (eph.NewFromElements) is strictly better than the fixed-target
	// fall-through below, which would produce a coordinate-less
	// *DeepSkyObject for a body that has no fixed coordinate at all — a
	// caller who wants real, perturbed, kernel-backed positions supplies
	// a provider, and that path above is untouched and still takes
	// precedence.
	//
	// Failure here (invalid elements, or no H/M1 photometry to pick a body
	// type) falls straight through to the fixed-target path below, exactly
	// like the NewPlanetaryMoon branch above does, since FromCatalog has no
	// error return.
	if c.HasElements && needsSmallBodyEphemeris(c.Kind) {
		if el, err := elementsFromTarget(c); err == nil {
			keplerID := id
			if !idOK {
				keplerID = keplerSyntheticID
			}

			if provider, err := eph.NewFromElements(keplerID, el); err == nil {
				switch {
				case c.HasM1:
					return newCometFromTarget(c, keplerID, provider), nil
				case c.HasH:
					return NewAsteroid(c.Name, keplerID, provider, asteroidOptsFrom(c)...), nil
				}
			}
		}
	}

	// ── Fixed targets ──

	// ── Fixed targets need a real position ──
	//
	// Everything above this line gets its position from an ephemeris, so a
	// missing catalog coordinate is irrelevant to it. Everything below is
	// pinned to c.Coord and cannot be built without one.
	if !c.HasCoord {
		return nil, fmt.Errorf("%w: %s", ErrNoCoordinates, c.Name)
	}

	// Star
	if c.Kind == resolve.KindStar || c.Kind == resolve.KindDoubleStar {
		pos, err := starAtJ2000(c)
		if err != nil {
			return nil, fmt.Errorf("plan: %s: %w", c.Name, err)
		}

		var opts []StarOption
		if pos.PmRA().Radians() != 0 || pos.PmDec().Radians() != 0 {
			opts = append(opts, WithProperMotion(pos.PmRA(), pos.PmDec()))
		}

		// Gated on the catalog's own parallax: for a moving star without
		// one, SOFA's propagation substitutes a token minimum, which is not
		// a measurement.
		if c.Parallax.Radians() != 0 {
			opts = append(opts, WithParallax(pos.Parallax()))
		}

		if c.HasRadialVelocity {
			opts = append(opts, WithRadialVelocity(pos.RV()))
		}

		if c.HasVMag {
			opts = append(opts, WithStarMagnitude(c.VMag))
		}

		if len(c.Aliases) > 0 {
			opts = append(opts, WithAliases(c.Aliases...))
		}

		return NewStar(c.Name, pos.RA(), pos.Dec(), opts...), nil
	}

	// Deep-sky object (galaxy, nebula, cluster, etc.)

	var opts []DSOOption
	if c.HasVMag {
		opts = append(opts, WithDSOMagnitude(c.VMag))
	}

	if c.HasRadialVelocity {
		opts = append(opts, WithDSORadialVelocity(c.RadialVelocity))
	}

	if string(c.Kind) != "" {
		opts = append(opts, WithDSOKind(string(c.Kind)))
	}

	if len(c.Aliases) > 0 {
		opts = append(opts, WithDSOAliases(c.Aliases...))
	}

	return NewDeepSkyObject(c.Name, c.Coord.RA(), c.Coord.Dec(), opts...), nil
}

// elementsFromTarget builds c's orbit in whichever form describes it. A
// closed orbit published with a semi-major axis goes through eph.NewElements,
// as it always has. Anything else — an orbit with e >= 1, which has no
// usable semi-major axis, or one published only in perihelion form, as the
// MPC's comet file publishes every comet — goes through
// eph.ElementsFromPerihelion.
func elementsFromTarget(c catalog.Target) (eph.Elements, error) {
	var (
		el  eph.Elements
		err error
	)

	if c.Eccentricity < 1 && c.SemiMajorAxis > 0 {
		el, err = eph.NewElements(c.Epoch, c.SemiMajorAxis, c.Eccentricity,
			c.Inclination, c.AscendingNode, c.ArgPeriapsis, c.MeanAnomaly)
	} else {
		// The osculation epoch, where the source published one: the MPC's
		// comet file carries it, falling back itself to the perihelion time
		// for an unperturbed orbit, and that is the fallback here too.
		epoch := c.Epoch
		if epoch.IsZero() {
			epoch = c.PerihelionTime
		}

		el, err = eph.ElementsFromPerihelion(epoch, c.PerihelionTime, c.PerihelionDistance, c.Eccentricity,
			c.Inclination, c.AscendingNode, c.ArgPeriapsis)
	}

	if err != nil {
		return eph.Elements{}, fmt.Errorf("plan: elements of %q: %w", c.Name, err)
	}

	return el, nil
}

// newCometFromTarget builds a *Comet from c's M1/K1 (and optional M2/K2/PC)
// photometry, shared identically by both the kernel-backed and
// Kepler-backed FromCatalog paths so they can't drift apart.
func newCometFromTarget(c catalog.Target, id eph.ID, p eph.Provider) *Comet {
	comet := NewComet(c.Name, id, p, c.M1, c.K1)
	if c.M2 != 0 {
		comet.M2 = c.M2
		comet.K2 = c.K2
		comet.PC = c.PC
	}

	return comet
}

// asteroidOptsFrom builds the AsteroidOptions for c's H/G-family
// photometry, shared identically by both the kernel-backed and
// Kepler-backed FromCatalog paths so they can't drift apart.
func asteroidOptsFrom(c catalog.Target) []AsteroidOption {
	var opts []AsteroidOption

	if c.HasG1G2 {
		opts = append(opts, WithHG1G2(c.H, c.G1, c.G2))
		if c.HasSpin && c.HasOblateness {
			opts = append(opts, WithSpin(c.SpinRA, c.SpinDec, c.Oblateness))
		}
	} else {
		g := c.G
		if g == 0 {
			g = 0.15
		}

		opts = append(opts, WithHG(c.H, g))
	}

	if c.HasDiameter {
		opts = append(opts, WithDiameter(c.Diameter))
	}

	if c.HasAlbedo {
		opts = append(opts, WithAlbedo(c.Albedo))
	}

	return opts
}

// naifMajorRangeID returns spkID as a NAIF ID when it is one of the Sun's,
// a planet's, a system barycenter's or a planetary satellite's: 1 to 999.
// Small bodies' SPK-IDs start at 1,000,000 and spacecraft's are negative.
func naifMajorRangeID(spkID string) (int, bool) {
	n, err := strconv.Atoi(spkID)
	if err != nil || n < 1 || n > 999 {
		return 0, false
	}

	return n, true
}

// majorBodyForNAIF returns the point plan observes for the major body a
// NAIF code names: the code itself for the Sun, the Moon, the Earth, Mercury
// and Venus, and from Mars out the system barycenter for either the
// barycenter or the planet's center, since a planetary kernel holds only the
// barycenter, a few hundredths of an arcsecond from the planet (see eph.ID).
// The Earth-Moon barycenter (3) names no body to observe.
func majorBodyForNAIF(naif int) (eph.ID, bool) {
	switch naif {
	case 10:
		return eph.Sun, true
	case 301:
		return eph.Moon, true
	case 1, 199:
		return eph.Mercury, true
	case 2, 299:
		return eph.Venus, true
	case 399:
		return eph.Earth, true
	case 4, 499:
		return eph.MarsBarycenter, true
	case 5, 599:
		return eph.JupiterBarycenter, true
	case 6, 699:
		return eph.SaturnBarycenter, true
	case 7, 799:
		return eph.UranusBarycenter, true
	case 8, 899:
		return eph.NeptuneBarycenter, true
	case 9, 999:
		return eph.PlutoBarycenter, true
	default:
		return 0, false
	}
}

// ephIDOf is the NAIF code a catalog target names, and whether it names one.
// An SPK-ID is a NAIF code (JPL, SBDB). A small body a catalog gives by its
// number instead — FINK's asteroids, MPCORB's packed "00433" — is NAIF's
// 20000000 + n: every small-body code NAIF issues is above 1,000,000, so a
// positive number below that can only be a catalog number. Read as a NAIF
// code, 433 named no body and 301 would have been the Moon; a lookup used to
// retry a missing code as a small body's, and no longer does (#809).
func ephIDOf(c catalog.Target) (eph.ID, bool) {
	if c.SPKID != "" {
		return parseEphID(c.SPKID)
	}

	switch c.Kind { //nolint:exhaustive // only small bodies are numbered by their catalogs
	case resolve.KindAsteroid, resolve.KindDwarfPlanet, resolve.KindComet:
		if n, err := strconv.Atoi(c.ID); err == nil && n > 0 && n < 1000000 {
			return eph.SmallBodyID(n)
		}
	}

	return parseEphID(c.ID)
}

// parseEphID reads a NAIF code, and whether id is one. Zero is the Solar
// System barycenter, so a failure is the bool, not a value of the ID.
func parseEphID(id string) (eph.ID, bool) {
	n, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return 0, false
	}

	return eph.ID(n), true
}
