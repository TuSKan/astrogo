package ephemeris

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/vector"
)

// carryTolerance is the most a carried body may move a place: 1 µas, in
// radians.
const carryTolerance = 1e-6 / 3600 * math.Pi / 180

// carrier is B, the body the light-time solve carries back over τ instead of
// evaluating at t − τ (#791): the first body on the target's chain of centers
// that is also on the Earth's. Its barycentric state and the acceleration its
// model keeps are taken at t, from the evaluations the observer has already
// made there.
//
// # Why
//
// On ephemeris.Default an evaluation at a new epoch is SOFA's Epv00, 34 µs,
// and the exact solve makes two per place: one at t for the observer and one
// at t − τ for the target. The second exists only to learn where B was τ
// earlier: the Earth, for the Moon; the Sun, for the planets. Carried, a place
// costs one, and the target below B is still evaluated exactly at t − τ.
//
// # When
//
// Only B = Earth and B = Sun are carried, each with a model and a bound on
// what the model leaves out (carryResidual). The error of a carry over τ is
// that residual acceleration × τ²/2, which seen from a light time τ away is an
// angle of residual × τ/(2c): within carryTolerance while τ ≤ maxTau. Past
// that, the solve evaluates B exactly. On a JPL kernel B is the Earth-Moon or
// the Solar System barycenter, whose evaluations are Chebyshev sums or
// nothing, and nothing is carried.
type carrier struct {
	id       ID
	pos, vel vector.Vec3 // barycentric, at t
	acc      vector.Vec3 // the modeled acceleration, at t, in AU/day²
	maxTau   float64     // days
}

// The masses a carried body's model leaves out, each at the nearest it comes
// to that body. Distances are from E. M. Standish's J2000 elements
// ("Keplerian Elements for Approximate Positions of the Major Planets", Table
// 1), as perihelion a(1 − e) and aphelion a(1 + e), in AU.
type perturber struct {
	gm      float64 // m³/s²
	nearest float64 // AU
}

// sunResidual is the Sun's whole acceleration about the Solar System
// barycenter, since the Sun is carried linearly: every planet system at its
// perihelion. Summed, 3.11e-7 m/s², 74% of it Jupiter's; a Sun carried over
// τ ≤ 2.60 h stays within carryTolerance, which holds the planets out to
// Saturn, Uranus while its light time is under that (it runs from 2.5 to
// 2.8 h), and leaves Neptune to the exact solve.
var sunResidual = []perturber{
	{constants.Ephemeris.MercurySystemGravitationalParameter.Value, 0.30750},
	{constants.Ephemeris.VenusSystemGravitationalParameter.Value, 0.71843},
	{constants.Ephemeris.EarthMoonGravitationalParameter.Value, 0.98329},
	{constants.Ephemeris.MarsSystemGravitationalParameter.Value, 1.38140},
	{constants.Ephemeris.JupiterSystemGravitationalParameter.Value, 4.95114},
	{constants.Ephemeris.SaturnSystemGravitationalParameter.Value, 9.02302},
	{constants.Ephemeris.UranusSystemGravitationalParameter.Value, 18.28234},
	{constants.Ephemeris.NeptuneSystemGravitationalParameter.Value, 29.81161},
}

// earthResidual is what the Earth's model, the Sun's pull at t, leaves out:
// the Moon, at a distance it never comes within (350,000 km, below every
// perigee), and each planet at its nearest to the Earth, the gap between the
// Earth's orbit and its own. Summed, 4.07e-5 m/s², nearly all the Moon's; an
// Earth carried over τ ≤ 71.5 s stays within carryTolerance, where the Moon's
// light time is 1.3 s.
var earthResidual = []perturber{
	{constants.Ephemeris.MoonGravitationalParameter.Value, 350_000e3 / constants.IAU.AstronomicalUnit.Value},
	{constants.Ephemeris.MercurySystemGravitationalParameter.Value, 0.98329 - 0.46670},
	{constants.Ephemeris.VenusSystemGravitationalParameter.Value, 0.98329 - 0.72824},
	{constants.Ephemeris.MarsSystemGravitationalParameter.Value, 1.38140 - 1.01671},
	{constants.Ephemeris.JupiterSystemGravitationalParameter.Value, 4.95114 - 1.01671},
	{constants.Ephemeris.SaturnSystemGravitationalParameter.Value, 9.02302 - 1.01671},
	{constants.Ephemeris.UranusSystemGravitationalParameter.Value, 18.28234 - 1.01671},
	{constants.Ephemeris.NeptuneSystemGravitationalParameter.Value, 29.81161 - 1.01671},
}

