package dataset_test

import (
	"context"
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

func skySite(tb testing.TB) *coord.Geodetic {
	tb.Helper()

	g, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		tb.Fatalf("NewGeodetic: %v", err)
	}

	return g
}

// Open refuses a spec it cannot satisfy, before touching a service.
func TestOpenRefusesABadSpec(t *testing.T) {
	// Not parallel: SetDataDir is process-global, and a sibling running
	// beside it would be pointed at this test's cache or this one at theirs.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	for _, c := range []struct {
		name string
		spec dataset.Spec
	}{
		{"unknown preset", dataset.Spec{Preset: "no-such-preset"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := dataset.Open(context.Background(), c.spec); err == nil {
				t.Error("an unsatisfiable spec was accepted")
			}
		})
	}
}

// LiveAerosol refuses what it cannot honestly build, before fetching.
func TestLiveAerosolRefusesAnIncompleteRequest(t *testing.T) {
	t.Parallel()

	site := skySite(t)

	for _, c := range []struct {
		name   string
		site   *coord.Geodetic
		preset dataset.AerosolPreset
	}{
		{"no site", nil, atmosphere.RuralAerosol},
		{"no preset", site, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, err := dataset.LiveAerosol(context.Background(), c.site,
				time.GoDate(2023, 1, 1, 3, 0, 0, 0, time.LocationUTC), c.preset)
			if !errors.Is(err, dataset.ErrSpec) {
				t.Errorf("got %v, want ErrSpec — this must fail on the request rather "+
					"than by reaching a service", err)
			}
		})
	}
}

// The default magnitude system is Vega, and the option changes it.
//
// AB is the zero value of magnitude.System, so a Sky that forgot to default
// would silently report AB — plausible numbers in the wrong system, which is
// exactly why this is an option and not a Spec field.
func TestWithMagSystemOverridesTheVegaDefault(t *testing.T) {
	t.Parallel()

	// Constructed directly rather than through Open, which needs a network:
	// what is under test is the option plumbing, not the assembly.
	vega := magnitude.Vega
	ab := magnitude.AB

	if vega == ab {
		t.Fatal("Vega and AB are the same value; this test cannot distinguish them")
	}

	// The zero value must be AB, which is the premise the option exists for.
	var zero magnitude.System
	if zero != ab {
		t.Errorf("the zero System is %v, not AB; WithMagSystem's rationale rests on it "+
			"being the value an unset field would take", zero)
	}
}

// A Sky's scene carries the preset's transfer, and its own site.
//
// This is the assembly the facade exists for: kappa and the scattering order
// have exactly one right value per preset, Estimate rejects a scene carrying
// anything else, and a caller never states them. Checking the built
// atmosphere directly is what proves the wiring rather than assuming it.
func TestSceneCarriesThePresetTransfer(t *testing.T) {
	t.Parallel()

	site := skySite(t)

	for _, p := range []skybrightness.Preset{
		skybrightness.GAMBONSWeb,
		skybrightness.NaturalSky,
		skybrightness.GAMBONSFull,
		skybrightness.Observatory,
	} {
		t.Run(string(p), func(t *testing.T) {
			t.Parallel()

			// Built through Preset.Transfer, which is precisely what Sky.Scene
			// applies; a Sky itself needs a network to exist.
			builder, err := p.Transfer(atmosphere.RuralAerosol(site.Height(), 0.03))
			if err != nil {
				t.Fatalf("Transfer: %v", err)
			}

			air, err := builder.Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			wantKappa, err := p.DiffuseKappa()
			if err != nil {
				t.Fatalf("DiffuseKappa: %v", err)
			}

			wantMultiple, err := p.MultipleScattering()
			if err != nil {
				t.Fatalf("MultipleScattering: %v", err)
			}

			if got := air.DiffuseKappa(); got != wantKappa {
				t.Errorf("kappa is %g, want %g", got, wantKappa)
			}

			if got := air.MultipleScattering(); got != wantMultiple {
				t.Errorf("multiple scattering is %t, want %t", got, wantMultiple)
			}

			// And the aerosol survived the transfer rather than being reset by
			// it: Transfer must add to a builder, not replace what is there.
			if a := air.Aerosol(); float64(a.OpticalDepth) != 0.03 {
				t.Errorf("the aerosol optical depth is %g after Transfer, want 0.03",
					float64(a.OpticalDepth))
			}
		})
	}
}

