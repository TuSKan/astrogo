package solar_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/solar"
	"github.com/TuSKan/astrogo/unit"
)

// A hand-built spectrum, so the interpolation and range behaviour can be
// checked without a network fetch or a FITS fixture.
func fixture() *solar.Spectrum {
	return &solar.Spectrum{
		WavelengthNM: []unit.WavelengthNM{400, 500, 600, 700},
		Irradiance:   []float64{1.0, 2.0, 1.5, 1.2},
	}
}

// Tabulated points come back exactly, and midpoints interpolate linearly.
func TestSpectrumAt(t *testing.T) {
	t.Parallel()

	s := fixture()

	for i, lambda := range s.WavelengthNM {
		if got := s.At(lambda); got != s.Irradiance[i] {
			t.Errorf("At(%v) = %v, want the tabulated %v", lambda, got, s.Irradiance[i])
		}
	}

	if got := s.At(450); math.Abs(got-1.5) > 1e-12 {
		t.Errorf("At(450) = %v, want 1.5 by linear interpolation", got)
	}

	if got := s.At(550); math.Abs(got-1.75) > 1e-12 {
		t.Errorf("At(550) = %v, want 1.75", got)
	}
}

// Outside the tabulated range the answer is zero, not an extrapolation.
// A solar spectrum extrapolated past its endpoints is how a model quietly
// invents flux in a band the reference never covered.
func TestSpectrumOutsideRangeIsZero(t *testing.T) {
	t.Parallel()

	s := fixture()

	for _, lambda := range []unit.WavelengthNM{100, 399.9, 700.1, 3000} {
		if got := s.At(lambda); got != 0 {
			t.Errorf("At(%v) = %v outside the tabulated range, want 0", lambda, got)
		}
	}
}

func TestSpectrumResample(t *testing.T) {
	t.Parallel()

	s := fixture()

	at := []unit.WavelengthNM{400, 450, 700, 900}
	dst := make([]float64, len(at))

	if err := s.Resample(dst, at); err != nil {
		t.Fatalf("Resample: %v", err)
	}

	want := []float64{1.0, 1.5, 1.2, 0}
	for i := range want {
		if math.Abs(dst[i]-want[i]) > 1e-12 {
			t.Errorf("at %v nm: got %v, want %v", at[i], dst[i], want[i])
		}
	}

	if err := s.Resample(make([]float64, 2), at); !errors.Is(err, solar.ErrGrid) {
		t.Errorf("mismatched lengths: err = %v, want ErrGrid", err)
	}
}

// The binary search must not fall over on a degenerate table.
func TestSpectrumEdgeCases(t *testing.T) {
	t.Parallel()

	empty := &solar.Spectrum{}
	if got := empty.At(500); got != 0 {
		t.Errorf("At on an empty spectrum = %v, want 0", got)
	}

	flat := &solar.Spectrum{
		WavelengthNM: []unit.WavelengthNM{500, 500},
		Irradiance:   []float64{3, 4},
	}

	if got := flat.At(500); got != 3 && got != 4 {
		t.Errorf("At on a zero-width interval = %v, want one of the tabulated values", got)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	t.Parallel()

	if _, err := solar.Parse(readerOf("not a FITS file at all")); err == nil {
		t.Error("Parse accepted non-FITS bytes")
	}
}

type stringReader struct {
	s string
	i int
}

func readerOf(s string) *stringReader { return &stringReader{s: s} }

func (r *stringReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, errEOF
	}

	n := copy(p, r.s[r.i:])
	r.i += n

	return n, nil
}

var errEOF = errors.New("EOF")