// carryResidual is the most a model leaves out, in m/s²: each mass's pull at
// its nearest.
func carryResidual(ps []perturber) float64 {
	au := constants.IAU.AstronomicalUnit.Value

	var a float64

	for _, p := range ps {
		d := p.nearest * au
		a += p.gm / (d * d)
	}

	return a
}

// carryMaxTau is the longest light time, in days, over which a residual
// acceleration in m/s² keeps a carry within carryTolerance:
// residual·τ/(2c) ≤ tolerance.
func carryMaxTau(residual float64) float64 {
	return 2 * constants.SI2019.SpeedOfLight.Value * carryTolerance / residual /
		constants.Derived.JulianDaySeconds.Value
}

var (
	sunMaxTau   = carryMaxTau(carryResidual(sunResidual))
	earthMaxTau = carryMaxTau(carryResidual(earthResidual))

	// carriedMaxTau are the bodies a solve may carry, with the longest light
	// time each is carried for.
	carriedMaxTau = map[ID]float64{Earth: earthMaxTau, Sun: sunMaxTau}
)

// gmSunAU is the Sun's mass parameter in AU³/day², for the Earth's model.
var gmSunAU = func() float64 {
	au, day := constants.IAU.AstronomicalUnit.Value, constants.Derived.JulianDaySeconds.Value

	return constants.Ephemeris.SunGravitationalParameter.Value * day * day / (au * au * au)
}()

// carrierFor is the carrier for target, or false when nothing is carried: B is
// neither the Earth nor the Sun. It reads only what the Instant has located
// at t already, the target's chain and the observer's.
func (in *Instant) carrierFor(target ID) (carrier, bool, error) {
	b, err := in.meet(target, Earth)
	if err != nil {
		return carrier{}, false, err
	}

	maxTau, carried := carriedMaxTau[b]
	if !carried {
		return carrier{}, false, nil
	}

	st, err := in.Geometric(b, SolarSystemBarycenter)
	if err != nil {
		return carrier{}, false, fmt.Errorf("ephemeris: %v to carry: %w", b, err)
	}

	c := carrier{id: b, pos: st.Pos, vel: st.Vel, maxTau: maxTau}

	if b == Earth {
		sun, err := in.Geometric(Sun, SolarSystemBarycenter)
		if err != nil {
			return carrier{}, false, fmt.Errorf("ephemeris: the Sun's pull on the Earth: %w", err)
		}

		r := st.Pos.Sub(sun.Pos)
		rn := r.Norm()
		c.acc = r.MulScalar(-gmSunAU / (rn * rn * rn))
	}

	return c, true, nil
}

// at is B's barycentric state carried back to τ days before t.
func (c carrier) at(tau float64) (pos, vel vector.Vec3) {
	return c.pos.Sub(c.vel.MulScalar(tau)).Add(c.acc.MulScalar(tau * tau / 2)), c.vel.Sub(c.acc.MulScalar(tau))
}

// meet is the first body on target's chain of centers that is also on
// other's, the Solar System barycenter if nothing nearer. The chains are the
// provider's, read at the Instant's epoch through locate.
func (in *Instant) meet(target, other ID) (ID, error) {
	var (
		ids [chainDepth + 1]ID
		n   int
	)

	for id := other; ; {
		ids[n] = id
		n++

		if id == SolarSystemBarycenter || n > chainDepth {
			break
		}

		st, err := in.locate(id)
		if err != nil {
			return 0, err
		}

		id = st.Center
	}

	for id, depth := target, 0; depth <= chainDepth; depth++ {
		if indexOf(ids[:n], id) >= 0 {
			return id, nil
		}

		if id == SolarSystemBarycenter {
			break
		}

		st, err := in.locate(id)
		if err != nil {
			return 0, err
		}

		id = st.Center
	}

	return SolarSystemBarycenter, nil
}
