package optics

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/unit"
)

// positiveFinite reports whether v is a positive, finite number — the
// shared guard behind every constructor/option validation in this
// package.
func positiveFinite(v float64) bool {
	return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}

// ── Telescope ─────────────────────────────────────────────────────────────────

// Telescope describes an optical system's aperture and focal length —
// enough to derive every other optical quantity in this package
// (magnification, field of view, exit pupil, resolution limits) for a
// given Eyepiece or Sensor. Fields are unexported and only reachable
// through the validating constructor NewTelescope; a zero-value
// Telescope{} would silently divide by zero.
type Telescope struct {
	aperture    unit.Length
	focalLength unit.Length
}

// NewTelescope constructs a Telescope from its aperture and focal length.
// Returns ErrNonPositiveDimension if either is not a positive, finite number.
//
// Equipment is specified in millimeters, so both arguments are almost always
// [unit.Millimeters](x). A bare number would be meters, which no manufacturer
// or catalog quotes.
func NewTelescope(aperture, focalLength unit.Length) (Telescope, error) {
	if !positiveFinite(aperture.Meters()) || !positiveFinite(focalLength.Meters()) {
		return Telescope{}, fmt.Errorf("optics: telescope aperture=%v focalLength=%v: %w",
			aperture, focalLength, ErrNonPositiveDimension)
	}

	return Telescope{aperture: aperture, focalLength: focalLength}, nil
}

// Aperture returns the telescope's aperture.
func (t Telescope) Aperture() unit.Length { return t.aperture }

// FocalLength returns the telescope's focal length.
func (t Telescope) FocalLength() unit.Length { return t.focalLength }

// FocalRatio returns the telescope's focal ratio (f-number): focal length
// divided by aperture — e.g. 10 for an "f/10" telescope.
//
// A ratio of two lengths is dimensionless, so it is taken on the stored values
// rather than through an accessor: converting both to millimeters first would
// divide and then multiply by the same scale factor.
func (t Telescope) FocalRatio() float64 { return float64(t.focalLength) / float64(t.aperture) }

// WithBarlow returns a new Telescope with its focal length scaled by
// factor — a Barlow lens (factor > 1) increases the effective focal
// length; a focal reducer (0 < factor < 1) decreases it. Aperture is
// unchanged: a Barlow/reducer doesn't change the light-gathering
// aperture, only the effective focal length. Returns
// ErrInvalidBarlowFactor if factor is not a positive, finite number.
func (t Telescope) WithBarlow(factor float64) (Telescope, error) {
	if !positiveFinite(factor) {
		return Telescope{}, fmt.Errorf("optics: barlow factor=%v: %w", factor, ErrInvalidBarlowFactor)
	}

	return Telescope{aperture: t.aperture, focalLength: t.focalLength * unit.Length(factor)}, nil
}

// ── Eyepiece ──────────────────────────────────────────────────────────────────

// Eyepiece describes an eyepiece's focal length and apparent field of
// view, with an optional field-stop diameter (WithFieldStop) for an exact
// true-field-of-view computation. Fields are unexported and only reachable
// through the validating constructor NewEyepiece.
type Eyepiece struct {
	afov         angle.Angle
	focalLength  unit.Length
	fieldStop    unit.Length
	hasFieldStop bool
}

// EyepieceOption configures an Eyepiece at construction time.
type EyepieceOption func(*Eyepiece)

// WithFieldStop sets the eyepiece's field-stop diameter — when known
// (typically from the manufacturer's spec), Telescope.TrueFOV uses it for an
// exact true-field-of-view computation instead of the
// apparent-field/magnification approximation.
func WithFieldStop(diameter unit.Length) EyepieceOption {
	return func(e *Eyepiece) {
		e.fieldStop = diameter
		e.hasFieldStop = true
	}
}

// NewEyepiece constructs an Eyepiece from its focal length and apparent field
// of view. Returns ErrNonPositiveDimension if focalLength or apparentFOV is
// not positive, or if a field stop supplied via WithFieldStop is not positive.
//
// As with [NewTelescope], eyepieces are specified in millimeters, so the focal
// length is almost always [unit.Millimeters](x).
func NewEyepiece(
	focalLength unit.Length, apparentFOV angle.Angle, opts ...EyepieceOption,
) (Eyepiece, error) {
	if !positiveFinite(focalLength.Meters()) {
		return Eyepiece{}, fmt.Errorf("optics: eyepiece focalLength=%v: %w",
			focalLength, ErrNonPositiveDimension)
	}

	if !positiveFinite(apparentFOV.Degrees()) {
		return Eyepiece{}, fmt.Errorf("optics: eyepiece apparentFOV=%v: %w", apparentFOV, ErrNonPositiveDimension)
	}

	e := Eyepiece{focalLength: focalLength, afov: apparentFOV}
	for _, opt := range opts {
		opt(&e)
	}

	if e.hasFieldStop && !positiveFinite(e.fieldStop.Meters()) {
		return Eyepiece{}, fmt.Errorf("optics: eyepiece fieldStop=%v: %w",
			e.fieldStop, ErrNonPositiveDimension)
	}

	return e, nil
}

// FocalLength returns the eyepiece's focal length.
func (e Eyepiece) FocalLength() unit.Length { return e.focalLength }

// ApparentFOV returns the eyepiece's apparent field of view.
func (e Eyepiece) ApparentFOV() angle.Angle { return e.afov }

