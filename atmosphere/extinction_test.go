package atmosphere_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// Extinction is its three terms added, each from the Atmosphere's own state:
// with no ozone and no aerosol it is exactly the Rayleigh optical depth in
// magnitudes, ozone adds in proportion to its column, and aerosol adds its
// own optical depth.
func TestExtinctionIsTheSumOfItsTerms(t *testing.T) {
	t.Parallel()

	const lambda = unit.WavelengthNM(550)

	build := func(b *atmosphere.Builder) *atmosphere.Atmosphere {
		t.Helper()

		air, err := b.Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}

		return air
	}

	k := func(air *atmosphere.Atmosphere) float64 {
		t.Helper()

		v, err := air.Extinction(lambda)
		if err != nil {
			t.Fatalf("Extinction: %v", err)
		}

		return v
	}

	molecular := build(atmosphere.NewBuilder().Surface(700, 280))

	tau, err := atmosphere.RayleighOpticalDepth(lambda, 700)
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	if got, want := k(molecular), magPerTau*float64(tau); got != want {
		t.Errorf("with no ozone and no aerosol, Extinction is %v, want the Rayleigh term %v", got, want)
	}

	ozone := k(build(atmosphere.NewBuilder().Surface(700, 280).Ozone(300))) - k(molecular)
	twice := k(build(atmosphere.NewBuilder().Surface(700, 280).Ozone(600))) - k(molecular)

	if ozone <= 0 {
		t.Fatalf("300 DU of ozone adds %v at 550 nm, in the Chappuis band", ozone)
	}

	if math.Abs(twice-2*ozone) > 1e-15 {
		t.Errorf("600 DU adds %v and 300 DU adds %v; absorption is linear in the column", twice, ozone)
	}

	hazy := build(atmosphere.NewBuilder().Surface(700, 280).Aerosol(0.1, 550, 1.3, 0.9, 0.7))
	aerosol := k(hazy) - k(molecular)

	if want := magPerTau * float64(hazy.Aerosol().TauAt(lambda)); math.Abs(aerosol-want) > 1e-15 {
		t.Errorf("an aerosol optical depth of 0.1 adds %v, want %v", aerosol, want)
	}

	// Rayleigh scales with the mass of air overhead, which is what makes a
	// mountain site's extinction lower and what a single coefficient scaled
	// by height had to approximate.
	thin := build(atmosphere.NewBuilder().Surface(350, 280))
	if got, want := k(thin), k(molecular)/2; math.Abs(got-want) > 1e-15 {
		t.Errorf("half the pressure gives %v, want half the Rayleigh term, %v", got, want)
	}
}

// The ozone term is linear between the 10 nm nodes and continuous across
// them, so it neither steps nor overshoots between samples.
func TestExtinctionOzoneInterpolatesBetweenNodes(t *testing.T) {
	t.Parallel()

	without, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	with, err := atmosphere.NewBuilder().Ozone(1000).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	ozone := func(nm float64) float64 {
		t.Helper()

		a, err := with.Extinction(unit.WavelengthNM(nm))
		if err != nil {
			t.Fatalf("Extinction at %g nm: %v", nm, err)
		}

		b, err := without.Extinction(unit.WavelengthNM(nm))
		if err != nil {
			t.Fatalf("Extinction at %g nm: %v", nm, err)
		}

		return a - b
	}

	for node := 320.0; node < 1000; node += 10 {
		lo, mid, hi := ozone(node), ozone(node+5), ozone(node+10)

		if want := (lo + hi) / 2; math.Abs(mid-want) > 1e-12*math.Max(1, want) {
			t.Errorf("at %g nm the ozone term is %v, want %v, halfway between its nodes", node+5, mid, want)
		}

		if lo <= 0 {
			t.Errorf("at %g nm the ozone term is %v; ozone absorbs at every wavelength here", node, lo)
		}
	}

	// The last node is the table's last value, reached exactly.
	if end, near := ozone(1000), ozone(1000-1e-9); math.Abs(end-near) > 1e-12 {
		t.Errorf("the ozone term steps at 1000 nm: %v against %v just below", end, near)
	}
}

// Outside 320 to 1000 nm Extinction refuses rather than extrapolating a
// model nobody has validated there, and a wavelength that is not a number
// is refused with it.
func TestExtinctionRefusesWavelengthsOutsideItsModel(t *testing.T) {
	t.Parallel()

	air, err := atmosphere.NewBuilder().Ozone(300).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, nm := range []float64{319.999, 1000.001, 0, -550, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := air.Extinction(unit.WavelengthNM(nm)); !errors.Is(err, atmosphere.ErrExtinctionWavelength) {
			t.Errorf("Extinction(%v nm) = %v, want ErrExtinctionWavelength", nm, err)
		}
	}

	for _, nm := range []float64{320, 1000} {
		if k, err := air.Extinction(unit.WavelengthNM(nm)); err != nil || !(k > 0) {
			t.Errorf("Extinction(%g nm) = %v, %v; the model's own bounds are inside it", nm, k, err)
		}
	}
}

// An Atmosphere with no surface pressure, which only its zero value has, has
// no Rayleigh term to compute; Extinction says so rather than reporting the
// air as transparent.
func TestExtinctionNeedsASurfacePressure(t *testing.T) {
	t.Parallel()

	var air atmosphere.Atmosphere

	if _, err := air.Extinction(550); !errors.Is(err, atmosphere.ErrPressure) {
		t.Errorf("the zero Atmosphere's Extinction = %v, want ErrPressure", err)
	}
}
