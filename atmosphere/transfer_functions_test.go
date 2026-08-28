package atmosphere_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// The molecular scale height is the familiar 8.4 km at standard temperature.
//
// # Why the constant is worth pinning
//
// Because it is derived from R_d*T/g rather than tabulated, precisely so a
// warm site and a cold one get different answers — and a derivation is exactly
// the kind of thing that can be right in form and wrong by a factor. "About
// 8.4 km at 288.15 K" is the number every atmospheric text quotes, so it
// checks the gas constant and gravity behind the formula as well as the
// formula.
func TestMolecularScaleHeightIsTheFamiliar8400m(t *testing.T) {
	t.Parallel()

	got, err := atmosphere.MolecularScaleHeight(288.15)
	if err != nil {
		t.Fatalf("MolecularScaleHeight: %v", err)
	}

	if math.Abs(got-8435) > 5 {
		t.Errorf("at 288.15 K the scale height is %.1f m, want about 8435", got)
	}

	// Linear in temperature, which is the whole reason it is derived: colder
	// air is a thinner column.
	cold, err := atmosphere.MolecularScaleHeight(144.075)
	if err != nil {
		t.Fatalf("MolecularScaleHeight: %v", err)
	}

	if rel := math.Abs(cold/got - 0.5); rel > 1e-12 {
		t.Errorf("half the temperature gives %.4f× the scale height, want exactly half",
			cold/got)
	}
}

// A temperature that is not a temperature is refused.
func TestMolecularScaleHeightRefusesNonPositiveTemperature(t *testing.T) {
	t.Parallel()

	for _, k := range []unit.TemperatureK{0, -1, -288.15} {
		if _, err := atmosphere.MolecularScaleHeight(k); !errors.Is(err, atmosphere.ErrTemperature) {
			t.Errorf("%g K was accepted: %v", float64(k), err)
		}
	}
}

// The multiple-scattering factor is Winkler's 1 + 4.5*tau_R.
//
// A correction that is one at zero optical depth and grows from there — if it
// ever returned less than one it would be removing light that multiple
// scattering adds, which is the wrong direction and the only way this can be
// meaningfully wrong.
func TestMultipleScatteringFactorGrowsFromUnity(t *testing.T) {
	t.Parallel()

	at0, err := atmosphere.MultipleScatteringFactor(0)
	if err != nil {
		t.Fatalf("MultipleScatteringFactor: %v", err)
	}

	if at0 != 1 {
		t.Errorf("with no molecular depth the factor is %g, want exactly 1 — there is "+
			"nothing to scatter twice in a vacuum", at0)
	}

	prev := at0

	for _, tau := range []unit.OpticalDepth{0.01, 0.1, 0.15, 0.5} {
		got, err := atmosphere.MultipleScatteringFactor(tau)
		if err != nil {
			t.Fatalf("MultipleScatteringFactor(%g): %v", float64(tau), err)
		}

		if want := 1 + 4.5*float64(tau); math.Abs(got-want) > 1e-12 {
			t.Errorf("at tau %g the factor is %g, want %g", float64(tau), got, want)
		}

		if got <= prev {
			t.Errorf("at tau %g the factor is %g, no larger than the %g below it",
				float64(tau), got, prev)
		}

		prev = got
	}

	// A negative depth is a sign error upstream, not a small correction.
	if _, err := atmosphere.MultipleScatteringFactor(-0.1); !errors.Is(err, atmosphere.ErrOpticalDepth) {
		t.Errorf("a negative optical depth was accepted: %v", err)
	}
}

