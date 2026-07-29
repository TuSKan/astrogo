package optics_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/optics"
)

// A 200mm f/10 (2000mm focal length) telescope with a 25mm, 68° apparent
// field of view eyepiece — a common real-world combination used as the
// known-value fixture throughout this file: 80x magnification, 0.85° true
// field of view (no field stop supplied), 2.5mm exit pupil, 0.58″ Dawes
// limit, 400x max useful magnification.
func newFixture(t *testing.T) (optics.Telescope, optics.Eyepiece) {
	t.Helper()

	scope, err := optics.NewTelescope(200, 2000)
	if err != nil {
		t.Fatalf("NewTelescope: %v", err)
	}

	eyepiece, err := optics.NewEyepiece(25, angle.Deg(68))
	if err != nil {
		t.Fatalf("NewEyepiece: %v", err)
	}

	return scope, eyepiece
}

func TestTelescope_FocalRatio(t *testing.T) {
	scope, _ := newFixture(t)

	if got := scope.FocalRatio(); math.Abs(got-10) > 1e-9 {
		t.Errorf("FocalRatio = %v, want 10", got)
	}
}

func TestTelescope_Magnification(t *testing.T) {
	scope, eyepiece := newFixture(t)

	if got := scope.Magnification(eyepiece); math.Abs(got-80) > 1e-9 {
		t.Errorf("Magnification = %v, want 80", got)
	}
}

func TestTelescope_TrueFOV_NoFieldStop(t *testing.T) {
	scope, eyepiece := newFixture(t)

	got := scope.TrueFOV(eyepiece).Degrees()
	if math.Abs(got-0.85) > 1e-6 {
		t.Errorf("TrueFOV = %v°, want 0.85°", got)
	}
}

// TestTelescope_TrueFOV_WithFieldStop confirms the field-stop branch is
// actually used (and differs from the AFOV/magnification approximation)
// when a field stop is supplied. A 68° eyepiece with a 27.8mm field stop
// (25mm TeleVue-style Plössl-class eyepiece) — TFOV = fieldStop/scopeFL
// in radians.
func TestTelescope_TrueFOV_WithFieldStop(t *testing.T) {
	scope, err := optics.NewTelescope(200, 2000)
	if err != nil {
		t.Fatalf("NewTelescope: %v", err)
	}

	eyepiece, err := optics.NewEyepiece(25, angle.Deg(68), optics.WithFieldStop(27.8))
	if err != nil {
		t.Fatalf("NewEyepiece: %v", err)
	}

	want := angle.Rad(27.8 / 2000).Degrees()

	got := scope.TrueFOV(eyepiece).Degrees()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("TrueFOV (field stop) = %v°, want %v°", got, want)
	}

	// Sanity: the field-stop-based figure should differ from the
	// AFOV/magnification approximation (they're not identical formulas).
	approx := eyepiece.ApparentFOV().DivScalar(scope.Magnification(eyepiece)).Degrees()
	if math.Abs(got-approx) < 1e-9 {
		t.Error("field-stop TrueFOV unexpectedly identical to the AFOV/magnification approximation")
	}
}

func TestTelescope_ExitPupil(t *testing.T) {
	scope, eyepiece := newFixture(t)

	if got := scope.ExitPupil(eyepiece); math.Abs(got-2.5) > 1e-9 {
		t.Errorf("ExitPupil = %v mm, want 2.5 mm", got)
	}
}

func TestTelescope_DawesLimit(t *testing.T) {
	scope, _ := newFixture(t)

	if got := scope.DawesLimit().Arcseconds(); math.Abs(got-0.58) > 1e-6 {
		t.Errorf("DawesLimit = %v″, want 0.58″", got)
	}
}

func TestTelescope_MaxUsefulMagnification(t *testing.T) {
	scope, _ := newFixture(t)

	if got := scope.MaxUsefulMagnification(); math.Abs(got-400) > 1e-9 {
		t.Errorf("MaxUsefulMagnification = %v, want 400", got)
	}
}

// TestTelescope_LimitingMagnitude cross-checks against the well-known
// "12.5 for a 100mm/4-inch telescope" figure widely cited alongside this
// formula (7.5 + 5·log10(aperture in cm)).
func TestTelescope_LimitingMagnitude(t *testing.T) {
	scope, err := optics.NewTelescope(100, 1000)
	if err != nil {
		t.Fatalf("NewTelescope: %v", err)
	}

	if got := scope.LimitingMagnitude(); math.Abs(got-12.5) > 1e-9 {
		t.Errorf("LimitingMagnitude = %v, want 12.5", got)
	}
}

