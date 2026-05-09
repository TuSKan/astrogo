package atmosphere

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// ── Refraction Correctness ───────────────────────────────────────────────────

func TestRefractionRigorous_KnownValues(t *testing.T) {
	model := RefractionRigorous{}
	env := StandardAtmosphere

	tests := []struct {
		name   string
		alt    float64 // degrees
		minRef float64 // minimum refraction in arcminutes
		maxRef float64 // maximum refraction in arcminutes
	}{
		{"zenith_90", 90, 0.0, 0.01},
		{"high_45", 45, 0.9, 1.1},
		{"medium_20", 20, 2.4, 2.8},
		{"low_10", 10, 5.0, 5.6},
		{"horizon_0", 0, 28.0, 40.0},
		// Below horizon: should return 0
		{"below_-10", -10, 0.0, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := model.RefractFromTrue(angle.Deg(tt.alt), env)
			refArcmin := math.Abs(ref.Degrees() * 60.0)
			if refArcmin < tt.minRef || refArcmin > tt.maxRef {
				t.Errorf("altitude=%g°: refraction=%.3f arcmin, want [%.1f, %.1f]",
					tt.alt, refArcmin, tt.minRef, tt.maxRef)
			}
		})
	}
}

func TestRefractionApproximate_KnownValues(t *testing.T) {
	model := RefractionApproximate{}
	env := StandardAtmosphere

	// At 45° altitude, Saemundsson and Bennett should agree within ~0.1 arcmin.
	refTrue := model.RefractFromTrue(angle.Deg(45), env)
	refApp := model.RefractFromApparent(angle.Deg(45), env)

	diffArcmin := math.Abs(refTrue.Degrees()-refApp.Degrees()) * 60.0
	if diffArcmin > 0.15 {
		t.Errorf("true vs apparent at 45°: diff=%.3f arcmin, want <0.15", diffArcmin)
	}
}

func TestRefraction_ZeroPressure(t *testing.T) {
	model := RefractionRigorous{}
	env := Atmosphere{Pressure: 0, Temperature: 15, Humidity: 0.5, Wavelength: 0.55}

	ref := model.RefractFromTrue(angle.Deg(45), env)
	if ref.Degrees() != 0 {
		t.Errorf("zero pressure should produce zero refraction, got %v", ref)
	}
}

func TestRefraction_WavelengthDependence(t *testing.T) {
	model := RefractionRigorous{}
	envBlue := Atmosphere{Pressure: 1013.25, Temperature: 15, Humidity: 0.5, Wavelength: 0.40}
	envRed := Atmosphere{Pressure: 1013.25, Temperature: 15, Humidity: 0.5, Wavelength: 0.70}

	refBlue := model.RefractFromTrue(angle.Deg(20), envBlue)
	refRed := model.RefractFromTrue(angle.Deg(20), envRed)

	// Shorter wavelength (blue) should refract MORE than longer wavelength (red).
	if refBlue.Degrees() <= refRed.Degrees() {
		t.Errorf("blue (λ=0.40μm) should refract more than red (λ=0.70μm): blue=%.4f° red=%.4f°",
			refBlue.Degrees(), refRed.Degrees())
	}
}

func TestRefractionNone(t *testing.T) {
	model := RefractionNone{}
	env := StandardAtmosphere

	ref := model.RefractFromTrue(angle.Deg(10), env)
	if ref != 0 {
		t.Errorf("RefractionNone should return 0, got %v", ref)
	}
	ref = model.RefractFromApparent(angle.Deg(10), env)
	if ref != 0 {
		t.Errorf("RefractionNone should return 0, got %v", ref)
	}
}

// ── Airmass Correctness ─────────────────────────────────────────────────────

func TestAirmass_KnownValues(t *testing.T) {
	tests := []struct {
		alt     float64
		wantMin float64
		wantMax float64
	}{
		{90, 0.99, 1.01}, // Zenith: X = 1.0
		{30, 1.95, 2.05}, // X ≈ 2.0
		{0, 35.0, 42.0},  // Horizon: X ≈ 38 (Pickering)
	}

	for _, tt := range tests {
		am, err := Airmass(angle.Deg(tt.alt))
		if err != nil {
			t.Errorf("altitude=%g°: unexpected error: %v", tt.alt, err)
			continue
		}
		if am < tt.wantMin || am > tt.wantMax {
			t.Errorf("altitude=%g°: airmass=%.2f, want [%.1f, %.1f]",
				tt.alt, am, tt.wantMin, tt.wantMax)
		}
	}
}

