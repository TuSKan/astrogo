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
	// source's horizon. Nil means Lambertian, the zero [UpwardEmission].
	Emission EmissionShape

	// Flags records whether the spectrum and emission function are
	// measured or assumed.
	Flags Flag
}

// EmissionShape is how much light a source sends at a given angle above its
// own horizon, relative to its output at the zenith.
//
// Real installations differ enormously here — a fully shielded luminaire
// emits almost nothing above the horizontal, while an unshielded globe emits
// comparably in every upward direction — and this term matters as much as
// total output for how far skyglow travels. A model that ignores it is not
// predicting propagation.
//
// An interface because the published forms are genuinely different functions
// rather than one function with different constants: [UpwardEmission] is a
// cosine power with a horizontal component, and [GarstangEmission] is
// Garstang's two-population form. A struct holding both would have fields
// that are silently ignored depending on other fields, which is a worse thing
// to hand a caller than a choice of types.
type EmissionShape interface {
	// Weight returns the relative emission at elevation above the source's
	// horizon. Zero or below the horizon returns zero.
	Weight(elevation angle.Angle) float64
}

// UpwardEmission is a cosine-power emission shape with a horizontal
// component, and is the zero value's Lambertian default.
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

// GarstangEmission is Garstang's upward-emission function for a city.
//
// Garstang (1986), PASP 98, 364, as used by Kocifaj (2007) Eq. 27:
//
//	B(Q, q, z0) = 2*Q*(1 - q)*cos(z0) + 0.554*q*z0^4
//
// where z0 is the zenith angle at the source, in radians. It models a city as
// two populations rather than one shape: light that reaches the sky after
// reflecting off the ground, which leaves Lambertian, and light radiated
// directly upward by luminaires, which peaks toward the horizon.
//
// The z0^4 term is why it is not a cosine model. Direct upward emission grows
// with zenith angle and is largest near the horizontal, so it dominates at
// exactly the angles whose light travels furthest — which is what makes the
// split between Q and q matter more for a distant observer than the total
// output does. Shielding a city changes q, and that changes the sky hundreds
// of kilometres away.
//
// This is the emission function the World Atlas, the SkyGlow Simulator and
// most of the light-pollution literature are built on, so it is the shape to
// use when comparing against any of them.
type GarstangEmission struct {
	// ReflectedFraction is Garstang's Q: the fraction of a city's light that
	// reaches the sky after reflecting off the ground, and therefore leaves
	// with a Lambertian distribution.
	ReflectedFraction float64

	// DirectFraction is Garstang's q: the fraction radiated directly into the
	// upward hemisphere by the luminaires themselves. Kocifaj (2007) uses
	// Q = q = 0.15 in its numerical runs.
	DirectFraction float64
}

// Weight implements [EmissionShape].
//
// Normalised by the value at the zenith, so it is a shape rather than an
// absolute output and composes with the emitter's own radiance the same way
// [UpwardEmission] does. At the zenith z0 is zero, the direct term vanishes
// and the value is 2*Q*(1-q); a configuration where that is not positive has
// no zenith emission to normalise against and returns zero.
func (g GarstangEmission) Weight(elevation angle.Angle) float64 {
	sin := elevation.Sin()
	if sin <= 0 {
		return 0 // at or below the source's horizon
	}

	q := clamp01(g.DirectFraction)
	qq := g.ReflectedFraction

	if qq < 0 {
		qq = 0
	}

	// The zenith angle at the source, whose cosine is the sine of the
	// elevation above its horizon.
	z0 := math.Acos(math.Min(1, sin))

	zenith := 2 * qq * (1 - q)
	if zenith <= 0 {
		return 0
	}

	b := 2*qq*(1-q)*sin + 0.554*q*z0*z0*z0*z0

	return b / zenith
}

// clamp01 confines a fraction to [0,1].
func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
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

	// A nil shape is Lambertian, which is the zero UpwardEmission and the
	// least surprising thing an unset field can mean.
	shape := u.Emission
	if shape == nil {
		shape = UpwardEmission{Cosine: 1}
	}

	w := shape.Weight(elevation)
	for i := range dst {
		dst[i] *= w
	}

	return nil
}
