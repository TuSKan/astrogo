package skybrightness_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestOzoneDimsWhatCrossesTheLayer holds each component to where it meets the
// ozone layer (#632). Through 258 DU of ozone, each component's spectrum over
// the same component's with none is exp(-tau * X_O3):
//
//   - light from beyond the atmosphere — starlight, zodiacal and diffuse
//     galactic light, the extragalactic background and airglow — with X_O3
//     of the line of sight;
//   - moonlight with X_O3 of the Moon, whose beam crosses the layer before it
//     scatters below it;
//   - artificial skyglow with no ozone at all, since it never leaves the air
//     below the layer.
//
// tau is computed here from the cross sections atmosphere's table prints at
// four of its nodes, the column, and the Dobson unit, not read back from
// atmosphere. Before #632 every ratio was exactly 1: the scene's ozone column
// was read by nothing.
func TestOzoneDimsWhatCrossesTheLayer(t *testing.T) {
	t.Parallel()

	const column = 258 // Dobson units

	when, _ := nearFullMoonUp(t)
	model, scene, grid := assembleSky(t, when)

	ebl, err := skybrightness.NewModel("extragalactic", skybrightness.NewExtragalacticBackground())
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	// Low enough that the line of sight's ozone airmass, 3.8, is far from the
	// Moon's, above 60 degrees, so charging moonlight the wrong one shows.
	view := coord.NewAltAz(angle.Deg(15), angle.Deg(200))

	estimate := func(m *skybrightness.Model, air *atmosphere.Atmosphere) *skybrightness.Estimate {
		t.Helper()

		s := *scene
		s.Atmosphere = air

		est, err := m.Estimate(context.Background(), skybrightness.Query{Scene: &s, Direction: view, Grid: grid})
		if err != nil {
			t.Fatalf("Estimate: %v", err)
		}

		return est
	}

	clean, dimmed := estimate(model, fullSkyAir(t, 0)), estimate(model, fullSkyAir(t, column))
	cleanEBL, dimmedEBL := estimate(ebl, fullSkyAir(t, 0)), estimate(ebl, fullSkyAir(t, column))

	viewX, err := atmosphere.OzoneAirmass(view.Alt())
	if err != nil {
		t.Fatalf("OzoneAirmass(view): %v", err)
	}

	moonX := moonOzoneAirmass(t, scene)

	// The cross section at 223 K, cm^2 per molecule, at four nodes of the
	// table in atmosphere/extinction.go.
	nodes := []struct {
		nm    unit.WavelengthNM
		sigma float64
	}{{500, 1.1791e-21}, {550, 3.2853e-21}, {600, 5.0348e-21}, {650, 2.4440e-21}}

	ratio := func(clean, dimmed *skybrightness.Estimate, id skybrightness.ComponentID, i int) float64 {
		t.Helper()

		without, ok := clean.Component(id)
		if !ok {
			t.Fatalf("%s: no spectrum without ozone", id)
		}

		with, ok := dimmed.Component(id)
		if !ok {
			t.Fatalf("%s: no spectrum with ozone", id)
		}

		if without[i] <= 0 {
			t.Fatalf("%s at band %d is %g, nothing to dim", id, i, without[i])
		}

		return with[i] / without[i]
	}

	for _, node := range nodes {
		i := gridIndex(t, grid, node.nm)
		tau := node.sigma * column * atmosphere.DobsonUnitMoleculesPerCM2()

		for _, c := range []struct {
			id           skybrightness.ComponentID
			clean, dimmd *skybrightness.Estimate
			x            float64
			tolerance    float64
		}{
			{skybrightness.Starlight, clean, dimmed, viewX, 1e-12},
			{skybrightness.Zodiacal, clean, dimmed, viewX, 1e-12},
			{skybrightness.DiffuseGalactic, clean, dimmed, viewX, 1e-12},
			{skybrightness.AirglowContinuum, clean, dimmed, viewX, 1e-12},
			{skybrightness.Extragalactic, cleanEBL, dimmedEBL, viewX, 1e-12},
			// Resampled from ROLO's bands onto the grid before ozone is
			// applied, so exact at a grid wavelength as well.
			{skybrightness.Moonlight, clean, dimmed, moonX, 1e-12},
			{skybrightness.Artificial, clean, dimmed, 0, 1e-12},
		} {
			want := math.Exp(-tau * c.x)
			if got := ratio(c.clean, c.dimmd, c.id, i); math.Abs(got-want) > c.tolerance {
				t.Errorf("%s at %v nm: with ozone / without = %.12f, want exp(-%.5f * %.4f) = %.12f",
					c.id, node.nm, got, tau, c.x, want)
			}
		}
	}
}

// moonOzoneAirmass is the ozone airmass of the Moon as ScatteredMoonlight
// places it in scene: its ephemeris direction through scene's refraction.
func moonOzoneAirmass(t *testing.T, scene *skybrightness.Scene) float64 {
	t.Helper()

	at := time.FromGo(scene.Time)

	moon, err := scene.Ephemeris.State(eph.Moon, at)
	if err != nil {
		t.Fatalf("moon state: %v", err)
	}

	icrs, err := eph.ToICRS(moon.Pos)
	if err != nil {
		t.Fatalf("ToICRS: %v", err)
	}

	altaz, err := coord.NewContext(at, scene.Observer, scene.Atmosphere.Refraction()).ICRSToAltAz(icrs)
	if err != nil {
		t.Fatalf("ICRSToAltAz: %v", err)
	}

	x, err := atmosphere.OzoneAirmass(altaz.Alt())
	if err != nil {
		t.Fatalf("OzoneAirmass(moon at %v): %v", altaz.Alt(), err)
	}

	return x
}
