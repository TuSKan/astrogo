package atmosphere_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
)

// TestRefractionValidateRefusesWhatIsNotAir: a Refraction is checked as air,
// and air that fails is refracted as NaN on every path rather than through
// SOFA's clamp of it (#903).
//
// iauRefco clamps each input with a comparison, which a NaN fails, so a NaN
// came out as the clamp's bound: a NaN temperature refracted as −150 °C,
// 365.64″ against 155.37″ at 20°; an infinite pressure as 10,000 hPa,
// 1517.50″. A NaN pressure was no refraction at all. Each is now NaN, which
// cannot pass for an altitude.
func TestRefractionValidateRefusesWhatIsNotAir(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		air  atmosphere.Refraction
	}{
		{"standard", atmosphere.StandardRefraction()},
		{"4,205 m", atmosphere.AtAltitude(4205)},
		{"no refraction", atmosphere.Refraction{}},
		{"cold, dense, dry, red", atmosphere.Refraction{Pressure: 1030, Temperature: -40, Wavelength: 0.7}},
	} {
		if err := c.air.Validate(); err != nil {
			t.Errorf("%s: Validate = %v, want nil", c.name, err)
		}
	}

	nan, inf := math.NaN(), math.Inf(1)

	for _, c := range []struct {
		name  string
		spoil func(*atmosphere.Refraction)
	}{
		{"NaN pressure", func(r *atmosphere.Refraction) { r.Pressure = nan }},
		{"infinite pressure", func(r *atmosphere.Refraction) { r.Pressure = inf }},
		{"negative pressure", func(r *atmosphere.Refraction) { r.Pressure = -1 }},
		{"NaN temperature", func(r *atmosphere.Refraction) { r.Temperature = nan }},
		{"infinite cold", func(r *atmosphere.Refraction) { r.Temperature = -inf }},
		{"absolute zero", func(r *atmosphere.Refraction) { r.Temperature = -273.15 }},
		{"NaN humidity", func(r *atmosphere.Refraction) { r.Humidity = nan }},
		{"humidity above 1", func(r *atmosphere.Refraction) { r.Humidity = 1.01 }},
		{"negative humidity", func(r *atmosphere.Refraction) { r.Humidity = -0.01 }},
		{"NaN wavelength", func(r *atmosphere.Refraction) { r.Wavelength = nan }},
		{"infinite wavelength", func(r *atmosphere.Refraction) { r.Wavelength = inf }},
		{"negative wavelength", func(r *atmosphere.Refraction) { r.Wavelength = -0.5 }},
	} {
		air := atmosphere.StandardRefraction()
		c.spoil(&air)

		if err := air.Validate(); !errors.Is(err, atmosphere.ErrRefraction) {
			t.Errorf("%s: Validate = %v, want ErrRefraction", c.name, err)
		}

		alt := angle.Deg(20)

		for _, m := range []struct {
			name  string
			model atmosphere.RefractionModel
		}{
			{"the default model", nil},
			{"RefractionSOFA", atmosphere.RefractionSOFA{}},
			{"RefractionBennett", atmosphere.RefractionBennett{}},
		} {
			env := air
			env.Model = m.model

			if r := env.RefractFromTrue(alt); !math.IsNaN(r.Radians()) {
				t.Errorf("%s, %s: RefractFromTrue = %.3f″, want NaN", c.name, m.name, r.Arcseconds())
			}

			if r := env.RefractFromApparent(alt); !math.IsNaN(r.Radians()) {
				t.Errorf("%s, %s: RefractFromApparent = %.3f″, want NaN", c.name, m.name, r.Arcseconds())
			}

			// The models themselves, for a caller who holds one.
			if m.model == nil {
				continue
			}

			if r := m.model.RefractFromTrue(alt, air); !math.IsNaN(r.Radians()) {
				t.Errorf("%s: %s.RefractFromTrue = %.3f″, want NaN", c.name, m.name, r.Arcseconds())
			}

			if r := m.model.RefractFromApparent(alt, air); !math.IsNaN(r.Radians()) {
				t.Errorf("%s: %s.RefractFromApparent = %.3f″, want NaN", c.name, m.name, r.Arcseconds())
			}
		}
	}
}

// TestBuilderRefusesARefractionThatIsNotAir: Builder.Refraction's humidity and
// wavelength are checked as Surface's pressure and temperature already were,
// so a NaN from a weather feed fails Build instead of reaching SOFA (#903).
func TestBuilderRefusesARefractionThatIsNotAir(t *testing.T) {
	t.Parallel()

	if _, err := atmosphere.NewBuilder().Refraction(nil, 0.5, 0.55).Build(); err != nil {
		t.Fatalf("a valid refraction failed to build: %v", err)
	}

	for _, c := range []struct {
		name                   string
		humidity, wavelengthUM float64
	}{
		{"NaN humidity", math.NaN(), 0.55},
		{"humidity above 1", 1.5, 0.55},
		{"NaN wavelength", 0.5, math.NaN()},
		{"negative wavelength", 0.5, -0.55},
	} {
		if _, err := atmosphere.NewBuilder().Refraction(nil, c.humidity, c.wavelengthUM).Build(); !errors.Is(err, atmosphere.ErrRefraction) {
			t.Errorf("%s: Build = %v, want ErrRefraction", c.name, err)
		}
	}
}
