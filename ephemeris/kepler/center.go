package kepler

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// perturber is a planet system whose pull the center choice weighs: its
// barycenter and its mass parameter, in m³/s².
type perturber struct {
	id core.ID
	gm float64
}

// planetSystems are the masses beside the Sun the center choice weighs: the
// eight planet systems, the Earth's as the Earth-Moon pair. Pluto's system is
// 7·10⁻⁹ of the Sun and decides nothing.
var planetSystems = [...]perturber{
	{core.MercuryBarycenter, constants.Ephemeris.MercurySystemGravitationalParameter.Value},
	{core.VenusBarycenter, constants.Ephemeris.VenusSystemGravitationalParameter.Value},
	{core.EarthMoonBarycenter, constants.Ephemeris.EarthMoonGravitationalParameter.Value},
	{core.MarsBarycenter, constants.Ephemeris.MarsSystemGravitationalParameter.Value},
	{core.JupiterBarycenter, constants.Ephemeris.JupiterSystemGravitationalParameter.Value},
	{core.SaturnBarycenter, constants.Ephemeris.SaturnSystemGravitationalParameter.Value},
	{core.UranusBarycenter, constants.Ephemeris.UranusSystemGravitationalParameter.Value},
	{core.NeptuneBarycenter, constants.Ephemeris.NeptuneSystemGravitationalParameter.Value},
}

// centerChoice is what the center choice weighed for one orbit, at its
// osculation epoch: the acceleration each fit leaves out, the barycentric
// fit's central mass, and the orbit's heliocentric state.
type centerChoice struct {
	helio, bary float64 // the perturbing accelerations, m³/s² per AU²
	interiorGM  float64 // the Sun's and every interior planet's, m³/s²
	r, v        vector.Vec3
}

// fitCenter returns el as Register stores it on a provider over base (#803):
// about the Sun as given, or refit about the Solar System barycenter when that
// leaves the smaller perturbing acceleration at the osculation epoch.
//
// # The rule
//
// A two-body fit is exact while nothing but its central mass acts, so the
// better center is the one about which the rest of the system pulls least
// where the orbit osculates:
//
//   - About the Sun, the fit leaves out each planet's direct pull, less the
//     same planet's pull on the Sun, since a heliocentric frame accelerates
//     with the Sun.
//   - About the barycenter, with the Sun and every planet inside the body's
//     distance as its central mass, it leaves out everything but that central
//     term.
//
// Measured on 26 real small bodies against Horizons' integrations, the rule
// picks the better center for every one, 30 days and a year either side of the
// epoch: the Sun for the near-Earth asteroids, the main belt, the Hildas,
// Achilles, Encke, 67P and C/2023 A3; the barycenter for 29P, the Centaurs,
// C/2014 UN271, the trans-Neptunian objects, Sedna, and Halley and Hale-Bopp
// far out. The fit it keeps is the better by a factor of 1.7 to 28,000 at 30
// days, except for Hektor, a Trojan with Jupiter just inside its distance,
// where the barycenter wins by 16%. Nothing is tuned: the inputs are the
// masses and where they are.
//
// # What it reads
//
// The choice reads the Sun and the planets from the offline SOFA series the
// default base uses, whatever base the Provider has: it compares accelerations
// that differ by factors like those above, which those series place far better
// than they need to, and a base that answers only some bodies — a test's, or a
// kernel short of the outer planets — then changes nothing about it.
//
// The refit reads the Sun from base. [Provider.Locate] answers the orbit about
// the barycenter, and ephemeris.At meets it there with the base's Earth, so its
// state has to be placed about the base's barycenter, not SOFA's. SOFA's Sun
// sits a constant 116 km from Horizons', measured at four epochs from 2000 to
// 2026, and a refit through it would carry that into every position over a JPL
// kernel, where every trans-Neptunian object measured fits about the
// barycenter to under a kilometer at 30 days.
//
// Only an osculating set referred to the Sun is considered. A Laplace plane, a
// secular precession and a period describe a satellite's mean elements, and
// [PlutoElements] are fitted to Pluto's path over two and a half centuries:
// the state of either at its epoch is not the body's, so a refit through it
// would be a different orbit, which for Pluto missed by 1.96 AU (#877).
func fitCenter(el Elements, base core.Provider) (Elements, error) {
	if el.CentralBody().ID != core.Sun || el.plane != nil || el.precession.rate() != 0 || el.periodDays != 0 || el.fitted {
		return el, nil
	}

	choice, err := chooseCenter(el)
	if err != nil {
		return Elements{}, err
	}

	if !(choice.bary < choice.helio) {
		return el, nil
	}

	sun, sunVel, err := barycentric(base, core.Sun, el.Epoch())
	if err != nil {
		return Elements{}, fmt.Errorf("kepler: refitting about the barycenter: %w", err)
	}

	refit, err := FromState(el.Epoch(), choice.r.Add(sun), choice.v.Add(sunVel),
		CentralBody{ID: core.SolarSystemBarycenter, GM: choice.interiorGM})
	if err != nil {
		return Elements{}, fmt.Errorf("kepler: refitting about the barycenter: %w", err)
	}

	return refit, nil
}

// chooseCenter weighs both fits for el at its osculation epoch. See fitCenter.
func chooseCenter(el Elements) (centerChoice, error) {
	at := el.Epoch()

	r, v, err := el.StateAt(at)
	if err != nil {
		return centerChoice{}, err
	}

	sofa := &sofaBase{}

	sun, _, err := barycentric(sofa, core.Sun, at)
	if err != nil {
		return centerChoice{}, err
	}

	sunGM := el.CentralBody().GM
	body := r.Add(sun)
	distance := r.Norm()

	// pull is a mass's acceleration on a point, toward it.
	pull := func(gm float64, from, to vector.Vec3) vector.Vec3 {
		d := to.Sub(from)

		return d.MulScalar(gm / math.Pow(d.Norm(), 3))
	}

	var (
		helio    vector.Vec3
		actual   = pull(sunGM, body, sun)
		interior = sunGM
	)

	for _, p := range planetSystems {
		pos, _, err := barycentric(sofa, p.id, at)
		if err != nil {
			return centerChoice{}, err
		}

		helio = helio.Add(pull(p.gm, body, pos)).Sub(pull(p.gm, sun, pos))
		actual = actual.Add(pull(p.gm, body, pos))

		if pos.Sub(sun).Norm() < distance {
			interior += p.gm
		}
	}

	// The barycentric fit keeps −GM·r/|r|³ about the origin.
	bary := actual.Add(body.MulScalar(interior / math.Pow(body.Norm(), 3)))

	return centerChoice{helio: helio.Norm(), bary: bary.Norm(), interiorGM: interior, r: r, v: v}, nil
}

// barycentric locates id about the Solar System barycenter at t, following the
// centers the base answers it about.
func barycentric(base core.Provider, id core.ID, t time.Time) (pos, vel vector.Vec3, err error) {
	// The SOFA chains are at most three deep: the Earth-Moon barycenter
	// about the Earth, the Earth about the Sun, the Sun about the barycenter.
	for range 8 {
		st, err := base.Locate(id, t)
		if err != nil {
			return vector.Zero(), vector.Zero(), fmt.Errorf("kepler: locating %v about the barycenter: %w", id, err)
		}

		pos, vel = pos.Add(st.Pos), vel.Add(st.Vel)

		if st.Center == core.SolarSystemBarycenter {
			return pos, vel, nil
		}

		id = st.Center
	}

	return vector.Zero(), vector.Zero(), fmt.Errorf("%w: %v does not reach the barycenter", ErrUnsupportedBody, id)
}
