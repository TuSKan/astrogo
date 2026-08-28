package skybrightness

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for spectral buffers. Match with errors.Is.
var (
	// ErrNegativeRadiance is returned when a component contributes a
	// negative spectral radiance. Radiance is a photon flux and cannot be
	// negative; a negative value means a bug, not a dark sky.
	ErrNegativeRadiance = errors.New("skybrightness: spectral radiance must be non-negative")

	// ErrNonFiniteRadiance is returned for a NaN or infinite radiance,
	// which would otherwise propagate silently through every later sum and
	// projection.
	ErrNonFiniteRadiance = errors.New("skybrightness: spectral radiance must be finite")
)

// SpectralRadiance is a sky spectrum sampled on a [unit.SpectralGrid], in
// W m^-2 sr^-1 nm^-1.
//
// It is a plain slice rather than a struct carrying its own grid. Components
// accumulate into a caller-owned buffer, so the grid travels once through
// the call rather than being re-validated per element, and a full-sky
// evaluation reuses one allocation across thousands of directions.
type SpectralRadiance []float64

// NewSpectralRadiance returns a zeroed buffer sized for grid.
func NewSpectralRadiance(grid unit.SpectralGrid) SpectralRadiance {
	return make(SpectralRadiance, grid.Len())
}

// Zero resets every sample to zero, keeping the allocation. This is what
// makes a sky-map loop allocation-free across directions.
func (s SpectralRadiance) Zero() {
	for i := range s {
		s[i] = 0
	}
}

// Add accumulates other into s element-wise. Both must be the same length.
func (s SpectralRadiance) Add(other SpectralRadiance) error {
	if len(s) != len(other) {
		return fmt.Errorf("%w: adding %d samples into %d", unit.ErrGridMismatch, len(other), len(s))
	}

	for i := range s {
		s[i] += other[i]
	}

	return nil
}

// Scale multiplies every sample by factor.
func (s SpectralRadiance) Scale(factor float64) {
	for i := range s {
		s[i] *= factor
	}
}

// Validate reports the first sample that is negative or not finite.
//
// This runs at component boundaries rather than being assumed, because a
// negative radiance produced by an interpolation or a subtraction is
// physically impossible and would otherwise reach a magnitude conversion
// as a NaN, where its origin is no longer recoverable.
func (s SpectralRadiance) Validate() error {
	for i, v := range s {
		switch {
		case math.IsNaN(v) || math.IsInf(v, 0):
			return fmt.Errorf("%w: sample %d = %g", ErrNonFiniteRadiance, i, v)
		case v < 0:
			return fmt.Errorf("%w: sample %d = %g", ErrNegativeRadiance, i, v)
		}
	}

	return nil
}

// Clone returns an independent copy.
func (s SpectralRadiance) Clone() SpectralRadiance {
	out := make(SpectralRadiance, len(s))
	copy(out, s)

	return out
}

// Integrate returns the radiance integrated over the whole grid, in
// W m^-2 sr^-1 — the spectrally integrated (bolometric over the grid's
// span) radiance, not a passband quantity. For a passband use
// [magnitude.MeanFluxDensity].
func (s SpectralRadiance) Integrate(grid unit.SpectralGrid) (unit.Radiance, error) {
	v, err := grid.Integrate(s)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: integrate radiance: %w", err)
	}

	return unit.Radiance(v), nil
}

// DefaultOpticalGrid is the spectral axis used when a caller does not
// choose one: 330-1000 nm at 1 nm.
//
// The lower bound is where the atmosphere's ozone Huggins bands make
// ground-level night-sky radiance negligible and where most detectors lose
// response; the upper bound covers the near-infrared airglow OH bands that
// dominate a dark site's background beyond 700 nm. 1 nm resolves the
// airglow line structure well enough for broadband projection while
// keeping a full-sky evaluation tractable; a component needing finer
// sampling around a specific line says so in its own documentation.
func DefaultOpticalGrid() unit.SpectralGrid {
	g, err := unit.NewSpectralGrid(330, 1, 671)
	if err != nil {
		// Unreachable: the arguments are constants satisfying Validate.
		panic("skybrightness: DefaultOpticalGrid is invalid: " + err.Error())
	}

	return g
}

// BlackbodyShape returns a Planck spectral radiance over grid, at temperature
// t in kelvin.
//
// # What it is for
//
// [PresetInputs.StarShape] and [NewIntegratedStarlight] both require a spectral
// shape from the caller, and deliberately: integrated starlight is the summed
// light of stars of every type, no single blackbody is right for it, and a
// package that picked one silently would be choosing the answer's colour on the
// caller's behalf. But every caller needs *some* shape, and until this existed
// each one had to write Planck's law out again — which is how two callers end
// up with two different constants.
//
// A 5500 K Planck function is the conventional stand-in for the integrated
// light of the sky, close to the Sun's effective temperature and to the
// flux-weighted mean of a typical stellar population. It is an approximation
// with a name, which is the most this can offer.
//
// # What it does not affect
//
// The components renormalise a shape so its average across the passband is
// one. So the temperature sets the spectrum's colour — how the band-integrated
// value is distributed across wavelength, and therefore how extinction, which
// is steepest in the blue, redistributes it — and not the band value itself,
// which the star map already fixes.
//
// Returned in W m^-2 sr^-1 nm^-1 for a blackbody of unit emissivity, though
// only the shape survives renormalisation.
func BlackbodyShape(grid unit.SpectralGrid, t float64) (SpectralRadiance, error) {
	if err := grid.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoGrid, err)
	}

	if t <= 0 || math.IsNaN(t) || math.IsInf(t, 0) {
		return nil, fmt.Errorf("%w: temperature %g K", ErrNoGrid, t)
	}

	// Planck's law per unit wavelength,
	//
	//	B(lambda, T) = 2hc^2 / lambda^5 / (exp(hc / (lambda k T)) - 1)
	//
	// with the constants taken from the module's own set rather than written
	// out, and the result per nanometre rather than per metre.
	const (
		metrePerNM  = 1e-9
		perMToPerNM = 1e-9
	)

	var (
		h  = constants.SI2019.PlanckConstant.Value
		c  = constants.SI2019.SpeedOfLight.Value
		kB = constants.SI2019.BoltzmannConstant.Value
	)

	out := NewSpectralRadiance(grid)

	for i := range out {
		lambda := float64(grid.At(i)) * metrePerNM

		exponent := h * c / (lambda * kB * t)
		if exponent > 700 {
			// exp overflows past about 709 and the radiance there is zero to
			// any precision that matters, so the far blue of a cool source is
			// filled in rather than turned into an infinity.
			out[i] = 0

			continue
		}

		out[i] = 2 * h * c * c / (math.Pow(lambda, 5) * math.Expm1(exponent)) * perMToPerNM
	}

	return out, nil
}
