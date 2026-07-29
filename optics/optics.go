package optics

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
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
	apertureMM    float64
	focalLengthMM float64
}

// NewTelescope constructs a Telescope from its aperture and focal length,
// both in millimetres. Returns ErrNonPositiveDimension if either is not a
// positive, finite number.
func NewTelescope(apertureMM, focalLengthMM float64) (Telescope, error) {
	if !positiveFinite(apertureMM) || !positiveFinite(focalLengthMM) {
		return Telescope{}, fmt.Errorf("optics: telescope aperture=%v focalLength=%v: %w", apertureMM, focalLengthMM, ErrNonPositiveDimension)
	}

	return Telescope{apertureMM: apertureMM, focalLengthMM: focalLengthMM}, nil
}

// ApertureMM returns the telescope's aperture in millimetres.
func (t Telescope) ApertureMM() float64 { return t.apertureMM }

// FocalLengthMM returns the telescope's focal length in millimetres.
func (t Telescope) FocalLengthMM() float64 { return t.focalLengthMM }

// FocalRatio returns the telescope's focal ratio (f-number): focal length
// divided by aperture — e.g. 10 for an "f/10" telescope.
func (t Telescope) FocalRatio() float64 { return t.focalLengthMM / t.apertureMM }

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

	return Telescope{apertureMM: t.apertureMM, focalLengthMM: t.focalLengthMM * factor}, nil
}

// ── Eyepiece ──────────────────────────────────────────────────────────────────

// Eyepiece describes an eyepiece's focal length and apparent field of
// view, with an optional field-stop diameter (WithFieldStop) for an exact
// true-field-of-view computation. Fields are unexported and only reachable
// through the validating constructor NewEyepiece.
type Eyepiece struct {
	afov          angle.Angle
	focalLengthMM float64
	fieldStopMM   float64
	hasFieldStop  bool
}

// EyepieceOption configures an Eyepiece at construction time.
type EyepieceOption func(*Eyepiece)

// WithFieldStop sets the eyepiece's field-stop diameter in millimetres —
// when known (typically from the manufacturer's spec), Telescope.TrueFOV
// uses it for an exact true-field-of-view computation instead of the
// apparent-field/magnification approximation.
func WithFieldStop(diameterMM float64) EyepieceOption {
	return func(e *Eyepiece) {
		e.fieldStopMM = diameterMM
		e.hasFieldStop = true
	}
}

// NewEyepiece constructs an Eyepiece from its focal length (millimetres)
// and apparent field of view. Returns ErrNonPositiveDimension if
// focalLengthMM or apparentFOV is not positive, or if a field stop
// supplied via WithFieldStop is not positive.
func NewEyepiece(focalLengthMM float64, apparentFOV angle.Angle, opts ...EyepieceOption) (Eyepiece, error) {
	if !positiveFinite(focalLengthMM) {
		return Eyepiece{}, fmt.Errorf("optics: eyepiece focalLength=%v: %w", focalLengthMM, ErrNonPositiveDimension)
	}

	if !positiveFinite(apparentFOV.Degrees()) {
		return Eyepiece{}, fmt.Errorf("optics: eyepiece apparentFOV=%v: %w", apparentFOV, ErrNonPositiveDimension)
	}

	e := Eyepiece{focalLengthMM: focalLengthMM, afov: apparentFOV}
	for _, opt := range opts {
		opt(&e)
	}

	if e.hasFieldStop && !positiveFinite(e.fieldStopMM) {
		return Eyepiece{}, fmt.Errorf("optics: eyepiece fieldStop=%v: %w", e.fieldStopMM, ErrNonPositiveDimension)
	}

	return e, nil
}

// FocalLengthMM returns the eyepiece's focal length in millimetres.
func (e Eyepiece) FocalLengthMM() float64 { return e.focalLengthMM }

// ApparentFOV returns the eyepiece's apparent field of view.
func (e Eyepiece) ApparentFOV() angle.Angle { return e.afov }

