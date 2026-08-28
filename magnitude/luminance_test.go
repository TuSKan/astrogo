package magnitude_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/unit"
)

// One watt per square metre per steradian of pure 555 nm light is 683 cd/m2.
//
// # Why this is the anchoring case
//
// Because it is the definition of the lumen rather than a property of any
// curve, so it checks the whole projection against something outside this
// package. V(lambda) is 1 at its peak by construction, so a radiance of one
// concentrated there must come out as exactly the luminous efficacy — and it
// does so only if the integral, the efficacy and the units all line up. Any
// one of them wrong moves this number.
//
// The curve here is a narrow triangle centred on 555 nm rather than the real
// CIE tabulation, so the test needs no network. Its integral is its width
// over two, which is what makes the expected value hand-computable.
func TestLuminanceAnchorsOnTheLumenDefinition(t *testing.T) {
	t.Parallel()

	// 1 nm steps across a 10 nm window centred on 555.
	grid, err := unit.NewSpectralGrid(550, 1, 11)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	// A triangle peaking at 1.0 on 555 nm, zero at 550 and 560.
	curve := magnitude.Passband{
		Name:         "test V(lambda)",
		WavelengthNM: []unit.WavelengthNM{550, 555, 560},
		Response:     []float64{0, 1, 0},
		Detector:     magnitude.EnergyIntegrating,
	}

	// A flat radiance of 1 W m^-2 sr^-1 nm^-1 across the window. The weighted
	// integral is then the area of the triangle, 10/2 = 5 nm.
	spectrum := make([]float64, grid.Len())
	for i := range spectrum {
		spectrum[i] = 1
	}

	got, err := magnitude.Luminance(spectrum, grid, curve, 683, 0.9)
	if err != nil {
		t.Fatalf("Luminance: %v", err)
	}

	want := 683.0 * 5

	if rel := math.Abs(float64(got)-want) / want; rel > 1e-9 {
		t.Errorf("luminance is %g cd/m2, want %g (relative %.3g)", float64(got), want, rel)
	}
}

// Luminance is an integral, so a wider curve over the same radiance gives a
// larger number.
//
// This is the property that separates it from every other projection in this
// package, all of which divide by the band's normalisation and would return
// the same value for both curves here. Getting it wrong produces a number in
// the right units, of plausible size, wrong by the width of whatever curve
// was passed in.
func TestLuminanceIsATotalNotAMean(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(500, 1, 101)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	spectrum := make([]float64, grid.Len())
	for i := range spectrum {
		spectrum[i] = 1
	}

	narrow := magnitude.Passband{
		Name:         "narrow",
		WavelengthNM: []unit.WavelengthNM{545, 550, 555},
		Response:     []float64{0, 1, 0},
		Detector:     magnitude.EnergyIntegrating,
	}

	wide := magnitude.Passband{
		Name:         "wide",
		WavelengthNM: []unit.WavelengthNM{530, 550, 570},
		Response:     []float64{0, 1, 0},
		Detector:     magnitude.EnergyIntegrating,
	}

	narrowL, err := magnitude.Luminance(spectrum, grid, narrow, 683, 0.9)
	if err != nil {
		t.Fatalf("Luminance narrow: %v", err)
	}

	wideL, err := magnitude.Luminance(spectrum, grid, wide, 683, 0.9)
	if err != nil {
		t.Fatalf("Luminance wide: %v", err)
	}

	// Four times the width, four times the area, same peak.
	if ratio := float64(wideL / narrowL); math.Abs(ratio-4) > 1e-9 {
		t.Errorf("the wide curve gives %.6f× the narrow one, want 4 — a mean would give 1",
			ratio)
	}
}

// Luminance scales with the radiance and with the efficacy, both linearly.
func TestLuminanceIsLinear(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(550, 1, 11)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	curve := magnitude.Passband{
		Name:         "test",
		WavelengthNM: []unit.WavelengthNM{550, 555, 560},
		Response:     []float64{0, 1, 0},
		Detector:     magnitude.EnergyIntegrating,
	}

	unitSpectrum := make([]float64, grid.Len())
	doubled := make([]float64, grid.Len())

	for i := range unitSpectrum {
		unitSpectrum[i] = 1
		doubled[i] = 2
	}

	base, err := magnitude.Luminance(unitSpectrum, grid, curve, 683, 0.9)
	if err != nil {
		t.Fatalf("Luminance: %v", err)
	}

	twiceLight, err := magnitude.Luminance(doubled, grid, curve, 683, 0.9)
	if err != nil {
		t.Fatalf("Luminance: %v", err)
	}

	twiceEff, err := magnitude.Luminance(unitSpectrum, grid, curve, 1366, 0.9)
	if err != nil {
		t.Fatalf("Luminance: %v", err)
	}

	for _, c := range []struct {
		name string
		got  unit.LuminanceCdM2
	}{
		{"twice the radiance", twiceLight},
		{"twice the efficacy", twiceEff},
	} {
		if ratio := float64(c.got / base); math.Abs(ratio-2) > 1e-12 {
			t.Errorf("%s gave %.9f×, want exactly 2", c.name, ratio)
		}
	}
}

// An efficacy that cannot scale anything is refused rather than silently
// producing a dark sky.
func TestLuminanceRefusesAnUnusableEfficacy(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(550, 1, 11)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	curve := magnitude.Passband{
		Name:         "test",
		WavelengthNM: []unit.WavelengthNM{550, 555, 560},
		Response:     []float64{0, 1, 0},
		Detector:     magnitude.EnergyIntegrating,
	}

	spectrum := make([]float64, grid.Len())

	for _, efficacy := range []float64{0, -683, math.NaN(), math.Inf(1)} {
		_, err := magnitude.Luminance(spectrum, grid, curve, efficacy, 0.9)
		if !errors.Is(err, magnitude.ErrLuminousEfficacy) {
			t.Errorf("an efficacy of %g was accepted: %v", efficacy, err)
		}
	}
}
