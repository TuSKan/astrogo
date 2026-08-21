package unit

import (
	"errors"
	"fmt"
	"math"
)

// Sentinel errors for spectral grid construction and use. Match with
// errors.Is.
var (
	// ErrGridStep is returned when a grid's step is not positive and finite.
	ErrGridStep = errors.New("unit: spectral grid step must be positive and finite")

	// ErrGridStart is returned when a grid's start wavelength is not
	// positive and finite.
	ErrGridStart = errors.New("unit: spectral grid start must be positive and finite")

	// ErrGridLength is returned when a grid has fewer than two samples.
	// A single sample has no interval and cannot be integrated over.
	ErrGridLength = errors.New("unit: spectral grid needs at least 2 samples")

	// ErrSourceNotIncreasing is returned when a curve handed to Resample is
	// not sampled in strictly increasing wavelength order.
	ErrSourceNotIncreasing = errors.New("unit: source wavelengths must be strictly increasing")

	// ErrGridMismatch is returned when an operation receives a value slice
	// whose length does not match the grid it is supposed to lie on.
	ErrGridMismatch = errors.New("unit: value count does not match spectral grid")
)

// SpectralGrid is a uniform wavelength axis: N samples starting at StartNM,
// spaced StepNM apart. It is the shared spectral representation for every
// astrogo package that works per-wavelength — sky radiance, passband
// response, instrument throughput — so a spectrum, a filter curve and a
// detector QE curve can be combined without one of them silently being on
// a different axis.
//
// Uniform spacing is deliberate. It makes integration a fixed-weight sum
// with no per-sample interval lookup, makes grid equality a three-field
// comparison, and makes a resample a pure index computation. Datasets that
// arrive on a non-uniform axis are resampled onto a grid at their provider
// boundary, where the interpolation choice can be documented, rather than
// silently inside a numeric kernel.
type SpectralGrid struct {
	// StartNM is the wavelength of sample 0.
	StartNM WavelengthNM
	// StepNM is the spacing between adjacent samples.
	StepNM WavelengthNM
	// N is the number of samples.
	N int
}

// NewSpectralGrid returns a validated grid.
func NewSpectralGrid(startNM, stepNM WavelengthNM, n int) (SpectralGrid, error) {
	g := SpectralGrid{StartNM: startNM, StepNM: stepNM, N: n}

	if err := g.Validate(); err != nil {
		return SpectralGrid{}, err
	}

	return g, nil
}

// Validate reports whether the grid is usable.
func (g SpectralGrid) Validate() error {
	switch {
	case !isPositiveFinite(float64(g.StartNM)):
		return fmt.Errorf("%w: got %g", ErrGridStart, float64(g.StartNM))
	case !isPositiveFinite(float64(g.StepNM)):
		return fmt.Errorf("%w: got %g", ErrGridStep, float64(g.StepNM))
	case g.N < 2:
		return fmt.Errorf("%w: got %d", ErrGridLength, g.N)
	default:
		return nil
	}
}

// Len reports the number of samples.
func (g SpectralGrid) Len() int { return g.N }

// At returns the wavelength of sample i. It does not bounds-check: callers
// iterating 0..Len()-1 are already in range, and an explicit check here
// would cost more than the loop body in the hot path.
func (g SpectralGrid) At(i int) WavelengthNM {
	return g.StartNM + WavelengthNM(i)*g.StepNM
}

// EndNM returns the wavelength of the last sample.
func (g SpectralGrid) EndNM() WavelengthNM { return g.At(g.N - 1) }

// Contains reports whether lambda lies within the grid's closed span.
func (g SpectralGrid) Contains(lambda WavelengthNM) bool {
	return lambda >= g.StartNM && lambda <= g.EndNM()
}

// Equal reports whether two grids describe the same wavelength axis.
func (g SpectralGrid) Equal(o SpectralGrid) bool {
	return g.StartNM == o.StartNM && g.StepNM == o.StepNM && g.N == o.N
}

// String renders the grid as its span and spacing.
func (g SpectralGrid) String() string {
	return fmt.Sprintf("%g-%gnm/%gnm(%d)", float64(g.StartNM), float64(g.EndNM()), float64(g.StepNM), g.N)
}

