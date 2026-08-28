package optics_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/optics"
	"github.com/TuSKan/astrogo/unit"
)

func mustGrid(t *testing.T, start, step unit.WavelengthNM, n int) unit.SpectralGrid {
	t.Helper()

	g, err := unit.NewSpectralGrid(start, step, n)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	return g
}

// flat returns a throughput with constant efficiency across a span.
func flat(name string, lo, hi unit.WavelengthNM, eff float64) optics.Throughput {
	return optics.Throughput{
		Name:         name,
		WavelengthNM: []unit.WavelengthNM{lo, hi},
		Efficiency:   []float64{eff, eff},
		Reference:    "synthetic test fixture",
	}
}

func TestThroughputValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		tp   optics.Throughput
		want error
	}{
		{"empty", optics.Throughput{Name: "x"}, optics.ErrThroughputShape},
		{
			"mismatched",
			optics.Throughput{Name: "x", WavelengthNM: []unit.WavelengthNM{1, 2}, Efficiency: []float64{1}},
			optics.ErrThroughputShape,
		},
		{
			"not increasing",
			optics.Throughput{Name: "x", WavelengthNM: []unit.WavelengthNM{2, 1}, Efficiency: []float64{1, 1}},
			optics.ErrThroughputShape,
		},
		{
			"efficiency above one",
			optics.Throughput{Name: "x", WavelengthNM: []unit.WavelengthNM{1, 2}, Efficiency: []float64{1, 1.5}},
			optics.ErrThroughputEfficiency,
		},
		{
			"negative efficiency",
			optics.Throughput{Name: "x", WavelengthNM: []unit.WavelengthNM{1, 2}, Efficiency: []float64{1, -0.1}},
			optics.ErrThroughputEfficiency,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := tc.tp.Validate(); !errors.Is(err, tc.want) {
				t.Errorf("Validate = %v, want %v", err, tc.want)
			}
		})
	}
}

// An empty optical train transmits perfectly — the identity for a product.
func TestSystemEmptyIsUnity(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 10, 11)

	dst := make([]float64, g.Len())
	if err := optics.System(dst, g); err != nil {
		t.Fatalf("System: %v", err)
	}

	for i, v := range dst {
		if v != 1 {
			t.Errorf("System()[%d] = %v, want 1", i, v)
		}
	}
}

// Elements multiply, and an element that says nothing at a wavelength is
// opaque there rather than transparent.
func TestSystemMultipliesAndZerosOutsideRange(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 100, 5) // 400..800

	dst := make([]float64, g.Len())

	err := optics.System(dst, g,
		flat("mirror", 300, 900, 0.9),
		flat("filter", 500, 700, 0.5),
	)
	if err != nil {
		t.Fatalf("System: %v", err)
	}

	want := []float64{0, 0.45, 0.45, 0.45, 0} // filter blocks 400 and 800
	for i := range want {
		if math.Abs(dst[i]-want[i]) > 1e-12 {
			t.Errorf("System()[%d] (%v nm) = %v, want %v", i, g.At(i), dst[i], want[i])
		}
	}
}

// The photon rate of a flat spectrum through a fully-transmitting response
// has a closed form: the trapezoidal sum of L/(hc/lambda) over the grid.
// Checking against that catches an inverted photon-energy factor, which is
// otherwise plausible to within an order of magnitude.
//
// The throughput deliberately spans wider than the grid. A band with edges
// *inside* the grid is sampled with ramps at those edges, so the discrete
// integral legitimately differs from an ideal top-hat by about one sample
// — a discretisation artifact of a sharp-edged filter, not an error. Using
// a band with no interior edge keeps the reference exact.
func TestPhotonRateAgainstClosedForm(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 500, 1, 101) // 500..600

	const radiance = 1e-8 // W m^-2 sr^-1 nm^-1, flat

	spectrum := make([]float64, g.Len())
	for i := range spectrum {
		spectrum[i] = radiance
	}

	inst := optics.Instrument{
		Name:              "test",
		CollectingAreaM2:  1,
		PixelSolidAngleSR: 1,
		Throughput:        []optics.Throughput{flat("band", 400, 800, 1)},
	}

	got, err := inst.PhotonRate(spectrum, g)
	if err != nil {
		t.Fatalf("PhotonRate: %v", err)
	}

	// Independent trapezoidal sum of L*lambda/(hc) across the grid.
	var want float64

	for i := range g.Len() {
		w := 1.0
		if i == 0 || i == g.Len()-1 {
			w = 0.5
		}

		want += w * radiance / constants.PhotonEnergyJ(g.At(i))
	}

	if rel := math.Abs(float64(got)-want) / want; rel > 1e-9 {
		t.Errorf("PhotonRate = %v, want %v (rel %g)", float64(got), want, rel)
	}
}