// FieldStop returns the eyepiece's field-stop diameter, and whether one was
// supplied via WithFieldStop.
func (e Eyepiece) FieldStop() (unit.Length, bool) { return e.fieldStop, e.hasFieldStop }

// ── Telescope × Eyepiece ────────────────────────────────────────────────────

// Magnification returns the telescope's magnifying power with eyepiece e:
// telescope focal length divided by eyepiece focal length.
func (t Telescope) Magnification(e Eyepiece) float64 {
	return float64(t.focalLength) / float64(e.focalLength)
}

// TrueFOV returns the actual angular field of view visible through
// eyepiece e. When e has a known field-stop diameter (WithFieldStop), it
// is used for an exact computation — the field stop subtends
// fieldStop/telescopeFocalLength radians at the focal plane. Otherwise it
// falls back to the apparent-field/magnification approximation
// AFOV/magnification, which is exact for a well-corrected eyepiece design
// but only an approximation on wide-field (large AFOV) eyepieces, where
// that simple ratio starts to diverge from the true field-stop-based
// figure — the doc comment on the returned value's precision follows from
// which branch was used, not stated separately here.
func (t Telescope) TrueFOV(e Eyepiece) angle.Angle {
	if fieldStop, ok := e.FieldStop(); ok {
		return angle.Rad(float64(fieldStop) / float64(t.focalLength))
	}

	return e.afov.DivScalar(t.Magnification(e))
}

// ExitPupil returns the exit pupil diameter for eyepiece e: telescope aperture
// divided by magnification. A dark-adapted human eye's pupil is typically
// 5-7mm; an exit pupil larger than the observer's own pupil wastes gathered
// light.
func (t Telescope) ExitPupil(e Eyepiece) unit.Length {
	return t.aperture / unit.Length(t.Magnification(e))
}

// ── Telescope-only figures ──────────────────────────────────────────────────

// MaxUsefulMagnification returns the traditional "2x aperture in mm"
// upper bound on useful magnification — beyond this the image dims and
// softens without resolving further real detail, under typical
// atmospheric seeing. A commonly cited amateur-astronomy rule of thumb,
// not a hard physical limit: excellent optics and steady seeing can push
// somewhat higher, and poor seeing often caps well below it.
func (t Telescope) MaxUsefulMagnification() float64 {
	return 2 * t.aperture.Millimeters()
}

// DawesLimit returns the telescope's Dawes limit — the classical
// resolution limit for splitting two equally bright close stars,
// 116″/aperture(mm) (William Rutter Dawes, 1867, from his own empirical
// double-star observations at ~550nm).
func (t Telescope) DawesLimit() angle.Angle {
	return angle.Arcsec(116 / t.aperture.Millimeters())
}

// LimitingMagnitude returns the telescope's approximate visual limiting
// magnitude under ideal (dark-sky, fully dark-adapted eye) conditions:
// 7.5 + 5·log10(aperture in cm) — a widely used amateur-astronomy rule of
// thumb (e.g. ≈12.5 for a 100mm/4″ telescope), not a rigorous
// signal-detection-theory result. Real limiting magnitude is reduced by
// light pollution, atmospheric transparency, optical quality/collimation,
// and observer experience.
func (t Telescope) LimitingMagnitude() float64 {
	apertureCM := t.aperture.Millimeters() / 10

	return 7.5 + 5*math.Log10(apertureCM)
}

// ── Sensor ────────────────────────────────────────────────────────────────────

// Sensor describes a camera sensor's physical dimensions for
// Telescope.PixelScale/SensorFOV. Unlike Telescope/Eyepiece, its fields
// are plain and exported — there's no invalid combination worth
// construct-time-validating beyond what each computation already handles
// per call; a caller passing a zero-value Sensor gets a zero-value
// (not NaN/Inf) result back, since every field appears only in the
// numerator of these formulas.
type Sensor struct {
	// Width is the sensor's physical width.
	Width unit.Length
	// Height is the sensor's physical height.
	Height unit.Length
	// PixelPitch is the center-to-center pixel spacing. Datasheets quote it
	// in micrometers, so it is usually [unit.Millimeters](microns / 1000).
	PixelPitch unit.Length
}

// PixelScale returns the angular size of one pixel of sensor s at telescope
// t's focal plane — the standard "arcsec per pixel" plate-scale figure.
//
// The classical form is 206.265·pixelPitch(µm)/focalLength(mm), where 206265
// is the number of arcseconds in a radian and the 1000 converts microns to
// millimeters. Both factors existed only to reconcile two units; between two
// [unit.Length] values the small-angle relation is the ratio itself, and
// [angle.Angle] renders it in arcseconds on request.
func (t Telescope) PixelScale(s Sensor) angle.Angle {
	return angle.Rad(float64(s.PixelPitch) / float64(t.focalLength))
}

// SensorFOV returns sensor s's angular field of view (width, height) at
// telescope t's focal plane — the sensor's physical dimensions projected
// through the same small-angle relation TrueFOV's field-stop case uses:
// angle ≈ dimension/focalLength (radians).
func (t Telescope) SensorFOV(s Sensor) (w, h angle.Angle) {
	return angle.Rad(float64(s.Width) / float64(t.focalLength)),
		angle.Rad(float64(s.Height) / float64(t.focalLength))
}
