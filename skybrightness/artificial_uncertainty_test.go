package skybrightness_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
)

// The model's own figure: the top of Kocifaj, Bara & Falchi (2022) Sec. 3's
// ranges. TestKocifajFiguresMatchTheProvenance holds the code to it.
const kocifajModel = 0.25

// statingCity is cityAt with a stated source uncertainty.
func statingCity(t *testing.T, bearingDeg, distanceKM, radiance, relative float64) *skybrightness.UniformEmitter {
	t.Helper()

	e := cityAt(t, bearingDeg, distanceKM, radiance)
	e.RelativeUncertainty = relative

	return e
}

// artificialContribution evaluates the component once, at alt/az in
// artificialScene's air, and returns what it reports.
func artificialContribution(
	t *testing.T, emitters []skybrightness.GroundEmitter, scene *skybrightness.Scene, alt, az float64,
) skybrightness.Contribution {
	t.Helper()

	c, err := skybrightness.NewArtificialSkyglow(emitters)
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	contribution, err := c.AddRadiance(context.Background(), dst, grid, coord.NewAltAz(angle.Deg(alt), angle.Deg(az)), scene)
	if err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	return contribution
}

func TestUniformEmitterSourceUncertainty(t *testing.T) {
	t.Parallel()

	if r, known := (&skybrightness.UniformEmitter{}).SourceUncertainty(); known || r != 0 {
		t.Errorf("an emitter that states nothing reports (%v, %v), want (0, false)", r, known)
	}

	if r, known := (&skybrightness.UniformEmitter{RelativeUncertainty: 0.2}).SourceUncertainty(); !known || r != 0.2 {
		t.Errorf("an emitter stating 0.2 reports (%v, %v), want (0.2, true)", r, known)
	}
}

func TestArtificialSkyglowRejectsAnInvalidSourceUncertainty(t *testing.T) {
	t.Parallel()

	for _, bad := range []float64{-0.1, math.NaN(), math.Inf(1)} {
		_, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{statingCity(t, 0, 20, 1e-3, bad)})
		if !errors.Is(err, skybrightness.ErrUncertaintyValue) {
			t.Errorf("a source stating %v: err = %v, want ErrUncertaintyValue", bad, err)
		}
	}
}

// TestArtificialSkyglowUncertaintyCombinesModelAndInventory holds the budget
// to its definition (#933): in each band, the sources' stated uncertainties
// weighted by their shares of the radiance, added linearly, then in
// quadrature with the model's 25%.
//
// The cases are built so the shares are known exactly. Two sources at the
// same distance on opposite bearings, seen at the zenith, are both 90 degrees
// away in scattering angle, so they add equal radiance in every band, or nine
// to one when one is nine times brighter. The answer is then the same in
// every band, and the spectral weighting cannot hide a wrong combination.
func TestArtificialSkyglowUncertaintyCombinesModelAndInventory(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)

	for _, c := range []struct {
		name     string
		emitters []skybrightness.GroundEmitter
		want     float64
	}{
		{
			"one source at 10%",
			[]skybrightness.GroundEmitter{statingCity(t, 90, 20, 1e-3, 0.10)},
			math.Hypot(kocifajModel, 0.10),
		},
		{
			"two equal sources at 10% and 30%",
			[]skybrightness.GroundEmitter{
				statingCity(t, 90, 20, 1e-3, 0.10),
				statingCity(t, 270, 20, 1e-3, 0.30),
			},
			math.Hypot(kocifajModel, 0.5*0.10+0.5*0.30),
		},
		{
			"a source nine times brighter at 10%, beside one at 30%",
			[]skybrightness.GroundEmitter{
				statingCity(t, 90, 20, 9e-3, 0.10),
				statingCity(t, 270, 20, 1e-3, 0.30),
			},
			math.Hypot(kocifajModel, 0.9*0.10+0.1*0.30),
		},
	} {
		got := artificialContribution(t, c.emitters, scene, 90, 0)

		if !got.Known {
			t.Errorf("%s: the uncertainty is unknown", c.name)

			continue
		}

		if math.Abs(got.Relative-c.want) > 1e-12 {
			t.Errorf("%s: relative uncertainty %.15g, want %.15g", c.name, got.Relative, c.want)
		}
	}
}

