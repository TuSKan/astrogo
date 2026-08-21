package skybrightness_test

import (
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/skybrightness"
)

// sceneWithPressure builds two scenes that differ only in surface conditions.
//
// Same epoch, same site. What changes is the refraction the transform context
// is built with, and the optical depth the extinction uses.
func sceneWithPressure(t *testing.T, loc *coord.Geodetic, hPa, kelvin float64) *skybrightness.Scene {
	t.Helper()

	atm, err := atmosphere.NewBuilder().
		Surface(hPa, kelvin).
		Aerosol(0.02, 550, 1.3, 0.95, 0.65).
		BoundaryLayer(1500).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   loc,
		Time:       gotime.Date(2026, 3, 20, 5, 0, 0, 0, gotime.UTC),
		Atmosphere: atm,
		Ephemeris:  eph.Default(),
	}
}

// A scene's answer must not depend on which scene was evaluated before it.
//
// Components cache the transform context per scene so the expensive SOFA
// reduction happens once per epoch rather than once per direction. The cache
// was keyed on the epoch and the observer alone — but the context is built with
// the atmosphere's refraction, and Context.AltAzToICRS feeds the pressure,
// temperature, humidity and wavelength straight into Atoc13. Two scenes at the
// same time and place with different surface conditions therefore shared one
// context, and the second one silently reused the first one's refraction.
//
// Refraction is tens of arcminutes near the horizon, which is more than an
// order-8 pixel, so on a real map the stale context reads the sky from the
// wrong place — and the value it finds there is an ordinary radiance.
//
// Evaluating each scene from cold and comparing against the same scene
// evaluated after its neighbour is what makes a stale key visible: a correct
// cache gives the same answer both ways.
func TestSceneResultsDoNotDependOnEvaluationOrder(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()

	// One observer, shared. coord.NewGeodetic returns a pointer and the frame
	// cache compares observers by identity, so two separately-constructed sites
	// never hit the cache however identical they are. A caller modelling one
	// site under changing weather reuses the site, which is when the cache is
	// live and a key that omits the atmosphere matters.
	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	thick := sceneWithPressure(t, loc, 1013, 300)
	thin := sceneWithPressure(t, loc, 500, 250)

	// Near the horizon, where refraction is largest.
	dir := coord.NewAltAz(angle.Deg(3), angle.Deg(90))

	// A sky that varies with direction, because a uniform one cannot show this
	// at all: if every sightline has the same radiance then pointing the
	// telescope somewhere else changes nothing, and a stale refraction is
	// invisible no matter how wrong it is. The first version of this test used
	// the uniform fixture and could not have failed.
	build := func() map[string]skybrightness.Component {
		out := map[string]skybrightness.Component{}

		isl, err := skybrightness.NewIntegratedStarlight(
			gradientSky{}, solarShape(grid), grid, testBand())
		if err != nil {
			t.Fatalf("NewIntegratedStarlight: %v", err)
		}

		out["starlight"] = isl

		dgl, err := skybrightness.NewDiffuseGalacticLight(
			uniformDust(3.0), gradientSky{}, testBand())
		if err != nil {
			t.Fatalf("NewDiffuseGalacticLight: %v", err)
		}

		out["diffuse-galactic"] = dgl
		out["zodiacal"] = skybrightness.NewZodiacalLight()

		return out
	}

	for name := range build() {
		// Cold: a component that has never seen another scene.
		coldThin, errCold := evaluate(t, build()[name], thin, grid, dir)
		if errCold != nil {
			continue
		}

		// Warm: the same component, asked about the thick scene first.
		warm := build()[name]

		if _, err := evaluate(t, warm, thick, grid, dir); err != nil {
			continue
		}

		warmThin, err := evaluate(t, warm, thin, grid, dir)
		if err != nil {
			continue
		}

		for i := range coldThin {
			if coldThin[i] != warmThin[i] {
				t.Errorf("%s: the thin-atmosphere answer at %.0f nm is %.9e from cold and "+
					"%.9e after the thick scene; a cached frame is outliving its scene",
					name, float64(grid.At(i)), coldThin[i], warmThin[i])

				break
			}
		}
	}
}

// gradientSky varies steeply with direction, so a sightline that moves by an
// arcminute returns a different radiance.
type gradientSky struct{}

func (gradientSky) RadianceAt(lon, lat angle.Angle) (float64, error) {
	// Steep enough that refraction-scale shifts are visible, smooth enough that
	// the value stays an ordinary radiance.
	return 1e-9 * (1 + 0.5*lon.Sin() + 0.5*lat.Sin()), nil
}

func (gradientSky) Galactic() bool { return false }
