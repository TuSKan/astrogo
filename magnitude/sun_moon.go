package magnitude

import (
	"math"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// SunApparent returns the apparent V-band magnitude of the Sun.
// Varies slightly with Earth's orbital eccentricity (±0.03 mag over the year).
//
//	V ≈ −26.74 + 5·log₁₀(R)
//
// where R = Earth–Sun distance in AU.
func SunApparent(p eph.Provider, t time.Time) (float64, error) {
	st, err := p.State(eph.Sun, t)
	if err != nil {
		return 0, err
	}
	R := st.Distance()
	if R < 1e-12 {
		return -26.74, nil
	}
	return -26.74 + 5*math.Log10(R), nil
}

// MoonApparent returns the approximate V-band apparent magnitude of the Moon.
//
// Uses the Allen (2000) formula:
//
//	V ≈ −12.74 + 0.026·|α| + 4e-9·α⁴
//
// where α is the phase angle in degrees. At new moon (α≈180°) this gives
// approximately +2.0; at full moon (α≈0°) it gives −12.74.
func MoonApparent(p eph.Provider, t time.Time) (float64, error) {
	geo, sunPos, err := geometryVectors(p, eph.Moon, t)
	if err != nil {
		return 0, err
	}

	delta := geo.delta
	r := geo.r
	R := math.Sqrt(sunPos[0]*sunPos[0] + sunPos[1]*sunPos[1] + sunPos[2]*sunPos[2])

	cosAlpha := (r*r + delta*delta - R*R) / (2 * r * delta)
	cosAlpha = math.Max(-1, math.Min(1, cosAlpha))
	alphaDeg := math.Acos(cosAlpha) * 180 / math.Pi

	// Allen (2000) polynomial.
	v := -12.74 + 0.026*math.Abs(alphaDeg) + 4e-9*math.Pow(alphaDeg, 4)
	return v, nil
}