// FieldStopMM returns the eyepiece's field-stop diameter in millimetres,
// and whether one was supplied via WithFieldStop.
func (e Eyepiece) FieldStopMM() (mm float64, ok bool) { return e.fieldStopMM, e.hasFieldStop }

// ── Telescope × Eyepiece ────────────────────────────────────────────────────

// Magnification returns the telescope's magnifying power with eyepiece e:
// telescope focal length divided by eyepiece focal length.
func (t Telescope) Magnification(e Eyepiece) float64 {
	return t.focalLengthMM / e.focalLengthMM
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
	if fieldStopMM, ok := e.FieldStopMM(); ok {
		return angle.Rad(fieldStopMM / t.focalLengthMM)
	}

	return e.afov.DivScalar(t.Magnification(e))
}

// ExitPupil returns the exit pupil diameter in millimetres for eyepiece
// e: telescope aperture divided by magnification. A dark-adapted human
// eye's pupil is typically 5-7mm; an exit pupil larger than the
// observer's own pupil wastes gathered light.
func (t Telescope) ExitPupil(e Eyepiece) float64 {
	return t.apertureMM / t.Magnification(e)
}

// ── Telescope-only figures ──────────────────────────────────────────────────

// MaxUsefulMagnification returns the traditional "2x aperture in mm"
// upper bound on useful magnification — beyond this the image dims and
// softens without resolving further real detail, under typical
// atmospheric seeing. A commonly cited amateur-astronomy rule of thumb,
// not a hard physical limit: excellent optics and steady seeing can push
// somewhat higher, and poor seeing often caps well below it.
func (t Telescope) MaxUsefulMagnification() float64 {
	return 2 * t.apertureMM
}

// DawesLimit returns the telescope's Dawes limit — the classical
// resolution limit for splitting two equally bright close stars,
// 116″/aperture(mm) (William Rutter Dawes, 1867, from his own empirical
// double-star observations at ~550nm).
func (t Telescope) DawesLimit() angle.Angle {
	return angle.Arcsec(116 / t.apertureMM)
}

// LimitingMagnitude returns the telescope's approximate visual limiting
// magnitude under ideal (dark-sky, fully dark-adapted eye) conditions:
// 7.5 + 5·log10(aperture in cm) — a widely used amateur-astronomy rule of
// thumb (e.g. ≈12.5 for a 100mm/4″ telescope), not a rigorous
// signal-detection-theory result. Real limiting magnitude is reduced by
// light pollution, atmospheric transparency, optical quality/collimation,
// and observer experience.
func (t Telescope) LimitingMagnitude() float64 {
	apertureCM := t.apertureMM / 10

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
	// WidthMM is the sensor's physical width in millimetres.
	WidthMM float64
	// HeightMM is the sensor's physical height in millimetres.
	HeightMM float64
	// PixelMicrons is the pixel pitch (centre-to-centre spacing) in
	// micrometres.
	PixelMicrons float64
}

// PixelScale returns the angular size of one pixel of sensor s at
// telescope t's focal plane — the standard "arcsec per pixel" plate-scale
// figure: 206.265·pixelPitch(µm)/focalLength(mm) (206265 is the number of
// arcseconds in a radian, the small-angle plate-scale constant).
func (t Telescope) PixelScale(s Sensor) angle.Angle {
	return angle.Arcsec(206_265 * s.PixelMicrons / 1000 / t.focalLengthMM)
}

// SensorFOV returns sensor s's angular field of view (width, height) at
// telescope t's focal plane — the sensor's physical dimensions projected
// through the same small-angle relation TrueFOV's field-stop case uses:
// angle ≈ dimension/focalLength (radians).
func (t Telescope) SensorFOV(s Sensor) (w, h angle.Angle) {
	return angle.Rad(s.WidthMM / t.focalLengthMM), angle.Rad(s.HeightMM / t.focalLengthMM)
}
