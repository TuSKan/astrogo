package dataset_test

import (
	"context"
	"errors"
	"math"
	"slices"
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
	// Captured first, so the tests after this one do not inherit its deleted
	// cache directory (#971).
	t.Cleanup(remote.Capture().Restore)
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

// LiveAerosol builds nothing when the optical depth cannot be had.
//
// The value of this function is that its optical depth was measured, so a
// failed fetch must reach the caller rather than become a builder carrying
// some other number. Without consent and with an empty cache the fetch is
// refused before any source is asked.
func TestLiveAerosolPassesOnARefusedFetch(t *testing.T) {
	// Not parallel: consent and the data directory are process-global.
	t.Cleanup(remote.Capture().Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	remote.DisableDownloads()

	air, err := dataset.LiveAerosol(context.Background(), skySite(t),
		time.GoDate(2023, 1, 1, 3, 0, 0, 0, time.LocationUTC), atmosphere.RuralAerosol)
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Errorf("got %v, want the fetch's ErrDownloadDenied", err)
	}

	if air != nil {
		t.Error("LiveAerosol returned a builder although no optical depth was fetched")
	}
}

// The default magnitude system is Vega, and the option changes it.
//
// AB is the zero value of magnitude.System, so a Sky that forgot to default
// would silently report AB — plausible numbers in the wrong system, which is
// exactly why this is an option and not a Spec field.
//
// Until the audit's coverage pass this test checked only that Vega and AB
// are different values and that the zero value is AB, and never built a Sky
// or called WithMagSystem; the option it is named for could have done nothing
// and it would still pass. It now holds both of a Sky's projections, in each
// system, to the estimate's own.
func TestWithMagSystemOverridesTheVegaDefault(t *testing.T) {
	t.Parallel()

	// The zero value must be AB, which is the premise the option exists for.
	var zero magnitude.System
	if zero != magnitude.AB || magnitude.Vega == magnitude.AB {
		t.Fatalf("the zero System is %v and Vega is %v; WithMagSystem's rationale rests on "+
			"AB being the value an unset field would take, and Vega a different one",
			zero, magnitude.Vega)
	}

	in := skyInputs(t)

	byDefault, err := dataset.NewSky(skybrightness.GAMBONSWeb, in)
	if err != nil {
		t.Fatalf("NewSky: %v", err)
	}

	ab, err := dataset.NewSky(skybrightness.GAMBONSWeb, in, dataset.WithMagSystem(magnitude.AB))
	if err != nil {
		t.Fatalf("NewSky(WithMagSystem(AB)): %v", err)
	}

	// One estimate for both, so the system is the only thing that differs.
	scene, err := byDefault.Scene(skySite(t), time.GoDate(2026, 4, 2, 5, 0, 0, 0, time.LocationUTC), nil)
	if err != nil {
		t.Fatalf("Scene: %v", err)
	}

	est, err := byDefault.Zenith(t.Context(), scene)
	if err != nil {
		t.Fatalf("Zenith: %v", err)
	}

	reported := map[magnitude.System]float64{}

	for _, c := range []struct {
		name string
		sky  *dataset.Sky
		want magnitude.System
	}{
		{"default", byDefault, magnitude.Vega},
		{"WithMagSystem(AB)", ab, magnitude.AB},
	} {
		got, err := c.sky.SurfaceBrightness(est)
		if err != nil {
			t.Fatalf("%s: SurfaceBrightness: %v", c.name, err)
		}

		want, err := est.SurfaceBrightness(in.Band, c.want)
		if err != nil {
			t.Fatalf("%s: Estimate.SurfaceBrightness: %v", c.name, err)
		}

		if got != want {
			t.Errorf("%s: the Sky reports %.6f mag/arcsec2, but the estimate in %v is %.6f",
				c.name, got, c.want, want)
		}

		reported[c.want] = got

		rows, err := c.sky.Composition(est)
		if err != nil {
			t.Fatalf("%s: Composition: %v", c.name, err)
		}

		wantRows, err := est.Composition(in.Band, c.want)
		if err != nil {
			t.Fatalf("%s: Estimate.Composition: %v", c.name, err)
		}

		// Brightnesses compare exactly, since each is one component's own.
		// The fractions share a total that Composition sums in map order
		// (#1066), so two calls on one estimate can differ by that sum's
		// regrouping: at most one rounding per term, len(rows) eps relative.
		tol := float64(len(wantRows)) * 0x1p-52

		same := len(rows) == len(wantRows)
		for i := 0; same && i < len(rows); i++ {
			g, w := rows[i], wantRows[i]
			same = g.Component == w.Component && g.Brightness == w.Brightness &&
				math.Abs(g.Fraction-w.Fraction) <= tol*w.Fraction
		}

		if !same {
			t.Errorf("%s: the Sky's composition is\n%v\nbut the estimate's in %v is\n%v",
				c.name, rows, c.want, wantRows)
		}
	}

	// Without this the two rows above could agree by both being one system.
	if reported[magnitude.Vega] == reported[magnitude.AB] {
		t.Fatalf("Vega and AB both read %.6f mag/arcsec2 through this band; the test "+
			"cannot tell the systems apart", reported[magnitude.Vega])
	}
}

// A Sky hands back what it was built from, and that is what it evaluates.
//
// Model and Inputs are the escape hatches for a caller who needs something
// the facade does not expose, and Inputs exists so that a second model can be
// built without fetching again. Both are only useful if they are the real
// thing: a model built from Inputs must give the Sky's own answer, and so must
// the model Model returns, asked at the preset's grid and fidelity.
func TestASkyHandsBackWhatItEvaluates(t *testing.T) {
	t.Parallel()

	in := skyInputs(t)

	sky, err := dataset.NewSky(skybrightness.GAMBONSWeb, in)
	if err != nil {
		t.Fatalf("NewSky: %v", err)
	}

	if sky.Preset() != skybrightness.GAMBONSWeb {
		t.Errorf("Preset is %q, want %q", sky.Preset(), skybrightness.GAMBONSWeb)
	}

	if sky.Grid() != in.Grid {
		t.Errorf("Grid is %v, want the inputs' %v", sky.Grid(), in.Grid)
	}

	if got := sky.Band(); got.Name != in.Band.Name || !slices.Equal(got.Response, in.Band.Response) {
		t.Errorf("Band is %q, want the inputs' %q", got.Name, in.Band.Name)
	}

	scene, err := sky.Scene(skySite(t), time.GoDate(2026, 4, 2, 5, 0, 0, 0, time.LocationUTC), nil)
	if err != nil {
		t.Fatalf("Scene: %v", err)
	}

	est, err := sky.Zenith(t.Context(), scene)
	if err != nil {
		t.Fatalf("Zenith: %v", err)
	}

	fidelity, err := sky.Preset().Fidelity()
	if err != nil {
		t.Fatalf("Fidelity: %v", err)
	}

	rebuilt, err := skybrightness.NewPreset(sky.Preset(), sky.Inputs())
	if err != nil {
		t.Fatalf("NewPreset from the Sky's Inputs: %v", err)
	}

	for name, model := range map[string]*skybrightness.Model{
		"Model()":                  sky.Model(),
		"NewPreset(..., Inputs())": rebuilt,
	} {
		got, err := model.Zenith(t.Context(), skybrightness.Query{
			Scene: scene, Grid: sky.Grid(), Fidelity: fidelity,
		})
		if err != nil {
			t.Fatalf("%s: Zenith: %v", name, err)
		}

		if !slices.Equal(got.SpectralRadiance(), est.SpectralRadiance()) {
			t.Errorf("%s gives a zenith spectrum other than the Sky's own", name)
		}
	}
}

// SkyMap evaluates the hemisphere at the ring count asked for, on the Sky's
// own grid, and its result feeds HorizontalIlluminance directly, as its doc
// says.
func TestSkyMapFeedsTheHemisphereIntegrals(t *testing.T) {
	t.Parallel()

	sky, err := dataset.NewSky(skybrightness.GAMBONSWeb, skyInputs(t))
	if err != nil {
		t.Fatalf("NewSky: %v", err)
	}

	scene, err := sky.Scene(skySite(t), time.GoDate(2026, 4, 2, 5, 0, 0, 0, time.LocationUTC), nil)
	if err != nil {
		t.Fatalf("Scene: %v", err)
	}

	counts := map[int]int{}

	for _, rings := range []int{2, 4} {
		points, err := sky.SkyMap(t.Context(), scene, rings)
		if err != nil {
			t.Fatalf("SkyMap(%d): %v", rings, err)
		}

		counts[rings] = len(points)

		for _, p := range points {
			if p.Estimate == nil || p.Estimate.Grid() != sky.Grid() {
				t.Fatalf("SkyMap(%d): a point at %v is not on the Sky's grid", rings, p.Direction)
			}
		}

		e, err := skybrightness.HorizontalIlluminance(points, sky.Grid())
		if err != nil {
			t.Fatalf("HorizontalIlluminance(SkyMap(%d)): %v", rings, err)
		}

		if v := float64(e); !(v > 0) || math.IsInf(v, 0) {
			t.Errorf("the illuminance from SkyMap(%d) is %g; a lit sky gives a positive, "+
				"finite one", rings, v)
		}
	}

	if counts[4] <= counts[2] {
		t.Errorf("4 rings give %d points and 2 give %d; the ring count is not reaching "+
			"the map", counts[4], counts[2])
	}

	if _, err := sky.SkyMap(t.Context(), scene, 0); !errors.Is(err, skybrightness.ErrRingCount) {
		t.Errorf("SkyMap(0) returned %v, want ErrRingCount", err)
	}
}

// A Sky refuses what it cannot evaluate, with the error that says why, and
// never dereferences what it was not given.
func TestASkyRefusesWhatItCannotEvaluate(t *testing.T) {
	t.Parallel()

	in := skyInputs(t)

	if _, err := dataset.NewSky("no-such-preset", in); err == nil {
		t.Error("NewSky accepted an unknown preset")
	}

	noDust := in
	noDust.Dust = nil

	if _, err := dataset.NewSky(skybrightness.GAMBONSWeb, noDust); !errors.Is(err, skybrightness.ErrPreset) {
		t.Errorf("NewSky with no dust map returned %v, want ErrPreset", err)
	}

	sky, err := dataset.NewSky(skybrightness.GAMBONSWeb, in)
	if err != nil {
		t.Fatalf("NewSky: %v", err)
	}

	when := time.GoDate(2026, 4, 2, 5, 0, 0, 0, time.LocationUTC)

	if _, err := sky.Scene(nil, when, nil); !errors.Is(err, dataset.ErrSpec) {
		t.Errorf("Scene with no site returned %v, want ErrSpec", err)
	}

	// An atmosphere the caller described wrongly fails at Scene, where they
	// described it, rather than at the first evaluation.
	bad := atmosphere.NewBuilder().Ozone(-1)
	if _, err := sky.Scene(skySite(t), when, bad); !errors.Is(err, atmosphere.ErrAtmosphereBuilder) {
		t.Errorf("Scene with a negative ozone column returned %v, want ErrAtmosphereBuilder", err)
	}

	// No scene at all is the caller's error to hear about, not a panic.
	if _, err := sky.Zenith(t.Context(), nil); !errors.Is(err, skybrightness.ErrNoObserver) {
		t.Errorf("Zenith(nil) returned %v, want ErrNoObserver", err)
	}

	if _, err := sky.Direction(t.Context(), nil, angle.Deg(45), angle.Deg(0)); !errors.Is(err, skybrightness.ErrNoObserver) {
		t.Errorf("Direction(nil) returned %v, want ErrNoObserver", err)
	}

	if _, err := sky.SkyMap(t.Context(), nil, 2); !errors.Is(err, skybrightness.ErrNoObserver) {
		t.Errorf("SkyMap(nil) returned %v, want ErrNoObserver", err)
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
			builder, err := p.Transfer(atmosphere.RuralAerosol(unit.Meters(site.Height().Meters()), 0.03))
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

		site, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), unit.Meters(heightM))
		if err != nil {
			t.Fatalf("NewGeodetic: %v", err)
		}

		scene, err := sky.Scene(site, when,
			atmosphere.RuralAerosol(unit.Meters(heightM), atmosphere.CleanMountainAOD550))
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

// TestTheDefaultSceneCarriesOzone: a nil builder is a clear night, and a clear
// night has ozone.
//
// Until #906 the default carried none. Every component crossing the layer
// has applied ozone absorption since #632, but on the default path that
// absorption was exactly 1, and the sky came out 0.024 mag too bright at the
// zenith and 0.127 at 10° at Paranal. The default is now the column
// plan.VisibleTonight's air carries. Its effect has to grow toward the
// horizon, as the ozone airmass does.
func TestTheDefaultSceneCarriesOzone(t *testing.T) {
	t.Parallel()

	sky, err := dataset.NewSky(skybrightness.GAMBONSWeb, skyInputs(t))
	if err != nil {
		t.Fatalf("NewSky: %v", err)
	}

	site := skySite(t)
	when := time.GoDate(2026, 4, 2, 5, 0, 0, 0, time.LocationUTC)

	def, err := sky.Scene(site, when, nil)
	if err != nil {
		t.Fatalf("Scene(nil): %v", err)
	}

	if got := def.Atmosphere.Ozone(); got != atmosphere.SubtropicalOzoneDU {
		t.Fatalf("the default scene carries %g DU of ozone, want %d", float64(got), atmosphere.SubtropicalOzoneDU)
	}

	// The same air with nothing absorbing, which is what the default was.
	none, err := sky.Scene(site, when, atmosphere.NewBuilder().SurfaceAtAltitude(site.Height()))
	if err != nil {
		t.Fatalf("Scene without ozone: %v", err)
	}

	brightness := func(scene *skybrightness.Scene, deg float64) float64 {
		t.Helper()

		est, err := sky.Direction(t.Context(), scene, angle.Deg(deg), angle.Deg(0))
		if err != nil {
			t.Fatalf("Direction(%g°): %v", deg, err)
		}

		sb, err := sky.SurfaceBrightness(est)
		if err != nil {
			t.Fatalf("SurfaceBrightness(%g°): %v", deg, err)
		}

		return sb
	}

	prev := 0.0

	for _, deg := range []float64{90, 30, 10} {
		dimmer := brightness(def, deg) - brightness(none, deg)

		if !(dimmer > prev) {
			t.Errorf("at %g° the ozone dims the default sky by %.4f mag, not more than the %.4f "+
				"higher up; it absorbs, and its path grows toward the horizon", deg, dimmer, prev)
		}

		t.Logf("%g°: the default sky is %.4f mag dimmer than with no ozone", deg, dimmer)

		prev = dimmer
	}
}