// A lit source that states nothing leaves the budget unknown, whatever the
// others state: its share of the error is not zero, only unreported.
func TestArtificialSkyglowUncertaintyIsUnknownWhileASourceStatesNone(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)

	for _, c := range []struct {
		name     string
		emitters []skybrightness.GroundEmitter
	}{
		{"no source states one", []skybrightness.GroundEmitter{cityAt(t, 0, 20, 1e-3)}},
		{"one of two states none", []skybrightness.GroundEmitter{
			statingCity(t, 90, 20, 1e-3, 0.10),
			cityAt(t, 270, 20, 1e-3),
		}},
	} {
		if got := artificialContribution(t, c.emitters, scene, 90, 0); got.Known {
			t.Errorf("%s: the uncertainty is known, %.4g", c.name, got.Relative)
		}
	}
}

// The figure reaches the estimate's budget: a model of this component alone
// reports it as the total.
func TestArtificialSkyglowUncertaintyReachesTheBudget(t *testing.T) {
	t.Parallel()

	c, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{statingCity(t, 0, 20, 1e-3, 0.10)})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	model, err := skybrightness.NewModel("artificial-only", c)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	est, err := model.Estimate(context.Background(), skybrightness.Query{
		Scene:     artificialScene(t),
		Direction: coord.NewAltAz(angle.Deg(60), angle.Deg(0)),
		Grid:      skybrightness.DefaultOpticalGrid(),
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	total, err := est.TotalUncertainty()
	if err != nil {
		t.Fatalf("TotalUncertainty: %v", err)
	}

	if want := math.Hypot(kocifajModel, 0.10); math.Abs(total-want) > 1e-12 {
		t.Errorf("TotalUncertainty = %.15g, want %.15g", total, want)
	}
}

// TestArtificialSkyglowFlagsWhatThePaperDidNotValidate: Kocifaj, Bara &
// Falchi (2022) Sec. 3 validated sources 1.3 to 81.3 km away and aerosol
// optical depths up to 0.57 (#933).
func TestArtificialSkyglowFlagsWhatThePaperDidNotValidate(t *testing.T) {
	t.Parallel()

	extrapolated := func(emitters []skybrightness.GroundEmitter, scene *skybrightness.Scene) bool {
		return artificialContribution(t, emitters, scene, 30, 0).Flags&skybrightness.ExtrapolatedModel != 0
	}

	scene := artificialScene(t)

	for _, c := range []struct {
		km   float64
		want bool
	}{
		{1.0, true},
		{2, false},
		{20, false},
		{80, false},
		{100, true},
	} {
		if got := extrapolated([]skybrightness.GroundEmitter{cityAt(t, 0, c.km, 1e-3)}, scene); got != c.want {
			t.Errorf("a source %.1f km away: ExtrapolatedModel = %v, want %v", c.km, got, c.want)
		}
	}

	// The aerosol: flat in wavelength, so every band has the same optical
	// depth, and an asymmetry of 0.5, at which Eq. 4/5 stay physical to about
	// 0.7. At this test scene's 0.7 they leave the range near 0.5 and the
	// asymmetry clamp sets the same flag, which would hide this one.
	for _, c := range []struct {
		aod  float64
		want bool
	}{
		{0.55, false},
		{0.60, true},
	} {
		air, err := atmosphere.NewBuilder().
			Surface(743, 284).
			Aerosol(c.aod, 550, 0, 0.9, 0.5).
			AerosolScaleHeight(1500).
			Build()
		if err != nil {
			t.Fatalf("atmosphere Build: %v", err)
		}

		hazy := *scene
		hazy.Atmosphere = air

		if got := extrapolated([]skybrightness.GroundEmitter{cityAt(t, 0, 20, 1e-3)}, &hazy); got != c.want {
			t.Errorf("an aerosol optical depth of %.2f: ExtrapolatedModel = %v, want %v; the paper ran up to 0.57",
				c.aod, got, c.want)
		}
	}
}