// The background rate is the photon rate scaled by collecting area and
// pixel solid angle. Both must appear exactly once — a missing or doubled
// factor is the classic radiometric slip.
func TestBackgroundRateScalesWithAreaAndPixel(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 1, 401)

	spectrum := make([]float64, g.Len())
	for i := range spectrum {
		spectrum[i] = 1e-8
	}

	base := optics.Instrument{
		Name:              "base",
		CollectingAreaM2:  1,
		PixelSolidAngleSR: 1e-10,
		Throughput:        []optics.Throughput{flat("band", 500, 600, 1)},
	}

	scaled := base
	scaled.CollectingAreaM2 = 4
	scaled.PixelSolidAngleSR = 3e-10

	a, err := base.BackgroundRate(spectrum, g)
	if err != nil {
		t.Fatalf("BackgroundRate: %v", err)
	}

	b, err := scaled.BackgroundRate(spectrum, g)
	if err != nil {
		t.Fatalf("BackgroundRate: %v", err)
	}

	if rel := math.Abs(float64(b)/float64(a) - 12); rel > 1e-9 {
		t.Errorf("scaling area x4 and pixel x3 gave ratio %v, want 12", float64(b)/float64(a))
	}
}

// NewInstrument derives collecting area from the aperture and pixel solid
// angle from the pixel scale, so the geometry has to be right.
func TestNewInstrumentGeometry(t *testing.T) {
	t.Parallel()

	scope, err := optics.NewTelescope(200, 2000) // 200 mm aperture
	if err != nil {
		t.Fatalf("NewTelescope: %v", err)
	}

	inst, err := optics.NewInstrument("test", scope, angle.Arcsec(1), 0)
	if err != nil {
		t.Fatalf("NewInstrument: %v", err)
	}

	// Unobstructed area of a 0.2 m diameter aperture.
	wantArea := math.Pi * 0.1 * 0.1
	if rel := math.Abs(inst.CollectingAreaM2-wantArea) / wantArea; rel > 1e-12 {
		t.Errorf("CollectingAreaM2 = %v, want %v", inst.CollectingAreaM2, wantArea)
	}

	// One square arcsecond in steradians.
	if rel := math.Abs(inst.PixelSolidAngleSR-constants.ArcsecondSquaredToSteradian) /
		constants.ArcsecondSquaredToSteradian; rel > 1e-12 {
		t.Errorf("PixelSolidAngleSR = %v, want %v", inst.PixelSolidAngleSR, constants.ArcsecondSquaredToSteradian)
	}

	// A central obstruction removes area.
	obstructed, err := optics.NewInstrument("obstructed", scope, angle.Arcsec(1), 0.25)
	if err != nil {
		t.Fatalf("NewInstrument (obstructed): %v", err)
	}

	if rel := math.Abs(obstructed.CollectingAreaM2/inst.CollectingAreaM2 - 0.75); rel > 1e-12 {
		t.Errorf("25%% obstruction left %v of the area, want 0.75",
			obstructed.CollectingAreaM2/inst.CollectingAreaM2)
	}
}

func TestNewInstrumentRejectsBadGeometry(t *testing.T) {
	t.Parallel()

	scope, err := optics.NewTelescope(200, 2000)
	if err != nil {
		t.Fatalf("NewTelescope: %v", err)
	}

	if _, err := optics.NewInstrument("x", scope, angle.Arcsec(1), 1); !errors.Is(err, optics.ErrNonPositiveDimension) {
		t.Errorf("full obstruction = %v, want ErrNonPositiveDimension", err)
	}

	if _, err := optics.NewInstrument("x", scope, angle.Deg(0), 0); !errors.Is(err, optics.ErrNonPositiveDimension) {
		t.Errorf("zero pixel scale = %v, want ErrNonPositiveDimension", err)
	}
}

// A zero sky yields a zero background rate, with no NaN from the photon
// conversion — the Phase 0 state again.
func TestBackgroundRateZeroSky(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 1, 401)

	inst := optics.Instrument{
		Name:              "test",
		CollectingAreaM2:  1,
		PixelSolidAngleSR: 1e-10,
		Throughput:        []optics.Throughput{flat("band", 500, 600, 1)},
	}

	got, err := inst.BackgroundRate(make([]float64, g.Len()), g)
	if err != nil {
		t.Fatalf("BackgroundRate: %v", err)
	}

	if got != 0 {
		t.Errorf("BackgroundRate(zero sky) = %v, want 0", got)
	}
}