func TestAirmass_BelowHorizon(t *testing.T) {
	_, err := Airmass(angle.Deg(-5))
	if !errors.Is(err, ErrBelowHorizon) {
		t.Errorf("expected ErrBelowHorizon for alt=-5°, got %v", err)
	}
}

func TestAirmass_Monotonic(t *testing.T) {
	// Airmass should increase monotonically as altitude decreases.
	prev := 0.0
	for alt := 89.0; alt >= 1.0; alt -= 1.0 {
		am, err := Airmass(angle.Deg(alt))
		if err != nil {
			t.Fatalf("altitude=%g°: %v", alt, err)
		}
		if am <= prev {
			t.Errorf("airmass not monotonic: X(%.0f°)=%.3f <= X(%.0f°)=%.3f",
				alt, am, alt+1, prev)
		}
		prev = am
	}
}

// ── AtAltitude Correctness ──────────────────────────────────────────────────

func TestAtAltitude_SeaLevel(t *testing.T) {
	atm := AtAltitude(0)
	if math.Abs(atm.Pressure-1013.25) > 0.01 {
		t.Errorf("sea level pressure: got %.2f, want 1013.25", atm.Pressure)
	}
	if math.Abs(atm.Temperature-15.0) > 0.01 {
		t.Errorf("sea level temperature: got %.2f, want 15.0", atm.Temperature)
	}
	// Model must be nil so SOFA handles refraction.
	if atm.Model != nil {
		t.Error("AtAltitude(0) should return nil Model for SOFA refraction")
	}
}

func TestAtAltitude_Pressure_Decreases(t *testing.T) {
	prev := AtAltitude(0).Pressure
	for _, h := range []float64{500, 1000, 2000, 3000, 5000, 8000} {
		atm := AtAltitude(h)
		if atm.Pressure >= prev {
			t.Errorf("pressure not decreasing: P(%.0fm)=%.2f >= P(prev)=%.2f", h, atm.Pressure, prev)
		}
		prev = atm.Pressure
	}
}

func TestAtAltitude_Everest(t *testing.T) {
	// Everest summit (~8849m): pressure should be ~315 hPa, temp ~-42°C
	atm := AtAltitude(8849)
	if atm.Pressure < 300 || atm.Pressure > 340 {
		t.Errorf("Everest pressure: got %.1f hPa, want ~315 hPa", atm.Pressure)
	}
	if atm.Temperature < -48 || atm.Temperature > -38 {
		t.Errorf("Everest temperature: got %.1f°C, want ~-42.5°C", atm.Temperature)
	}
}

func TestAtAltitude_ModelAlwaysNil(t *testing.T) {
	for _, h := range []float64{-100, 0, 100, 2000, 5000} {
		atm := AtAltitude(h)
		if atm.Model != nil {
			t.Errorf("AtAltitude(%.0f): Model should be nil, got %T", h, atm.Model)
		}
	}
}

// ── HorizonDip ──────────────────────────────────────────────────────────────

func TestHorizonDip(t *testing.T) {
	// Sea level: no dip.
	dip := HorizonDip(0)
	if dip.Degrees() != 0 {
		t.Errorf("HorizonDip(0) = %v, want 0", dip)
	}

	// 100m: dip ≈ 0.29°
	dip100 := HorizonDip(100)
	if dip100.Degrees() < 0.25 || dip100.Degrees() > 0.35 {
		t.Errorf("HorizonDip(100m) = %.4f°, want ~0.29°", dip100.Degrees())
	}

	// Monotonically increasing.
	prev := 0.0
	for _, h := range []float64{10, 50, 100, 500, 1000, 5000} {
		d := HorizonDip(h).Degrees()
		if d <= prev {
			t.Errorf("dip not increasing: HorizonDip(%.0fm)=%.4f° <= prev=%.4f°", h, d, prev)
		}
		prev = d
	}
}

// ── ZenithDistance ───────────────────────────────────────────────────────────

func TestZenithDistance(t *testing.T) {
	zd := ZenithDistance(angle.Deg(30))
	if math.Abs(zd.Degrees()-60.0) > 0.001 {
		t.Errorf("ZenithDistance(30°) = %v, want 60°", zd)
	}
	zd90 := ZenithDistance(angle.Deg(90))
	if math.Abs(zd90.Degrees()) > 0.001 {
		t.Errorf("ZenithDistance(90°) = %v, want 0°", zd90)
	}
}
