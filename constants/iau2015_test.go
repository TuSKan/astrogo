package constants_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

func TestIAU2015_AstronomicalUnit(t *testing.T) {
	c := constants.IAU2015.AstronomicalUnit
	testutil.AssertExact(t, "AstronomicalUnit", c.Value, 1.495_978_707e11)

	if !c.Exact {
		t.Errorf("AstronomicalUnit.Exact = false, want true")
	}
}

func TestIAU2015_MeanEarthRadius(t *testing.T) {
	c := constants.IAU2015.MeanEarthRadius
	testutil.AssertExact(t, "MeanEarthRadius", c.Value, 6_371_000.0)

	if c.Value < 6.35e6 || c.Value > 6.40e6 {
		t.Errorf("MeanEarthRadius = %v, outside plausible band [6.35e6, 6.40e6]", c.Value)
	}

	if c.Value >= constants.WGS84.SemiMajorAxis.Value {
		t.Errorf("MeanEarthRadius (volumetric mean, %v) should be less than WGS84 equatorial radius (%v)",
			c.Value, constants.WGS84.SemiMajorAxis.Value)
	}
}

func TestIAU2015_BodyRadii_Values(t *testing.T) {
	s := constants.IAU2015

	cases := []struct {
		name string
		c    constants.Constant
		want float64
	}{
		{"Sun", s.SunEquatorialRadius, 696_000_000.0},
		{"Moon", s.MoonEquatorialRadius, 1_737_400.0},
		{"Mercury", s.MercuryEquatorialRadius, 2_440_530.0},
		{"Venus", s.VenusEquatorialRadius, 6_051_800.0},
		{"Mars", s.MarsEquatorialRadius, 3_396_190.0},
		{"Jupiter", s.JupiterEquatorialRadius, 71_492_000.0},
		{"Saturn", s.SaturnEquatorialRadius, 60_268_000.0},
		{"Uranus", s.UranusEquatorialRadius, 25_559_000.0},
		{"Neptune", s.NeptuneEquatorialRadius, 24_764_000.0},
		{"Pluto", s.PlutoEquatorialRadius, 1_188_300.0},
	}

	for _, tt := range cases {
		testutil.AssertExact(t, tt.name+" equatorial radius", tt.c.Value, tt.want)
	}
}

// TestIAU2015_BodyRadii_Uncertainties checks the real published WGCCRE
// Table 4 1σ uncertainties for the eight measured body radii — verified
// live against JPL SSD's Planetary Physical Parameters
// (ssd.jpl.nasa.gov/planets/phys_par.html) and Satellite Physical
// Parameters (ssd.jpl.nasa.gov/sats/phys_par/) pages, both of which cite
// this same Table 4, not fabricated.
func TestIAU2015_BodyRadii_Uncertainties(t *testing.T) {
	s := constants.IAU2015

	cases := []struct {
		name string
		c    constants.Constant
		want float64
	}{
		{"Moon", s.MoonEquatorialRadius, 100.0},
		{"Mercury", s.MercuryEquatorialRadius, 40.0},
		{"Venus", s.VenusEquatorialRadius, 1_000.0},
		{"Mars", s.MarsEquatorialRadius, 100.0},
		{"Saturn", s.SaturnEquatorialRadius, 4_000.0},
		{"Uranus", s.UranusEquatorialRadius, 4_000.0},
		{"Neptune", s.NeptuneEquatorialRadius, 15_000.0},
		{"Pluto", s.PlutoEquatorialRadius, 1_600.0},
	}

	for _, tt := range cases {
		testutil.AssertExact(t, tt.name+" equatorial radius uncertainty", tt.c.Uncertainty, tt.want)
	}
}

// TestIAU2015_BodyRadii_StrictlyDecreasing asserts the real physical
// ordering — a single transposed digit in any radius breaks it.
func TestIAU2015_BodyRadii_StrictlyDecreasing(t *testing.T) {
	s := constants.IAU2015

	ordered := []struct {
		name  string
		value float64
	}{
		{"Sun", s.SunEquatorialRadius.Value},
		{"Jupiter", s.JupiterEquatorialRadius.Value},
		{"Saturn", s.SaturnEquatorialRadius.Value},
		{"Uranus", s.UranusEquatorialRadius.Value},
		{"Neptune", s.NeptuneEquatorialRadius.Value},
		{"Earth (WGS84)", constants.WGS84.SemiMajorAxis.Value},
		{"Venus", s.VenusEquatorialRadius.Value},
		{"Mars", s.MarsEquatorialRadius.Value},
		{"Mercury", s.MercuryEquatorialRadius.Value},
		{"Moon", s.MoonEquatorialRadius.Value},
		{"Pluto", s.PlutoEquatorialRadius.Value},
	}

	for i := 1; i < len(ordered); i++ {
		if ordered[i-1].value <= ordered[i].value {
			t.Errorf("expected %s (%v) > %s (%v)",
				ordered[i-1].name, ordered[i-1].value, ordered[i].name, ordered[i].value)
		}
	}
}

func TestIAU2015_AllMeters(t *testing.T) {
	for _, c := range constants.IAU2015.All() {
		if c.Unit != unit.Meter {
			t.Errorf("%s: Unit = %v, want unit.Meter", c.Symbol, c.Unit)
		}
	}
}

func TestIAU2015_ExactnessSplit(t *testing.T) {
	s := constants.IAU2015

	exact := []constants.Constant{s.AstronomicalUnit, s.MeanEarthRadius, s.SunEquatorialRadius, s.JupiterEquatorialRadius}
	for _, c := range exact {
		if !c.Exact {
			t.Errorf("%s: Exact = false, want true", c.Symbol)
		}

		if !strings.Contains(c.Reference, "IAU 201") {
			t.Errorf("%s: Reference %q does not mention an IAU 201x resolution", c.Symbol, c.Reference)
		}
	}

	measured := []constants.Constant{
		s.MoonEquatorialRadius, s.MercuryEquatorialRadius, s.VenusEquatorialRadius, s.MarsEquatorialRadius,
		s.SaturnEquatorialRadius, s.UranusEquatorialRadius, s.NeptuneEquatorialRadius, s.PlutoEquatorialRadius,
	}
	for _, c := range measured {
		if c.Exact {
			t.Errorf("%s: Exact = true, want false (WGCCRE-measured)", c.Symbol)
		}

		if c.Uncertainty <= 0 {
			t.Errorf("%s: Uncertainty = %v, want a real positive WGCCRE Table 4 value", c.Symbol, c.Uncertainty)
		}

		if !strings.Contains(c.Reference, "WGCCRE") {
			t.Errorf("%s: Reference %q does not mention WGCCRE", c.Symbol, c.Reference)
		}
	}
}

func TestIAU2015_Name(t *testing.T) {
	if got := constants.IAU2015.Name(); got != "IAU 2015" {
		t.Errorf("IAU2015.Name() = %q, want %q", got, "IAU 2015")
	}
}

func TestIAU_CurrentAliasPointsAtLatest(t *testing.T) {
	if constants.IAU != constants.IAU2015 {
		t.Errorf("constants.IAU does not equal constants.IAU2015 — the current-vintage alias is stale")
	}
}
