package atmosphere_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// mustGrid builds a five-sample grid, enough to place points inside and
// outside a tabulated cross section.
func mustGrid(t *testing.T, start, step unit.WavelengthNM) unit.SpectralGrid {
	t.Helper()

	g, err := unit.NewSpectralGrid(start, step, 5)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	return g
}

// The Dobson Unit is defined as 0.01 mm of the pure gas at STP. Its
// accepted value is 2.687e16 molecules per square centimetre; deriving it
// from the SI-exact Boltzmann constant must reproduce that, which is what
// makes it a derivation rather than a hardcoded number.
func TestDobsonUnitDerivation(t *testing.T) {
	t.Parallel()

	got := atmosphere.DobsonUnitMoleculesPerCM2()

	const want = 2.6867e16

	if rel := math.Abs(got-want) / want; rel > 1e-4 {
		t.Errorf("1 DU = %.5e molecules/cm^2, want %.5e (rel %g)", got, want, rel)
	}
}

// A flat cross section over a column gives a flat optical depth equal to
// sigma*N — Beer-Lambert with nothing else in the way.
func TestCrossSectionOpticalDepth(t *testing.T) {
	t.Parallel()

	const sigma = 5e-21 // cm^2/molecule, an ozone-Chappuis-scale value

	cs := atmosphere.CrossSection{
		Species:      "test",
		WavelengthNM: []unit.WavelengthNM{400, 700},
		SigmaCM2:     []float64{sigma, sigma},
		TemperatureK: 250,
		Reference:    "synthetic test fixture, not measured data",
	}

	g := mustGrid(t, 450, 50) // 450..650, inside the table

	dst := make([]float64, g.Len())

	const column = 1e19 // molecules/cm^2

	if err := cs.OpticalDepth(dst, g, column); err != nil {
		t.Fatalf("OpticalDepth: %v", err)
	}

	want := sigma * column
	for i, got := range dst {
		if math.Abs(got-want)/want > 1e-12 {
			t.Errorf("tau[%d] (%v nm) = %v, want %v", i, g.At(i), got, want)
		}
	}
}

// Outside the measured range the optical depth is zero: no measurement is
// not the same as a claim of transparency-by-extrapolation, but it is the
// conservative reading and must be explicit.
func TestCrossSectionZeroOutsideTable(t *testing.T) {
	t.Parallel()

	cs := atmosphere.CrossSection{
		Species:      "test",
		WavelengthNM: []unit.WavelengthNM{500, 600},
		SigmaCM2:     []float64{1e-20, 1e-20},
		Reference:    "synthetic test fixture",
	}

	g := mustGrid(t, 400, 100) // 400..800

	dst := make([]float64, g.Len())
	if err := cs.OpticalDepth(dst, g, 1e19); err != nil {
		t.Fatalf("OpticalDepth: %v", err)
	}

	for i, got := range dst {
		lambda := g.At(i)
		inside := lambda >= 500 && lambda <= 600

		if inside && got == 0 {
			t.Errorf("tau at %v nm is zero inside the tabulated range", lambda)
		}

		if !inside && got != 0 {
			t.Errorf("tau at %v nm = %v, want 0 outside the tabulated range", lambda, got)
		}
	}
}

// An ozone column in Dobson Units must agree with the same column
// expressed in molecules per square centimetre.
func TestOzoneOpticalDepthMatchesExplicitColumn(t *testing.T) {
	t.Parallel()

	cs := atmosphere.CrossSection{
		Species:      "O3",
		WavelengthNM: []unit.WavelengthNM{400, 700},
		SigmaCM2:     []float64{5e-21, 5e-21},
		Reference:    "synthetic test fixture, not Serdyuchenko et al.",
	}

	g := mustGrid(t, 450, 50)

	const columnDU = unit.OzoneColumnDU(300) // a typical mid-latitude column

	viaDU := make([]float64, g.Len())
	if err := cs.OzoneOpticalDepth(viaDU, g, columnDU); err != nil {
		t.Fatalf("OzoneOpticalDepth: %v", err)
	}

	explicit := make([]float64, g.Len())

	err := cs.OpticalDepth(explicit, g, float64(columnDU)*atmosphere.DobsonUnitMoleculesPerCM2())
	if err != nil {
		t.Fatalf("OpticalDepth: %v", err)
	}

	for i := range viaDU {
		if math.Abs(viaDU[i]-explicit[i]) > 1e-15 {
			t.Errorf("sample %d: DU path %v, explicit path %v", i, viaDU[i], explicit[i])
		}
	}

	// A 300 DU ozone column at a Chappuis-scale cross section removes a
	// few per cent, not a few orders of magnitude. This is a sanity bound
	// on the unit chain, not a physical claim about the synthetic sigma.
	transmission := math.Exp(-viaDU[0])
	if transmission < 0.8 || transmission > 1 {
		t.Errorf("transmission through 300 DU = %v, want a few per cent of absorption", transmission)
	}
}

func TestCrossSectionValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cs   atmosphere.CrossSection
		want error
	}{
		{"empty", atmosphere.CrossSection{Species: "x"}, atmosphere.ErrCrossSectionShape},
		{
			"mismatched",
			atmosphere.CrossSection{Species: "x", WavelengthNM: []unit.WavelengthNM{1, 2}, SigmaCM2: []float64{1}},
			atmosphere.ErrCrossSectionShape,
		},
		{
			"not increasing",
			atmosphere.CrossSection{Species: "x", WavelengthNM: []unit.WavelengthNM{2, 1}, SigmaCM2: []float64{1, 1}},
			atmosphere.ErrCrossSectionShape,
		},
		{
			"negative sigma",
			atmosphere.CrossSection{Species: "x", WavelengthNM: []unit.WavelengthNM{1, 2}, SigmaCM2: []float64{1, -1}},
			atmosphere.ErrCrossSectionValue,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := tc.cs.Validate(); !errors.Is(err, tc.want) {
				t.Errorf("Validate = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCrossSectionRejectsNegativeColumn(t *testing.T) {
	t.Parallel()

	cs := atmosphere.CrossSection{
		Species:      "x",
		WavelengthNM: []unit.WavelengthNM{400, 700},
		SigmaCM2:     []float64{1e-20, 1e-20},
	}

	g := mustGrid(t, 450, 50)

	if err := cs.OpticalDepth(make([]float64, g.Len()), g, -1); !errors.Is(err, atmosphere.ErrColumnAmount) {
		t.Errorf("negative column = %v, want ErrColumnAmount", err)
	}

	if err := cs.OzoneOpticalDepth(make([]float64, g.Len()), g, -1); !errors.Is(err, atmosphere.ErrColumnAmount) {
		t.Errorf("negative DU column = %v, want ErrColumnAmount", err)
	}
}