// Wavelengths returns a newly allocated slice of every sample wavelength.
// For hot paths prefer At in a loop; this exists for provider and test code.
func (g SpectralGrid) Wavelengths() []WavelengthNM {
	out := make([]WavelengthNM, g.N)
	for i := range out {
		out[i] = g.At(i)
	}

	return out
}

// Integrate returns the trapezoidal integral of values over the grid, in
// the values' own unit multiplied by nanometres. It returns ErrGridMismatch
// if len(values) != g.N.
//
// Trapezoidal rather than Simpson: the integrands here are products of
// measured response curves and modelled spectra, both of which carry
// sampling error far larger than the quadrature difference, and Simpson
// would additionally require an odd sample count that grid users have no
// reason to guarantee.
func (g SpectralGrid) Integrate(values []float64) (float64, error) {
	if len(values) != g.N {
		return 0, fmt.Errorf("%w: %d values, grid has %d", ErrGridMismatch, len(values), g.N)
	}

	if err := g.Validate(); err != nil {
		return 0, err
	}

	// Interior samples carry a full step; the two endpoints carry half.
	sum := 0.5 * (values[0] + values[g.N-1])
	for _, v := range values[1 : g.N-1] {
		sum += v
	}

	return sum * float64(g.StepNM), nil
}

// Resample linearly interpolates a curve sampled at src wavelengths onto
// this grid, writing g.N values into dst. Points outside the source range
// are set to fill, which callers use to declare whether an out-of-range
// response is zero (a filter with no transmission there) or something else.
//
// src must be strictly increasing, and Resample returns
// ErrSourceNotIncreasing rather than interpolating if it is not. The
// interpolation walks a cursor forward on the assumption of that order, so
// out-of-order input does not fail loudly: a curve tabulated from the red end
// down resamples to all zeros, and a single transposed pair in an otherwise
// sorted table resamples to a curve that is positive, smooth, plausible and
// wrong by a factor of five at the peak. A throughput is not the kind of
// quantity where a wrong answer announces itself downstream, so the order is
// checked here, at the one point that can still tell.
//
// Linear interpolation is chosen because response curves are tabulated densely
// relative to their own structure; a spline would introduce ringing and can
// produce negative response between positive samples, which is unphysical for
// a throughput.
func (g SpectralGrid) Resample(dst []float64, src []WavelengthNM, values []float64, fill float64) error {
	switch {
	case len(dst) != g.N:
		return fmt.Errorf("%w: %d destination slots, grid has %d", ErrGridMismatch, len(dst), g.N)
	case len(src) != len(values):
		return fmt.Errorf("%w: %d source wavelengths, %d values", ErrGridMismatch, len(src), len(values))
	case len(src) == 0:
		for i := range dst {
			dst[i] = fill
		}

		return nil
	}

	// One pass, the same order as the resample itself.
	for i := 1; i < len(src); i++ {
		if src[i] <= src[i-1] {
			return fmt.Errorf("%w: src[%d] = %g does not exceed src[%d] = %g",
				ErrSourceNotIncreasing, i, float64(src[i]), i-1, float64(src[i-1]))
		}
	}

	j := 0

	for i := range dst {
		lambda := g.At(i)

		switch {
		case lambda < src[0] || lambda > src[len(src)-1]:
			dst[i] = fill

			continue
		case len(src) == 1:
			dst[i] = values[0]

			continue
		}

		// Grid wavelengths increase monotonically, so the source cursor
		// only ever moves forward: the whole resample is O(N+M), not
		// O(N log M).
		for j+2 < len(src) && src[j+1] < lambda {
			j++
		}

		span := float64(src[j+1] - src[j])
		if span <= 0 {
			dst[i] = values[j]

			continue
		}

		frac := float64(lambda-src[j]) / span
		dst[i] = values[j] + frac*(values[j+1]-values[j])
	}

	return nil
}

// isPositiveFinite reports whether v is a positive, finite float.
func isPositiveFinite(v float64) bool {
	return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}
