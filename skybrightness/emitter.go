package skybrightness

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// ErrNoEmitterLocation is returned by an emitter with no ground location.
var ErrNoEmitterLocation = errors.New("skybrightness: emitter needs a ground location")

// GroundEmitter is one artificial light source: a luminaire, a city, a
// satellite-derived raster cell, or an aggregated region.
//
// A source must carry enough information to propagate light *physically*,
// which is more than a brightness. Satellite radiance alone cannot
// determine the spectral power distribution, the upward emission function,
// the shielding, or the ground reflection — several different real
// installations produce the same satellite pixel. Those quantities
// therefore come from measurement, an explicit model, or a stated
// assumption, and an emitter reports which through Quality.
//
// This is an interface because it is a genuine data-provider boundary:
// an inventory of luminaires, a VIIRS raster and a municipal aggregate are
// structurally different sources of the same physical quantity.
type GroundEmitter interface {
	// Location is where the source sits.
	Location() *coord.Geodetic

	// SourceRadiance writes the radiance leaving the source toward the
	// observer, per wavelength, into dst.
	//
	// towardObserver is the bearing from the source to the observer, and
	// elevationAngle the angle above the source's horizon at which the
	// light leaves toward the observer's sky element. Together they select
	// the value of the upward emission function that applies — which is
	// why a source is not a single number.
	SourceRadiance(dst []float64, grid unit.SpectralGrid, towardObserver, elevationAngle angle.Angle) error

	// Quality reports whether the spectrum and emission function are
	// measured or assumed, so the assumption travels with the result.
	Quality() Flag
}

// UniformEmitter is a source with a fixed spectral power distribution and
// a parameterised upward emission function.
//
// It is the simplest emitter that is still physically honest: the spectrum
// and the emission shape are both explicit and both flagged as assumed
// unless the caller says otherwise, rather than being implied by a single
// brightness figure.
type UniformEmitter struct {
	// At is the source location.
	At *coord.Geodetic

	// Name identifies the source.
	Name string

	// WavelengthNM and Radiance tabulate the spectral radiance leaving the
	// source into the upper hemisphere, W m^-2 sr^-1 nm^-1.
	WavelengthNM []unit.WavelengthNM
	Radiance     []float64

	// Emission shapes how that radiance varies with the angle above the
	// source's horizon.
	Emission UpwardEmission

	// Flags records whether the spectrum and emission function are
	// measured or assumed.
	Flags Flag
}

// UpwardEmission describes how much light a source sends at a given angle
// above its own horizon.
//
// Real installations differ enormously here — a fully shielded luminaire
// emits almost nothing above the horizontal, while an unshielded globe
// emits comparably in every upward direction — and this term matters as
// much as total output for how far skyglow travels. A model that ignores
// it is not predicting propagation.
type UpwardEmission struct {
	// Cosine weights emission as cos^n of the zenith angle at the source:
	// n = 1 is Lambertian, larger n is more sharply upward, and n = 0 is
	// uniform over the hemisphere.
	Cosine float64

	// HorizontalFraction is the fraction of output emitted near the
	// horizontal rather than following the cosine term, in [0,1]. This is
	// the part that escapes to great distances, since it travels a long
	// slant path through the lower atmosphere.
	HorizontalFraction float64
}

// Weight returns the relative emission at elevation above the source's
// horizon, normalised so that a Lambertian source with no horizontal
// component has unit weight at the zenith.
func (u UpwardEmission) Weight(elevation angle.Angle) float64 {
	sin := elevation.Sin()
	if sin < 0 {
		return 0 // below the source's horizon: nothing escapes upward
	}

	n := u.Cosine
	if n < 0 {
		n = 0
	}

	// The cosine of the zenith angle at the source is the sine of the
	// elevation above its horizon.
	directional := math.Pow(sin, n)

	f := u.HorizontalFraction
	if f < 0 {
		f = 0
	} else if f > 1 {
		f = 1
	}

	return (1-f)*directional + f
}

// Location implements GroundEmitter.
func (u *UniformEmitter) Location() *coord.Geodetic { return u.At }

// Quality implements GroundEmitter.
func (u *UniformEmitter) Quality() Flag { return u.Flags }

// SourceRadiance implements GroundEmitter. The bearing toward the observer
// is unused here because a UniformEmitter is azimuthally symmetric, which
// is the assumption Kocifaj, Bará & Falchi (2022) Eq. 2 itself makes.
func (u *UniformEmitter) SourceRadiance(
	dst []float64,
	grid unit.SpectralGrid,
	_ angle.Angle,
	elevation angle.Angle,
) error {
	if u.At == nil {
		return fmt.Errorf("%w: %q", ErrNoEmitterLocation, u.Name)
	}

	if len(dst) != grid.Len() {
		return fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	if err := grid.Resample(dst, u.WavelengthNM, u.Radiance, 0); err != nil {
		return fmt.Errorf("skybrightness: emitter %q: %w", u.Name, err)
	}

	w := u.Emission.Weight(elevation)
	for i := range dst {
		dst[i] *= w
	}

	return nil
}
