package plan

import (
	"strconv"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
	eph "github.com/TuSKan/astrogo/ephemeris"
)

// FromCatalog converts a catalog.Target (wire format from resolvers) and an
// optional ephemeris provider into the appropriate concrete Observable type.
//
// Routing logic:
//   - Satellite TLE → *Satellite
//   - Planet/Moon/Star with provider → *Planet
//   - HasM1 (comet photometry) → *Comet
//   - HasH (asteroid photometry) → *Asteroid
//   - Star kind → *Star
//   - Everything else → *DeepSkyObject
func FromCatalog(c catalog.Target, p eph.Provider) Observable {
	id := parseEphID(c.ID)

	// ── Satellite ──
	if c.Kind == "Satellite" && p != nil {
		return NewSatellite(c.Name, id, p)
	}

	// ── Moving body with provider ──
	if p != nil {
		switch c.Kind {
		case resolve.KindPlanet, resolve.KindMoon, resolve.KindStar:
			// Sun/Moon/planets — the resolver uses KindStar for Sun
			if isPlanetID(id) {
				return NewPlanet(c.Name, id, p)
			}
		}

		// Comet
		if c.HasM1 {
			comet := NewComet(c.Name, id, p, c.M1, c.K1)
			if c.M2 != 0 {
				comet.M2 = c.M2
				comet.K2 = c.K2
			}

			return comet
		}

		// Asteroid
		if c.HasH {
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

			return NewAsteroid(c.Name, id, p, opts...)
		}

		// Generic moving body (unknown sub-type) → Planet as fallback
		return NewPlanet(c.Name, id, p)
	}

	// ── Fixed targets ──

	// Star
	if c.Kind == resolve.KindStar || c.Kind == resolve.KindDoubleStar {
		var opts []StarOption
		if c.PmRA.Radians() != 0 || c.PmDec.Radians() != 0 {
			opts = append(opts, WithProperMotion(c.PmRA, c.PmDec))
		}

		if c.Parallax.Radians() != 0 {
			opts = append(opts, WithParallax(c.Parallax))
		}

		if c.RadialVelocity != 0 {
			opts = append(opts, WithRadialVelocity(c.RadialVelocity))
		}

		if c.HasVMag {
			opts = append(opts, WithStarMagnitude(c.VMag))
		}

		if len(c.Aliases) > 0 {
			opts = append(opts, WithAliases(c.Aliases...))
		}

		ra := angle.Rad(0)
		dec := angle.Rad(0)

		if c.HasCoord {
			ra = c.Coord.RA()
			dec = c.Coord.Dec()
		}

		return NewStar(c.Name, ra, dec, opts...)
	}

	// Deep-sky object (galaxy, nebula, cluster, etc.)
	ra := angle.Rad(0)
	dec := angle.Rad(0)

	if c.HasCoord {
		ra = c.Coord.RA()
		dec = c.Coord.Dec()
	}

	var opts []DSOOption
	if c.HasVMag {
		opts = append(opts, WithDSOMagnitude(c.VMag))
	}

	if string(c.Kind) != "" {
		opts = append(opts, WithDSOKind(string(c.Kind)))
	}

	if len(c.Aliases) > 0 {
		opts = append(opts, WithDSOAliases(c.Aliases...))
	}

	return NewDeepSkyObject(c.Name, ra, dec, opts...)
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