func TestTelescope_WithBarlow(t *testing.T) {
	scope, err := optics.NewTelescope(200, 2000)
	if err != nil {
		t.Fatalf("NewTelescope: %v", err)
	}

	barlowed, err := scope.WithBarlow(2)
	if err != nil {
		t.Fatalf("WithBarlow: %v", err)
	}

	if got := barlowed.FocalLengthMM(); math.Abs(got-4000) > 1e-9 {
		t.Errorf("WithBarlow(2) FocalLengthMM = %v, want 4000", got)
	}

	if got := barlowed.ApertureMM(); math.Abs(got-200) > 1e-9 {
		t.Errorf("WithBarlow(2) ApertureMM = %v, want unchanged 200", got)
	}

	reduced, err := scope.WithBarlow(0.63)
	if err != nil {
		t.Fatalf("WithBarlow(reducer): %v", err)
	}

	if got := reduced.FocalLengthMM(); math.Abs(got-1260) > 1e-9 {
		t.Errorf("WithBarlow(0.63) FocalLengthMM = %v, want 1260", got)
	}

	if _, err := scope.WithBarlow(0); !errors.Is(err, optics.ErrInvalidBarlowFactor) {
		t.Errorf("WithBarlow(0) error = %v, want ErrInvalidBarlowFactor", err)
	}

	if _, err := scope.WithBarlow(-1); !errors.Is(err, optics.ErrInvalidBarlowFactor) {
		t.Errorf("WithBarlow(-1) error = %v, want ErrInvalidBarlowFactor", err)
	}
}

func TestTelescope_PixelScaleAndSensorFOV(t *testing.T) {
	scope, err := optics.NewTelescope(200, 1000) // 1000mm FL
	if err != nil {
		t.Fatalf("NewTelescope: %v", err)
	}

	sensor := optics.Sensor{WidthMM: 23.5, HeightMM: 15.6, PixelMicrons: 3.76}

	wantScale := 206_265 * 3.76 / 1000 / 1000 // arcsec/pixel
	if got := scope.PixelScale(sensor).Arcseconds(); math.Abs(got-wantScale) > 1e-9 {
		t.Errorf("PixelScale = %v″, want %v″", got, wantScale)
	}

	w, h := scope.SensorFOV(sensor)

	wantW := angle.Rad(23.5 / 1000).Degrees()
	wantH := angle.Rad(15.6 / 1000).Degrees()

	if got := w.Degrees(); math.Abs(got-wantW) > 1e-9 {
		t.Errorf("SensorFOV width = %v°, want %v°", got, wantW)
	}

	if got := h.Degrees(); math.Abs(got-wantH) > 1e-9 {
		t.Errorf("SensorFOV height = %v°, want %v°", got, wantH)
	}
}

// ── Error cases ──────────────────────────────────────────────────────────────

func TestNewTelescope_RejectsNonPositiveDimensions(t *testing.T) {
	cases := []struct {
		aperture, focalLength float64
	}{
		{0, 1000}, {200, 0}, {-200, 1000}, {200, -1000},
		{math.NaN(), 1000}, {math.Inf(1), 1000},
	}

	for _, c := range cases {
		if _, err := optics.NewTelescope(c.aperture, c.focalLength); !errors.Is(err, optics.ErrNonPositiveDimension) {
			t.Errorf("NewTelescope(%v, %v) error = %v, want ErrNonPositiveDimension", c.aperture, c.focalLength, err)
		}
	}
}

func TestNewEyepiece_RejectsNonPositiveDimensions(t *testing.T) {
	if _, err := optics.NewEyepiece(0, angle.Deg(68)); !errors.Is(err, optics.ErrNonPositiveDimension) {
		t.Errorf("NewEyepiece(0, ...) error = %v, want ErrNonPositiveDimension", err)
	}

	if _, err := optics.NewEyepiece(-25, angle.Deg(68)); !errors.Is(err, optics.ErrNonPositiveDimension) {
		t.Errorf("NewEyepiece(-25, ...) error = %v, want ErrNonPositiveDimension", err)
	}

	if _, err := optics.NewEyepiece(25, angle.Deg(0)); !errors.Is(err, optics.ErrNonPositiveDimension) {
		t.Errorf("NewEyepiece(25, 0°) error = %v, want ErrNonPositiveDimension", err)
	}

	if _, err := optics.NewEyepiece(25, angle.Deg(-10)); !errors.Is(err, optics.ErrNonPositiveDimension) {
		t.Errorf("NewEyepiece(25, -10°) error = %v, want ErrNonPositiveDimension", err)
	}

	if _, err := optics.NewEyepiece(25, angle.Deg(68), optics.WithFieldStop(0)); !errors.Is(err, optics.ErrNonPositiveDimension) {
		t.Errorf("NewEyepiece(field stop=0) error = %v, want ErrNonPositiveDimension", err)
	}

	if _, err := optics.NewEyepiece(25, angle.Deg(68), optics.WithFieldStop(-5)); !errors.Is(err, optics.ErrNonPositiveDimension) {
		t.Errorf("NewEyepiece(field stop=-5) error = %v, want ErrNonPositiveDimension", err)
	}
}

func TestEyepiece_FieldStopMM(t *testing.T) {
	noStop, err := optics.NewEyepiece(25, angle.Deg(68))
	if err != nil {
		t.Fatalf("NewEyepiece: %v", err)
	}

	if _, ok := noStop.FieldStopMM(); ok {
		t.Error("FieldStopMM: ok = true for an eyepiece with no WithFieldStop option")
	}

	withStop, err := optics.NewEyepiece(25, angle.Deg(68), optics.WithFieldStop(27.8))
	if err != nil {
		t.Fatalf("NewEyepiece(field stop): %v", err)
	}

	mm, ok := withStop.FieldStopMM()
	if !ok || math.Abs(mm-27.8) > 1e-9 {
		t.Errorf("FieldStopMM = (%v, %v), want (27.8, true)", mm, ok)
	}
}
