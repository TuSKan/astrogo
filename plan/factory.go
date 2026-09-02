package plan

import (
	"fmt"

	"strconv"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
	eph "github.com/TuSKan/astrogo/ephemeris"
)

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
//     preferred over eph.Default() here, which has no data for an arbitrary small body
//   - Star kind → *Star
//   - Everything else → *DeepSkyObject
func FromCatalog(c catalog.Target, p eph.Provider) (Observable, error) {
	id := parseEphID(c.ID)

	// ── Satellite ──
	if c.Kind == "Satellite" && p != nil {
		return NewSatellite(c.Name, id, p), nil
	}

	// ── Sun/Moon/planets — always have a real ephemeris, provider or not ──
	//
	// isPlanetID(id) means this target really is a major named body (the
	// resolver tags the Sun with KindStar, not KindPlanet, hence checking
	// all three Kinds here). NewPlanet itself defaults a nil provider to
	// eph.Default() — which answers every one of these bodies (Sun, Moon,
	// Mercury-Neptune, Pluto, the Solar System Barycenter) — so a caller
	// who didn't supply a provider still gets a real, moving *Planet
	// instead of silently degrading to a static, non-moving fixed-target
	// Observable built from whatever (or no) fixed coordinate happened to
	// be on c.Coord. A caller-supplied provider (a real kernel, for
	// perturbation-aware accuracy) always takes precedence, unchanged.
	if (c.Kind == resolve.KindPlanet || c.Kind == resolve.KindMoon || c.Kind == resolve.KindStar) && isPlanetID(id) {
		return NewPlanet(c.Name, id, p), nil
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
	// Failure here (invalid/hyperbolic elements, rejected by
	// eph.NewElements with ErrUnsupportedOrbit, or no H/M1 photometry to
	// pick a body type) falls straight through to the fixed-target path
	// below, exactly like the NewPlanetaryMoon branch above does, since
	// FromCatalog has no error return.
	if c.HasElements && needsSmallBodyEphemeris(c.Kind) {
		if el, err := eph.NewElements(c.Epoch, c.SemiMajorAxis, c.Eccentricity,
			c.Inclination, c.AscendingNode, c.ArgPeriapsis, c.MeanAnomaly); err == nil {
			keplerID := id
			if keplerID == 0 {
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
		var opts []StarOption
		if c.PmRA.Radians() != 0 || c.PmDec.Radians() != 0 {
			opts = append(opts, WithProperMotion(c.PmRA, c.PmDec))
		}

		if c.Parallax.Radians() != 0 {
			opts = append(opts, WithParallax(c.Parallax))
		}

		if c.HasRadialVelocity {
			opts = append(opts, WithRadialVelocity(c.RadialVelocity))
		}

		if c.HasVMag {
			opts = append(opts, WithStarMagnitude(c.VMag))
		}

		if len(c.Aliases) > 0 {
			opts = append(opts, WithAliases(c.Aliases...))
		}

		return NewStar(c.Name, c.Coord.RA(), c.Coord.Dec(), opts...), nil
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

// newCometFromTarget builds a *Comet from c's M1/K1 (and optional M2/K2)
// photometry, shared identically by both the kernel-backed and
// Kepler-backed FromCatalog paths so they can't drift apart.
func newCometFromTarget(c catalog.Target, id eph.ID, p eph.Provider) *Comet {
	comet := NewComet(c.Name, id, p, c.M1, c.K1)
	if c.M2 != 0 {
		comet.M2 = c.M2
		comet.K2 = c.K2
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

// parseEphID converts a string ID to an eph.ID, returning 0 on failure.
func parseEphID(id string) eph.ID {
	n, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return 0
	}

	return eph.ID(n)
}

// isPlanetID returns true for NAIF IDs that correspond to Sun/Moon/planets.
func isPlanetID(id eph.ID) bool {
	return id >= 1 && id <= 11
}
