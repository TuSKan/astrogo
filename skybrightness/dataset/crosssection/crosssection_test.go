package crosssection_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/crosssection"
	"github.com/TuSKan/astrogo/unit"
)

// The atlas's files carry provenance headers in no fixed form, so anything
// that does not parse as two numbers is skipped rather than rejected.
// Refusing them would mean hand-editing every download.
func TestParseSkipsHeaders(t *testing.T) {
	t.Parallel()

	file := `# O3 absorption cross section
Serdyuchenko et al. 2014, 293 K
lambda(nm)   sigma(cm2)

400   1.2e-23
450   2.5e-23
500   3.1e-21
550   4.8e-21
`

	cs, err := crosssection.Parse(strings.NewReader(file), "O3", crosssection.Nanometre)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(cs.WavelengthNM) != 4 {
		t.Fatalf("got %d rows, want 4", len(cs.WavelengthNM))
	}

	if cs.Species != "O3" {
		t.Errorf("Species = %q, want O3", cs.Species)
	}

	if cs.WavelengthNM[0] != 400 || cs.SigmaCM2[3] != 4.8e-21 {
		t.Errorf("first wavelength %v, last sigma %v", cs.WavelengthNM[0], cs.SigmaCM2[3])
	}
}

// Wavenumbers arrive in descending wavelength order, so the parser has to
// sort — atmosphere.CrossSection requires strictly increasing wavelength and
// would otherwise reject a perfectly good file.
func TestParseWavenumbersAreReordered(t *testing.T) {
	t.Parallel()

	// 25000, 20000 and 16666.67 cm^-1 are 400, 500 and 600 nm.
	file := "25000 1e-23\n20000 2e-23\n16666.6667 3e-23\n"

	cs, err := crosssection.Parse(strings.NewReader(file), "O3", crosssection.Wavenumber)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want := []float64{400, 500, 600}
	for i, w := range want {
		if math.Abs(float64(cs.WavelengthNM[i])-w) > 0.01 {
			t.Errorf("row %d is %v nm, want %v", i, cs.WavelengthNM[i], w)
		}
	}

	// And the cross sections must travel with their wavelengths, not stay
	// in file order.
	if cs.SigmaCM2[0] != 1e-23 || cs.SigmaCM2[2] != 3e-23 {
		t.Errorf("sigmas did not follow the sort: %v", cs.SigmaCM2)
	}
}

// Angstrom is a factor of ten from nanometres, and getting it wrong shifts
// every absorption feature by that factor while still producing a
// plausible-looking curve. That is why the unit is a parameter and not
// sniffed from the file.
func TestParseAngstrom(t *testing.T) {
	t.Parallel()

	cs, err := crosssection.Parse(strings.NewReader("4000 1e-23\n5000 2e-23\n"), "O3", crosssection.Angstrom)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if cs.WavelengthNM[0] != 400 || cs.WavelengthNM[1] != 500 {
		t.Errorf("angstrom conversion gave %v", cs.WavelengthNM)
	}
}

// A negative cross section is noise at the detection floor, not negative
// absorption. Clamping keeps the optical depth physical; passing it through
// would make transmission exceed one.
func TestParseClampsNegativeSigma(t *testing.T) {
	t.Parallel()

	cs, err := crosssection.Parse(strings.NewReader("400 -1e-25\n500 2e-23\n"), "O3", crosssection.Nanometre)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if cs.SigmaCM2[0] != 0 {
		t.Errorf("negative sigma became %v, want 0", cs.SigmaCM2[0])
	}
}

// Duplicated wavelengths must be collapsed, since CrossSection requires
// strictly increasing and files sometimes repeat a grid point.
func TestParseDropsDuplicates(t *testing.T) {
	t.Parallel()

	cs, err := crosssection.Parse(strings.NewReader("400 1e-23\n400 1.1e-23\n500 2e-23\n"), "O3", crosssection.Nanometre)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(cs.WavelengthNM) != 2 {
		t.Errorf("got %d rows, want 2 after collapsing the duplicate", len(cs.WavelengthNM))
	}
}

// The parsed result must feed atmosphere.CrossSection's own arithmetic, which
// is the point of the package.
func TestParsedCrossSectionComputesOpticalDepth(t *testing.T) {
	t.Parallel()

	cs, err := crosssection.Parse(strings.NewReader("400 1e-21\n500 2e-21\n600 3e-21\n"), "O3", crosssection.Nanometre)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	grid, err := unit.NewSpectralGrid(400, 100, 3)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	dst := make([]float64, grid.Len())

	// A round column, so the optical depth is sigma times the column.
	const column = 1e19

	if err := cs.OpticalDepth(dst, grid, column); err != nil {
		t.Fatalf("OpticalDepth: %v", err)
	}

	for i, want := range []float64{1e-21 * column, 2e-21 * column, 3e-21 * column} {
		if rel := math.Abs(dst[i]-want) / want; rel > 1e-12 {
			t.Errorf("band %d = %v, want %v", i, dst[i], want)
		}
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, file string }{
		{"empty", ""},
		{"headers only", "# nothing here\nlambda sigma\n"},
		{"one row", "400 1e-23\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := crosssection.Parse(strings.NewReader(tc.file), "O3", crosssection.Nanometre); !errors.Is(err, crosssection.ErrFormat) {
				t.Errorf("err = %v, want ErrFormat", err)
			}
		})
	}

	if _, err := crosssection.Parse(strings.NewReader("400 1e-23\n500 2e-23\n"), "O3", "furlongs"); !errors.Is(err, crosssection.ErrUnit) {
		t.Errorf("unknown unit: err = %v, want ErrUnit", err)
	}
}
