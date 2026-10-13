package atmosphere

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/unit"
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
	model := RefractionBennett{}

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
		want := AtAltitude(unit.Meters(h))
		got := StandardDefault(unit.Meters(h)).Refraction()

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

// TestBuilderRefusesNonFiniteInputs (#750): every Builder check was a plain
// comparison, which NaN fails, so only Ozone refused it; a NaN AOD built an
// atmosphere whose extinction was NaN with no error anywhere. Each input is
// tried with NaN and the infinities that its range does not admit.
func TestBuilderRefusesNonFiniteInputs(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)

	for _, c := range []struct {
		name string
		b    *Builder
		want error
	}{
		{"pressure NaN", NewBuilder().Surface(nan, 288), errSurfacePressure},
		{"pressure +Inf", NewBuilder().Surface(inf, 288), errSurfacePressure},
		{"temperature NaN", NewBuilder().Surface(1013, nan), errSurfaceTemperature},
		{"temperature +Inf", NewBuilder().Surface(1013, inf), errSurfaceTemperature},
		{"AOD NaN", NewBuilder().Aerosol(nan, 550, 1.3, 0.9, 0.7), errAerosolAOD},
		{"AOD +Inf", NewBuilder().Aerosol(inf, 550, 1.3, 0.9, 0.7), errAerosolAOD},
		{"reference wavelength 0", NewBuilder().Aerosol(0.1, 0, 1.3, 0.9, 0.7), errAerosolReference},
		{"reference wavelength NaN", NewBuilder().Aerosol(0.1, unit.WavelengthNM(nan), 1.3, 0.9, 0.7), errAerosolReference},
		{"Angstrom NaN", NewBuilder().Aerosol(0.1, 550, nan, 0.9, 0.7), errAerosolAngstrom},
		{"Angstrom -Inf", NewBuilder().Aerosol(0.1, 550, -inf, 0.9, 0.7), errAerosolAngstrom},
		{"SSA NaN", NewBuilder().Aerosol(0.1, 550, 1.3, nan, 0.7), errAerosolSSA},
		{"g NaN", NewBuilder().Aerosol(0.1, 550, 1.3, 0.9, nan), errAerosolAsymmetry},
		{"cloud fraction NaN", NewBuilder().AddCloud(CloudLayer{Fraction: unit.CloudFraction(nan)}), errCloudFraction},
		{"cloud optical depth NaN", NewBuilder().AddCloud(CloudLayer{Fraction: 0.5, OpticalDepth: unit.CloudOpticalDepth(nan)}), errCloudOpticalDepth},
		{"cloud optical depth +Inf", NewBuilder().AddCloud(CloudLayer{Fraction: 0.5, OpticalDepth: unit.CloudOpticalDepth(inf)}), errCloudOpticalDepth},
	} {
		if _, err := c.b.Build(); !errors.Is(err, ErrAtmosphereBuilder) || !errors.Is(err, c.want) {
			t.Errorf("%s: Build() = %v, want ErrAtmosphereBuilder joining %v", c.name, err, c.want)
		}
	}
}

// TestAirmassRefusesANonFiniteElevation (#750): NaN is below nothing, so it
// passed the horizon check and came back as an airmass of NaN.
func TestAirmassRefusesANonFiniteElevation(t *testing.T) {
	for _, h := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := Airmass(angle.Deg(h)); !errors.Is(err, ErrAltitude) {
			t.Errorf("Airmass(%v) = %v, want ErrAltitude", h, err)
		}

		if _, err := OzoneAirmass(angle.Deg(h)); !errors.Is(err, ErrAltitude) {
			t.Errorf("OzoneAirmass(%v) = %v, want ErrAltitude", h, err)
		}
	}
}

// Ozone refuses a column that would make ozone emit rather than absorb, and
// one that is not a number at all; Extinction would otherwise brighten a star
// with it. No ozone is a column like any other.
func TestBuilder_OzoneRefusesAnImpossibleColumn(t *testing.T) {
	for _, du := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := NewBuilder().Ozone(du).Build()
		if !errors.Is(err, ErrAtmosphereBuilder) || !errors.Is(err, errOzoneColumn) {
			t.Errorf("Ozone(%v).Build() = %v, want ErrAtmosphereBuilder joining errOzoneColumn", du, err)
		}
	}

	for _, du := range []float64{0, 258} {
		air, err := NewBuilder().Ozone(du).Build()
		if err != nil {
			t.Fatalf("Ozone(%g).Build(): %v", du, err)
		}

		if got := air.Ozone(); got != unit.OzoneColumnDU(du) {
			t.Errorf("Ozone(%g) built an Atmosphere holding %v DU", du, got)
		}
	}
}