// At sea level the extended-source depth reduces to kappa*(tau*m) summed over
// the two columns, and altitude removes each with its own scale height.
//
// # What separates this from an airmass multiplication
//
// The two columns thin at different rates. Checking that they are carried
// separately is the point: a single shared scale height gives a plausible
// answer everywhere and the wrong one at any real observatory, because the
// aerosol column is gone far sooner than the molecular one.
func TestExtendedSourceOpticalDepthCarriesTwoColumns(t *testing.T) {
	t.Parallel()

	const (
		rayleigh = 0.12
		aerosol  = 0.08
		airmass  = 2.0
		kappa    = 0.5
	)

	sea, err := atmosphere.ExtendedSourceOpticalDepth(rayleigh, aerosol, airmass, airmass, 0, kappa)
	if err != nil {
		t.Fatalf("ExtendedSourceOpticalDepth: %v", err)
	}

	// At sea level both exponentials are one.
	if want := kappa * (rayleigh*airmass + aerosol*airmass); math.Abs(float64(sea)-want) > 1e-15 {
		t.Errorf("at sea level the depth is %g, want %g", float64(sea), want)
	}

	// Two kilometres up, each column keeps its own fraction.
	const h = 2000

	up, err := atmosphere.ExtendedSourceOpticalDepth(rayleigh, aerosol, airmass, airmass, h, kappa)
	if err != nil {
		t.Fatalf("ExtendedSourceOpticalDepth: %v", err)
	}

	want := kappa * (rayleigh*airmass*math.Exp(-h/atmosphere.MolecularScaleHeightM) +
		aerosol*airmass*math.Exp(-h/atmosphere.AerosolScaleHeightM))

	if math.Abs(float64(up)-want) > 1e-15 {
		t.Errorf("at %g m the depth is %g, want %g", float64(h), float64(up), want)
	}

	if up >= sea {
		t.Errorf("the depth at %g m is %g, no less than the %g at sea level; there is less "+
			"air above a mountain", float64(h), float64(up), float64(sea))
	}

	// The aerosol column thins faster than the molecular one, so the mixture
	// gets relatively more molecular with height. A shared scale height would
	// keep the ratio fixed and pass every check above.
	seaShare := aerosol * airmass / (rayleigh*airmass + aerosol*airmass)

	upAer := aerosol * airmass * math.Exp(-h/atmosphere.AerosolScaleHeightM)
	upMol := rayleigh * airmass * math.Exp(-h/atmosphere.MolecularScaleHeightM)
	upShare := upAer / (upAer + upMol)

	if upShare >= seaShare {
		t.Errorf("the aerosol share is %.4f at %g m against %.4f at sea level; the aerosol "+
			"column has the shorter scale height and must fall away faster",
			upShare, float64(h), seaShare)
	}
}

// Inputs that cannot describe a sightline are refused.
//
// Every one of these returns a finite, plausible optical depth if allowed
// through — an airmass below one is geometrically impossible, a negative
// height is below the ground, and a kappa outside (0,1] would have an
// extended source lose more light than a point source in the same air.
func TestExtendedSourceOpticalDepthRefusesImpossibleInputs(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name                                          string
		rayleigh, aerosol                             unit.OpticalDepth
		molecularAirmass, aerosolAirmass, height, kap float64
		want                                          error
	}{
		{"negative rayleigh", -0.1, 0.08, 1, 1, 0, 0.5, atmosphere.ErrOpticalDepth},
		{"negative aerosol", 0.12, -0.08, 1, 1, 0, 0.5, atmosphere.ErrOpticalDepth},
		{"infinite rayleigh", unit.OpticalDepth(math.Inf(1)), 0.08, 1, 1, 0, 0.5, atmosphere.ErrOpticalDepth},
		{"molecular airmass below one", 0.12, 0.08, 0.5, 1, 0, 0.5, atmosphere.ErrAirmassRange},
		{"aerosol airmass below one", 0.12, 0.08, 1, 0.5, 0, 0.5, atmosphere.ErrAirmassRange},
		{"negative height", 0.12, 0.08, 1, 1, -10, 0.5, atmosphere.ErrOpticalDepth},
		{"zero kappa", 0.12, 0.08, 1, 1, 0, 0, atmosphere.ErrOpticalDepth},
		{"kappa above one", 0.12, 0.08, 1, 1, 0, 1.5, atmosphere.ErrOpticalDepth},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, err := atmosphere.ExtendedSourceOpticalDepth(
				c.rayleigh, c.aerosol, c.molecularAirmass, c.aerosolAirmass, c.height, c.kap)

			if !errors.Is(err, c.want) {
				t.Errorf("got %v, want %v", err, c.want)
			}
		})
	}
}