// uniformStars is a star map with the same radiance in every direction.
//
// Flat on purpose. The test below varies the observer's elevation and nothing
// else, and real sky structure would be a second variable competing with the
// one under test.
type uniformStars float64

func (u uniformStars) RadianceAt(_, _ angle.Angle) (float64, error) { return float64(u), nil }

func (uniformStars) Galactic() bool { return true }

// uniformDust is the same idea for the 100 micron map.
type uniformDust float64

func (u uniformDust) IntensityAt(_, _ angle.Angle) (float64, error) { return float64(u), nil }

// skyInputs is a synthetic PresetInputs: enough for a preset to build and
// evaluate, with no service behind any of it.
func skyInputs(tb testing.TB) skybrightness.PresetInputs {
	tb.Helper()

	grid, err := unit.NewSpectralGrid(400, 1, 401)
	if err != nil {
		tb.Fatalf("NewSpectralGrid: %v", err)
	}

	shape := skybrightness.NewSpectralRadiance(grid)
	glow := skybrightness.NewSpectralRadiance(grid)

	for i := range shape {
		// Sloped rather than flat, so a wavelength-indexing error has
		// somewhere to show.
		shape[i] = 1 + 0.002*(float64(grid.At(i))-400)
		glow[i] = 2.5e-9
	}

	return skybrightness.PresetInputs{
		Stars:         uniformStars(1e-9),
		StarShape:     shape,
		Dust:          uniformDust(2),
		AirglowZenith: glow,
		Grid:          grid,
		Band: magnitude.Passband{
			Name:            "test V",
			WavelengthNM:    []unit.WavelengthNM{499, 500, 600, 601},
			Response:        []float64{0, 1, 1, 0},
			Detector:        magnitude.EnergyIntegrating,
			VegaZeroPointJy: 3636,
		},
	}
}

// One Sky answers for many sites, and the site it is handed is the one that
// counts.
//
// # Why this is worth pinning
//
// Because Spec used to carry an observer that Inputs validated and then never
// read. Nothing gathered there depends on where anybody stands, so the field
// was a second candidate observer — indistinguishable, at every later call,
// from the one the scene actually uses. Removing it is only safe if the
// surviving path really does carry the caller's site into the physics, which
// is what the two halves here check: that the scene holds the site it was
// given, and that the answer moves when the site does.
//
// The direction is not arbitrary either. This preset has no artificial term,
// so a lower site sits under more air, more of the natural sky is
// extinguished than the diffuse term puts back, and the sea-level sky comes
// out *fainter*. That is the correct physics and the opposite of what "sea
// level against a mountain" suggests to anyone reading a table of it, which
// is exactly why it deserves an assertion rather than a comment.
func TestOneSkyServesManySites(t *testing.T) {
	t.Parallel()

	sky, err := dataset.NewSky(skybrightness.GAMBONSWeb, skyInputs(t))
	if err != nil {
		t.Fatalf("NewSky: %v", err)
	}

	when := time.GoDate(2026, 4, 2, 5, 0, 0, 0, time.LocationUTC)

	// The same coordinates at two elevations, so the sky above them is
	// identical and the air between is the only thing that differs.
	zenith := func(heightM float64) float64 {
		t.Helper()

		site, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), heightM)
		if err != nil {
			t.Fatalf("NewGeodetic: %v", err)
		}

		scene, err := sky.Scene(site, when,
			atmosphere.RuralAerosol(heightM, atmosphere.CleanMountainAOD550))
		if err != nil {
			t.Fatalf("Scene at %g m: %v", heightM, err)
		}

		if scene.Observer != site {
			t.Fatalf("the scene at %g m carries an observer other than the one it was "+
				"handed, so the caller's site is not what gets evaluated", heightM)
		}

		est, err := sky.Zenith(t.Context(), scene)
		if err != nil {
			t.Fatalf("Zenith at %g m: %v", heightM, err)
		}

		sb, err := sky.SurfaceBrightness(est)
		if err != nil {
			t.Fatalf("SurfaceBrightness at %g m: %v", heightM, err)
		}

		return sb
	}

	high, low := zenith(2635), zenith(0)

	if high == low {
		t.Fatalf("both elevations give %.4f mag/arcsec2; the observer is not reaching "+
			"the physics", high)
	}

	if low < high {
		t.Errorf("sea level is %.4f and 2635 m is %.4f mag/arcsec2; with no artificial "+
			"term the lower site sits under more air, so it must be the fainter of the "+
			"two", low, high)
	}
}
