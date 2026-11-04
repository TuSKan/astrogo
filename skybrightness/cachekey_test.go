package skybrightness_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
)

// requireSame fails unless a reused component's answer equals a fresh one's.
// Both run the same arithmetic on the same inputs, so they agree exactly;
// the tolerance is only there to say so without asserting bit identity.
func requireSame(t *testing.T, what string, reused, fresh float64) {
	t.Helper()

	if math.Abs(reused-fresh) > 1e-12*math.Abs(fresh) {
		t.Errorf("%s: the reused component gives %.10g, a fresh one %.10g", what, reused, fresh)
	}
}

// TestArtificialSkyglowCacheRespectsTheAtmosphere: the component cached its
// per-band optical depths against the observer and the time only, so a site
// reused under new weather kept the first weather's sky. Measured before the
// fix, a hazy sky after a clean one came out 94x too bright (#945).
func TestArtificialSkyglowCacheRespectsTheAtmosphere(t *testing.T) {
	t.Parallel()

	air := func(aod float64) *atmosphere.Atmosphere {
		a, err := atmosphere.NewBuilder().
			Surface(743, 284).
			Aerosol(aod, 550, 1.3, 0.95, 0.65).
			AerosolScaleHeight(1500).
			Build()
		if err != nil {
			t.Fatalf("atmosphere Build: %v", err)
		}

		return a
	}

	clean := artificialScene(t)
	clean.Atmosphere = air(0.02)

	// The same site and instant, under different weather.
	hazy := *clean
	hazy.Atmosphere = air(0.4)

	component := func() *skybrightness.ArtificialSkyglow {
		c, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{cityAt(t, 0, 20, 1e-3)})
		if err != nil {
			t.Fatalf("NewArtificialSkyglow: %v", err)
		}

		return c
	}

	reused := component()
	before := radianceAt(t, reused, clean, 30, 0)
	after := radianceAt(t, reused, &hazy, 30, 0)
	fresh := radianceAt(t, component(), &hazy, 30, 0)

	// The premise: the two atmospheres give different skies.
	if math.Abs(before-fresh) <= 1e-6*fresh {
		t.Fatalf("clean %.6g and hazy %.6g air give the same sky, so this tests nothing", before, fresh)
	}

	requireSame(t, "hazy air after clean", after, fresh)
}

// TestScatteredMoonlightCacheRespectsRefraction: the component cached the
// Moon's observed direction and airmass, both of which go through the
// atmosphere's refraction, against the observer and the time only (#945).
func TestScatteredMoonlightCacheRespectsRefraction(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)

	refracting := moonlightScene(t, when)

	still, err := atmosphere.NewBuilder().
		Surface(743, 284).
		Aerosol(0.02, 550, 1.3, 0.95, 0.65).
		Refraction(atmosphere.RefractionNone{}, 0, 0.55).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	// The same site and instant, without refraction.
	unrefracted := *refracting
	unrefracted.Atmosphere = still

	grid := skybrightness.DefaultOpticalGrid()
	idx := gridIndex(t, grid, 554)

	at := func(m *skybrightness.ScatteredMoonlight, scene *skybrightness.Scene) float64 {
		t.Helper()

		dst := skybrightness.NewSpectralRadiance(grid)
		if _, err := m.AddRadiance(context.Background(), dst, grid,
			coord.NewAltAz(angle.Deg(10), angle.Deg(0)), scene); err != nil {
			t.Fatalf("AddRadiance: %v", err)
		}

		return dst[idx]
	}

	component := func() *skybrightness.ScatteredMoonlight {
		m, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
		if err != nil {
			t.Fatalf("NewScatteredMoonlight: %v", err)
		}

		return m
	}

	reused := component()
	before := at(reused, refracting)
	after := at(reused, &unrefracted)
	fresh := at(component(), &unrefracted)

	if before == fresh {
		t.Fatalf("refraction does not change the sky here (%.10g), so this tests nothing", fresh)
	}

	requireSame(t, "unrefracted air after refracting", after, fresh)
}
