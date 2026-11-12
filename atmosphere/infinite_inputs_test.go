package atmosphere_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// An infinite column is refused. It gave an infinite optical depth inside the
// cross section's range and NaN, zero times infinity, outside it, with no
// error (#1038).
func TestOpticalDepthRefusesAnInfiniteColumn(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(400, 10, 31)
	if err != nil {
		t.Fatal(err)
	}

	xs := atmosphere.CrossSection{
		Species:      "O3",
		WavelengthNM: []unit.WavelengthNM{500, 550, 600},
		Sigma: []unit.Area{
			unit.SquareCentimeters(1e-21), unit.SquareCentimeters(2e-21), unit.SquareCentimeters(1e-21),
		},
	}
	dst := make([]float64, grid.Len())

	if err := xs.OpticalDepth(dst, grid, 1e19); err != nil {
		t.Fatalf("OpticalDepth with a finite column: %v", err)
	}

	for _, column := range []float64{math.Inf(1), math.Inf(-1)} {
		if err := xs.OpticalDepth(dst, grid, column); !errors.Is(err, atmosphere.ErrColumnAmount) {
			t.Errorf("column %v: err %v, want ErrColumnAmount", column, err)
		}
	}
}

// An infinite Rayleigh depth is refused, not answered with an infinite
// factor (#1038).
func TestMultipleScatteringFactorRefusesAnInfiniteDepth(t *testing.T) {
	t.Parallel()

	if f, err := atmosphere.MultipleScatteringFactor(0.1); err != nil || !(f > 1) {
		t.Fatalf("finite depth: %v, %v", f, err)
	}

	if f, err := atmosphere.MultipleScatteringFactor(unit.OpticalDepth(math.Inf(1))); !errors.Is(err, atmosphere.ErrOpticalDepth) {
		t.Errorf("+Inf: %v and err %v, want ErrOpticalDepth", f, err)
	}
}

// Every input NaN or an infinity passed is refused. `< 1` and `<= 0 || > 1`
// are false for NaN, so a NaN airmass or kappa came back as a NaN depth, an
// infinite airmass as an infinite one, and an infinite height as zero, as if
// there were no atmosphere, all with no error (#1038).
func TestExtendedSourceOpticalDepthRefusesNonFiniteInputs(t *testing.T) {
	t.Parallel()

	const (
		rayleigh = unit.OpticalDepth(0.1)
		aerosol  = unit.OpticalDepth(0.05)
	)

	if d, err := atmosphere.ExtendedSourceOpticalDepth(rayleigh, aerosol, 1.2, 1.2, 0, 0.5); err != nil || !(d > 0) {
		t.Fatalf("finite inputs: %v, %v", d, err)
	}

	nan, inf := math.NaN(), math.Inf(1)

	for _, c := range []struct {
		name               string
		molecular, aerosol float64
		height, kappa      float64
		want               error
	}{
		{"molecular airmass NaN", nan, 1.2, 0, 0.5, atmosphere.ErrAirmassRange},
		{"molecular airmass +Inf", inf, 1.2, 0, 0.5, atmosphere.ErrAirmassRange},
		{"aerosol airmass NaN", 1.2, nan, 0, 0.5, atmosphere.ErrAirmassRange},
		{"aerosol airmass +Inf", 1.2, inf, 0, 0.5, atmosphere.ErrAirmassRange},
		{"observer height +Inf", 1.2, 1.2, inf, 0.5, atmosphere.ErrOpticalDepth},
		{"kappa NaN", 1.2, 1.2, 0, nan, atmosphere.ErrOpticalDepth},
	} {
		d, err := atmosphere.ExtendedSourceOpticalDepth(rayleigh, aerosol, c.molecular, c.aerosol, unit.Meters(c.height), c.kappa)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: %v and err %v, want %v", c.name, d, err, c.want)
		}
	}
}
