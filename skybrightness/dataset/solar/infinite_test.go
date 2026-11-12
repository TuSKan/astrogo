package solar_test

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/solar"
)

// A row with an infinite wavelength or flux is dropped, as one with a NaN is.
// Kept, an infinite flux read as +Inf across a hundred nanometers of At, and
// an infinite wavelength held the last real value out to infinity rather than
// reading zero past the data (#1038).
func TestParseDropsAnInfiniteRow(t *testing.T) {
	t.Parallel()

	inf := math.Inf(1)

	for _, c := range []struct {
		name       string
		wavelength []float64
		flux       []float64
	}{
		{"flux +Inf", []float64{4000, 5000, 6000}, []float64{2.5, inf, 3.0}},
		{"flux -Inf", []float64{4000, 5000, 6000}, []float64{2.5, -inf, 3.0}},
		{"wavelength +Inf", []float64{4000, 6000, inf}, []float64{2.5, 3.0, 2.6}},
	} {
		spec, err := solar.Parse(bytes.NewReader(calspecLike(t, c.wavelength, c.flux)))
		if err != nil {
			t.Errorf("%s: Parse: %v", c.name, err)

			continue
		}

		if len(spec.WavelengthNM) != 2 || float64(spec.WavelengthNM[0]) != 400 || float64(spec.WavelengthNM[1]) != 600 {
			t.Errorf("%s: kept %v, want 400 and 600 nm", c.name, spec.WavelengthNM)
		}

		if got := spec.At(800); got != 0 {
			t.Errorf("%s: At(800 nm) = %v, want 0 past the data", c.name, got)
		}
	}
}

// A spectrum whose wavelengths do not ascend is refused. At bisects, so one in
// descending order read zero at every wavelength: zero moonlight (#1038).
func TestParseRefusesWavelengthsOutOfOrder(t *testing.T) {
	t.Parallel()

	raw := calspecLike(t, []float64{6000, 5000, 4000}, []float64{3.0, 2.6, 2.5})

	if spec, err := solar.Parse(bytes.NewReader(raw)); !errors.Is(err, solar.ErrNoSpectrum) {
		t.Errorf("descending: %v and err %v, want ErrNoSpectrum", spec, err)
	}
}
