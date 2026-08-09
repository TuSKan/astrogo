package atmos

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// TestRayleighOnly_ZenithHasHighestTransmission confirms transmission is
// highest at zenith and decreases toward the horizon (more airmass, more
// extinction), and every value stays in (0,1].
func TestRayleighOnly_ZenithHasHighestTransmission(t *testing.T) {
	r := NewRayleighOnly()
	grid := skybrightness.DefaultOpticalGrid()

	atm, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("build atmosphere: %v", err)
	}

	zenith := make([]unit.Transmission, grid.Len())
	if err := r.LineOfSight(coord.NewAltAz(angle.Deg(90), angle.Deg(0)), atm, grid, zenith); err != nil {
		t.Fatalf("LineOfSight(zenith): %v", err)
	}

	low := make([]unit.Transmission, grid.Len())
	if err := r.LineOfSight(coord.NewAltAz(angle.Deg(15), angle.Deg(0)), atm, grid, low); err != nil {
		t.Fatalf("LineOfSight(low): %v", err)
	}

	for i := range zenith {
		if zenith[i] <= 0 || zenith[i] > 1 {
			t.Fatalf("wavelength %d: zenith transmission %v out of (0,1]", i, zenith[i])
		}

		if low[i] >= zenith[i] {
			t.Errorf("wavelength %d: low-altitude transmission %v should be < zenith %v", i, low[i], zenith[i])
		}
	}
}

// TestRayleighOnly_BluerIsMoreAttenuated confirms shorter wavelengths are
// attenuated more than longer ones (the real, physical sign of Rayleigh
// scattering — sky is blue because blue is scattered away preferentially).
func TestRayleighOnly_BluerIsMoreAttenuated(t *testing.T) {
	r := NewRayleighOnly()
	grid := skybrightness.DefaultOpticalGrid()

	atm, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("build atmosphere: %v", err)
	}

	out := make([]unit.Transmission, grid.Len())
	if err := r.LineOfSight(coord.NewAltAz(angle.Deg(45), angle.Deg(0)), atm, grid, out); err != nil {
		t.Fatalf("LineOfSight: %v", err)
	}

	if out[0] >= out[grid.Len()-1] {
		t.Errorf("blue end (%v nm) transmission %v should be < red end (%v nm) transmission %v",
			float64(grid.At(0)), out[0], float64(grid.At(grid.Len()-1)), out[grid.Len()-1])
	}
}

// TestRayleighOnly_BelowHorizonErrors confirms a direction at/below the
// horizon returns an error rather than a meaningless transmission value.
func TestRayleighOnly_BelowHorizonErrors(t *testing.T) {
	r := NewRayleighOnly()
	grid := skybrightness.DefaultOpticalGrid()

	atm, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("build atmosphere: %v", err)
	}

	out := make([]unit.Transmission, grid.Len())
	if err := r.LineOfSight(coord.NewAltAz(angle.Deg(-5), angle.Deg(0)), atm, grid, out); err == nil {
		t.Error("expected an error for a below-horizon direction, got nil")
	}
}

// TestRayleighOnly_HigherPressureMoreAttenuation confirms a higher
// surface pressure (more overhead air) increases optical depth, so
// transmission decreases.
func TestRayleighOnly_HigherPressureMoreAttenuation(t *testing.T) {
	r := NewRayleighOnly()
	grid := skybrightness.DefaultOpticalGrid()
	dir := coord.NewAltAz(angle.Deg(60), angle.Deg(0))

	seaLevel, err := atmosphere.NewBuilder().Surface(1013.25, 288.15).Build()
	if err != nil {
		t.Fatalf("build sea-level atmosphere: %v", err)
	}

	highAlt, err := atmosphere.NewBuilder().Surface(600, 270).Build()
	if err != nil {
		t.Fatalf("build high-altitude atmosphere: %v", err)
	}

	tSea := make([]unit.Transmission, grid.Len())
	if err := r.LineOfSight(dir, seaLevel, grid, tSea); err != nil {
		t.Fatalf("LineOfSight(sea level): %v", err)
	}

	tHigh := make([]unit.Transmission, grid.Len())
	if err := r.LineOfSight(dir, highAlt, grid, tHigh); err != nil {
		t.Fatalf("LineOfSight(high altitude): %v", err)
	}

	for i := range tSea {
		if tHigh[i] <= tSea[i] {
			t.Errorf("wavelength %d: lower-pressure transmission %v should exceed sea-level %v", i, tHigh[i], tSea[i])
		}
	}
}

// TestRayleighOptical_DepthMatchesHansenTravisAt550nm sanity-checks the
// approximate formula against the commonly quoted sea-level Rayleigh
// optical depth at 550 nm (~0.097, e.g. as cited in atmospheric-optics
// references summarizing Hansen & Travis 1974).
func TestRayleighOptical_DepthMatchesHansenTravisAt550nm(t *testing.T) {
	tau := rayleighOpticalDepth(550)

	if diff := math.Abs(tau - 0.097); diff > 0.02 {
		t.Errorf("tau_R(550nm) = %v, want ~0.097 (diff %v)", tau, diff)
	}
}
