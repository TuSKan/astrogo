package magnitude

import (
	"errors"
	"math"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// ErrUnsupportedBody is returned when magnitude cannot be computed for a body.
var ErrUnsupportedBody = errors.New("magnitude: unsupported body")

// PhaseAngle computes the Sun–Target–Observer (phase) angle for a Solar System
// body at the given time. The observer is at the geocenter.
//
// Uses the law of cosines on the Sun–Target–Earth triangle:
//
//	cos(α) = (r² + Δ² − R²) / (2·r·Δ)
//
// where r = heliocentric distance, Δ = geocentric distance, R = Earth-Sun distance.
func PhaseAngle(p eph.Provider, target eph.ID, t time.Time) (angle.Angle, error) {
	geo, sun, err := geometryVectors(p, target, t)
	if err != nil {
		return 0, err
	}

	delta := geo.delta                                            // geocentric distance
	r := geo.r                                                    // heliocentric distance
	R := math.Sqrt(sun[0]*sun[0] + sun[1]*sun[1] + sun[2]*sun[2]) // Earth-Sun distance

	cosAlpha := (r*r + delta*delta - R*R) / (2 * r * delta)
	cosAlpha = math.Max(-1, math.Min(1, cosAlpha)) // clamp for numerical safety

	return angle.Rad(math.Acos(cosAlpha)), nil
}

// IlluminatedFraction returns the fraction of the target's disk that is
// illuminated as seen from the geocenter (0 = new, 1 = full).
func IlluminatedFraction(p eph.Provider, target eph.ID, t time.Time) (float64, error) {
	alpha, err := PhaseAngle(p, target, t)
	if err != nil {
		return 0, err
	}

	return (1 + math.Cos(alpha.Radians())) / 2, nil
}

// ── Internal geometry ────────────────────────────────────────────────────────

// bodyGeometry holds the distances needed for magnitude computations.
type bodyGeometry struct {
	delta float64 // geocentric distance (AU)
	r     float64 // heliocentric distance (AU)
}

// geometryVectors computes the geocentric and heliocentric distances for a body.
// Returns (bodyGeom, sunGeocentricPos, error).
func geometryVectors(p eph.Provider, target eph.ID, t time.Time) (bodyGeometry, [3]float64, error) {
	planetSt, err := p.State(target, t)
	if err != nil {
		return bodyGeometry{}, [3]float64{}, err
	}

	sunSt, err := p.State(eph.Sun, t)
	if err != nil {
		return bodyGeometry{}, [3]float64{}, err
	}

	// Earth→Planet (geocentric)
	delta := planetSt.Distance()

	// Sun→Planet = Earth→Planet − Earth→Sun (heliocentric)
	helioX := planetSt.Pos.X - sunSt.Pos.X
	helioY := planetSt.Pos.Y - sunSt.Pos.Y
	helioZ := planetSt.Pos.Z - sunSt.Pos.Z
	r := math.Sqrt(helioX*helioX + helioY*helioY + helioZ*helioZ)

	sunPos := [3]float64{sunSt.Pos.X, sunSt.Pos.Y, sunSt.Pos.Z}

	return bodyGeometry{delta: delta, r: r}, sunPos, nil
}
