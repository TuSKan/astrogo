package atmosphere

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// pickeringMolecular and schaeferOzone are the two airmasses as Pickering
// prints them in DIO 12 ‡1 (2002), footnote 39, written out again here from
// the text rather than taken from the package:
//
//	Xr = 1/sin(h + 244/(165 + 47*h^1.1))
//	Xo = (1 - (sin(z)/(1 + (20/6378)))^2)^-.5, where z = 90° - h
func pickeringMolecular(h float64) float64 {
	return 1 / math.Sin((h+244/(165+47*math.Pow(h, 1.1)))*math.Pi/180)
}

func schaeferOzone(h float64) float64 {
	z := (90 - h) * math.Pi / 180

	return math.Pow(1-math.Pow(math.Sin(z)/(1+20.0/6378), 2), -0.5)
}

// defaultNightAir is plan.VisibleTonight's default air at sea level: OPAC
// continental clean aerosol at Paranal's median optical depth, 258 DU.
func defaultNightAir(t *testing.T) *Atmosphere {
	t.Helper()

	air, err := ContinentalCleanAerosol(0, CleanMountainAOD550).Ozone(258).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	return air
}

// TestExtinctionTowardIsEachTermThroughItsOwnAirmass holds ExtinctionToward
// to its documented formula on the air's own optical depths (#625).
func TestExtinctionTowardIsEachTermThroughItsOwnAirmass(t *testing.T) {
	t.Parallel()

	air := defaultNightAir(t)

	rayleigh, ozone, aerosol, err := air.opticalDepths(547.8)
	if err != nil {
		t.Fatal(err)
	}

	for _, h := range []float64{0, 0.5, 1, 2, 5, 10, 30, 60, 90} {
		got, err := air.ExtinctionToward(547.8, angle.Deg(h))
		if err != nil {
			t.Fatalf("ExtinctionToward(%g°): %v", h, err)
		}

		want := 2.5 * math.Log10E * ((rayleigh+aerosol)*pickeringMolecular(h) + ozone*schaeferOzone(h))
		if math.Abs(got-want) > 1e-12 {
			t.Errorf("ExtinctionToward(%g°) = %.15g, want %.15g", h, got, want)
		}
	}
}

// TestExtinctionTowardDoesNotChargeOzoneTheMolecularAirmass is what #625
// changed. At the zenith every airmass is one and nothing differs; at the
// horizon one coefficient times the molecular airmass charged ozone 38.75
// airmasses where its shell has 12.66, and dimmed a star 0.628 mag too much
// on this air.
func TestExtinctionTowardDoesNotChargeOzoneTheMolecularAirmass(t *testing.T) {
	t.Parallel()

	air := defaultNightAir(t)

	k, err := air.Extinction(547.8)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		h, wantDiff float64
	}{
		{90, 0},
		{0, 0.628},
	} {
		x, err := Airmass(angle.Deg(c.h))
		if err != nil {
			t.Fatal(err)
		}

		toward, err := air.ExtinctionToward(547.8, angle.Deg(c.h))
		if err != nil {
			t.Fatal(err)
		}

		if diff := k*x - toward; math.Abs(diff-c.wantDiff) > 0.001 {
			t.Errorf("at %g°: k·X - ExtinctionToward = %.4f mag, want %.3f", c.h, diff, c.wantDiff)
		}
	}
}

// pickeringAerosol2km is Pickering's aerosol airmass for a 2 km scale height,
// DIO 12 ‡1 (2002), footnote 39, written out again from the text:
//
//	Xa = 1/sin(h + 20/(31 + 32*h^1.1))
func pickeringAerosol2km(h float64) float64 {
	return 1 / math.Sin((h+20/(31+32*math.Pow(h, 1.1)))*math.Pi/180)
}

// TestExtinctionTowardThroughDesertAirIsPickeringsAerosolAirmass is #630.
// ExtinctionToward charged aerosol the molecular airmass whatever its scale
// height. Through OPAC's desert air, a 2 km layer, a ray near the horizon
// spends far longer in the aerosol than that, so a low target was
// under-dimmed: by 50 aerosol airmasses at 0° and 17 at 1°. Now the aerosol
// term goes through Pickering's 2 km airmass, which is first held to the
// values in the table #630 gives for it, so a mistyped coefficient cannot
// pass on its own say-so.
func TestExtinctionTowardThroughDesertAirIsPickeringsAerosolAirmass(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ h, want float64 }{
		{0, 88.81}, {1, 43.49}, {2, 26.04}, {5, 11.27}, {10, 5.73},
	} {
		if got := pickeringAerosol2km(c.h); math.Abs(got-c.want) > 0.005 {
			t.Errorf("Pickering's 2 km aerosol airmass at %g° = %.3f, his table has %.2f", c.h, got, c.want)
		}
	}

	air, err := DesertAerosol(0, 0.3).Ozone(258).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	rayleigh, ozone, aerosol, err := air.opticalDepths(547.8)
	if err != nil {
		t.Fatal(err)
	}

	for _, h := range []float64{0, 0.5, 1, 2, 5, 10, 30, 90} {
		got, err := air.ExtinctionToward(547.8, angle.Deg(h))
		if err != nil {
			t.Fatalf("ExtinctionToward(%g°): %v", h, err)
		}

		want := 2.5 * math.Log10E *
			(rayleigh*pickeringMolecular(h) + aerosol*pickeringAerosol2km(h) + ozone*schaeferOzone(h))
		if math.Abs(got-want) > 1e-12 {
			t.Errorf("ExtinctionToward(%g°) through desert air = %.15g, want %.15g", h, got, want)
		}
	}

	// At the horizon the difference from the molecular airmass is the
	// aerosol's whole extra path: τ_aer·(Xa − Xr), 50.06 airmasses of it.
	molecular := 2.5 * math.Log10E * ((rayleigh+aerosol)*pickeringMolecular(0) + ozone*schaeferOzone(0))

	got, err := air.ExtinctionToward(547.8, angle.Deg(0))
	if err != nil {
		t.Fatal(err)
	}

	if extra := (got - molecular) / (2.5 * math.Log10E * aerosol); math.Abs(extra-50.06) > 0.01 {
		t.Errorf("desert air at 0° adds %.2f aerosol airmasses over the molecular, want 50.06", extra)
	}
}

// TestExtinctionTowardRefusesBelowTheHorizon: no airmass is defined there.
func TestExtinctionTowardRefusesBelowTheHorizon(t *testing.T) {
	t.Parallel()

	if _, err := defaultNightAir(t).ExtinctionToward(547.8, angle.Deg(-1)); !errors.Is(err, ErrBelowHorizon) {
		t.Errorf("ExtinctionToward(-1°) = %v, want ErrBelowHorizon", err)
	}
}

// TestExtinctionTowardRefusesWhatExtinctionRefuses: the same wavelength range
// and the same need for a surface pressure.
func TestExtinctionTowardRefusesWhatExtinctionRefuses(t *testing.T) {
	t.Parallel()

	if _, err := defaultNightAir(t).ExtinctionToward(1100, angle.Deg(45)); !errors.Is(err, ErrExtinctionWavelength) {
		t.Errorf("ExtinctionToward(1100 nm) = %v, want ErrExtinctionWavelength", err)
	}

	if _, err := (&Atmosphere{}).ExtinctionToward(547.8, angle.Deg(45)); !errors.Is(err, ErrPressure) {
		t.Errorf("ExtinctionToward on the zero Atmosphere = %v, want ErrPressure", err)
	}
}
