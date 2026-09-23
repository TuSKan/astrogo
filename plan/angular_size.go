package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// bodyEquatorialRadius maps each Solar System body this package can
// compute a real ephemeris position for to its published equatorial
// radius — see constants/iau2015.go for the values and their per-body
// IAU sourcing. The eph.ID-keyed map itself lives here rather
// than in constants: constants sits below ephemeris in this codebase's
// layering (see CLAUDE.md's Architecture section) and imports nothing
// but unit, so it can only publish the radii as bare constants.Constant
// values, not a table keyed by ephemeris/core's eph.ID.
//
// Earth's entry is the WGS 84 semi-major axis, not an IAU2015 member —
// it is exact to the WGS 84 standard. IAU 2015 B3's nominal equatorial
// radius is 37 m shorter.
var bodyEquatorialRadius = map[eph.ID]unit.Length{
	eph.Sun:     unit.Meters(constants.IAU.SunEquatorialRadius.Value),
	eph.Moon:    unit.Meters(constants.IAU.MoonEquatorialRadius.Value),
	eph.Mercury: unit.Meters(constants.IAU.MercuryEquatorialRadius.Value),
	eph.Venus:   unit.Meters(constants.IAU.VenusEquatorialRadius.Value),
	eph.Earth:   unit.Meters(constants.WGS84.SemiMajorAxis.Value),
	eph.Mars:    unit.Meters(constants.IAU.MarsEquatorialRadius.Value),
	eph.Jupiter: unit.Meters(constants.IAU.JupiterEquatorialRadius.Value),
	eph.Saturn:  unit.Meters(constants.IAU.SaturnEquatorialRadius.Value),
	eph.Uranus:  unit.Meters(constants.IAU.UranusEquatorialRadius.Value),
	eph.Neptune: unit.Meters(constants.IAU.NeptuneEquatorialRadius.Value),
	eph.Pluto:   unit.Meters(constants.IAU.PlutoEquatorialRadius.Value),
}

// BodyEquatorialRadius returns id's published equatorial radius, and whether
// one is known. Only the Sun, Moon, and the eight planets (plus Pluto) have
// one — asteroids, comets, and satellites report ok=false, since this library
// has no general-purpose physical-size catalog for them (see
// AngularDiameter's doc comment for that case).
func BodyEquatorialRadius(id eph.ID) (unit.Length, bool) {
	r, ok := bodyEquatorialRadius[id]

	return r, ok
}

// AngularDiameter computes mb's apparent angular diameter at t — the full
// disc width, not a radius — as seen topocentrically from ctx's observer.
//
// It uses the exact relation 2*asin(R/Δ), not the small-angle
// approximation 2R/Δ; the difference is negligible for every body here
// except the Moon near perigee, where it still costs nothing to get right.
//
// Δ is the TOPOCENTRIC distance (observer to body), matching both what a
// visual observer actually sees and what JPL Horizons reports as its
// Ang-diam quantity — not the geocentric distance, which differs by up to
// ~1.7% for the Moon depending on the observer's position on Earth's disc.
//
// Returns ErrNoPhysicalRadius if mb's EphID has no known radius AND mb
// doesn't implement the PhysicalRadius optional capability — a comet or
// satellite, for instance, since this library has no general-purpose
// physical-size catalog for either.
func AngularDiameter(mb MovingBody, t time.Time, ctx *coord.Context) (angle.Angle, error) {
	radius, ok := BodyEquatorialRadius(mb.EphID())
	if !ok {
		if pr, isPR := mb.(PhysicalRadius); isPR {
			radius, ok = pr.PhysicalRadius()
		}

		if !ok {
			return angle.Zero(), fmt.Errorf("plan: angular diameter of %v: %w", mb.EphID(), ErrNoPhysicalRadius)
		}
	}

	vec, err := mb.GeocentricVec(t)
	if err != nil {
		return angle.Zero(), fmt.Errorf("plan: angular diameter: %w", err)
	}

	topoDist := unit.AU(vec.Sub(ctx.ObsVec()).Norm())
	if topoDist <= 0 {
		return angle.Zero(), fmt.Errorf("plan: angular diameter: %w", ErrZeroDistance)
	}

	radiusOverDist := radius.Meters() / topoDist.Meters()
	if radiusOverDist > 1 {
		// Only reachable for a synthetic/broken distance placing the
		// observer inside the body — clamp rather than let asin produce
		// NaN (see plan/solver.go's item-6 fix for why silent NaN
		// propagation is worth guarding against defensively).
		radiusOverDist = 1
	}

	return angle.Asin(radiusOverDist).MulScalar(2), nil
}

// AngularDiameter is a convenience wrapper around the package-level
// AngularDiameter for *Planet, mirroring ApparentMagnitude/
// ApparentMagnitudeCtx's existing pattern on this type.
func (p *Planet) AngularDiameter(t time.Time, ctx *coord.Context) (angle.Angle, error) {
	return AngularDiameter(p, t, ctx)
}

// formatAngularSize renders a as a human-readable angular size string:
// arcminutes (e.g. "31.09'") at or above 1 arcminute, arcseconds (e.g.
// "15.42\"") below that — matching the precision an observer planning a
// session actually cares about at each scale.
func formatAngularSize(a angle.Angle) string {
	if arcmin := a.Arcminutes(); arcmin >= 1 {
		return fmt.Sprintf("%.2f′", arcmin)
	}

	return fmt.Sprintf("%.2f″", a.Arcseconds())
}
