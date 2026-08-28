package atmosphere

import (
	"math"
	"testing"
)

// ── Composition: Atmosphere embeds Refraction as its surface field ─────────

func TestAtmosphere_SurfaceRoundTrip(t *testing.T) {
	atm, err := NewBuilder().Surface(1000, 300).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	p, tempK := atm.Surface()
	if p != 1000 {
		t.Errorf("Surface() pressure = %v, want 1000", p)
	}

	if math.Abs(float64(tempK)-300) > 1e-9 {
		t.Errorf("Surface() temperature = %v K, want 300 K", tempK)
	}

	// The one explicit unit conversion this composition requires: the
	// embedded Refraction stores temperature in Celsius.
	refr := atm.Refraction()
	if math.Abs(refr.Temperature-26.85) > 1e-9 {
		t.Errorf("Refraction().Temperature = %v °C, want 26.85 °C", refr.Temperature)
	}

	if refr.Pressure != 1000 {
		t.Errorf("Refraction().Pressure = %v, want 1000", refr.Pressure)
	}
}

func TestBuilder_Refraction(t *testing.T) {
	model := RefractionRigorous{}

	atm, err := NewBuilder().
		Surface(1013.25, 288.15).
		Refraction(model, 0.4, 0.55).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	refr := atm.Refraction()
	if refr.Model != model {
		t.Errorf("Refraction().Model = %v, want %v", refr.Model, model)
	}

	if refr.Humidity != 0.4 {
		t.Errorf("Refraction().Humidity = %v, want 0.4", refr.Humidity)
	}

	if refr.Wavelength != 0.55 {
		t.Errorf("Refraction().Wavelength = %v, want 0.55", refr.Wavelength)
	}

	// Surface() must be unaffected by Refraction()'s fields.
	p, tempK := atm.Surface()
	if p != 1013.25 || math.Abs(float64(tempK)-288.15) > 1e-9 {
		t.Errorf("Surface() = (%v, %v), want (1013.25, 288.15)", p, tempK)
	}
}

func TestBuilder_RefractionDefaultsToNoModel(t *testing.T) {
	atm, err := NewBuilder().Surface(1013.25, 288.15).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if atm.Refraction().Model != nil {
		t.Errorf("Refraction().Model = %v, want nil when Builder.Refraction was never called", atm.Refraction().Model)
	}
}

// TestStandardDefault_MatchesAtAltitude proves StandardDefault's
// composition-based construction (surface: AtAltitude(heightM)) is
// behavior-preserving versus the old manual Celsius/Kelvin conversion.
func TestStandardDefault_MatchesAtAltitude(t *testing.T) {
	for _, h := range []float64{0, 500, 2635, 8849} {
		want := AtAltitude(h)
		got := StandardDefault(h).Refraction()

		if got != want {
			t.Errorf("StandardDefault(%v).Refraction() = %+v, want %+v", h, got, want)
		}
	}
}

func TestAtmosphere_ErrAtmosphereBuilder(t *testing.T) {
	_, err := NewBuilder().Surface(-1, -1).Build()
	if err == nil {
		t.Fatal("expected an error for negative pressure/temperature")
	}
}
