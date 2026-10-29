package skybrightness

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// The sourced uncertainties of #911. Each is checked against a figure worked
// from its paper by hand, or against a limit the formula must reach, rather
// than against a second copy of the formula.

// TestZodiacalUncertaintyFollowsLeinert: max(10%, 15 S10 / brightness) in the
// visual, with a 20% floor below 300 nm. 15 S10 is 19.2 in Table 17's units.
func TestZodiacalUncertaintyFollowsLeinert(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		table  float64
		lambda unit.WavelengthNM
		want   float64
	}{
		// The faintest cells, 56 S10, are 71.7 in Table 17: "about 27 per
		// cent", as the provenance says.
		{"faintest cell", 56 * 1.28, 550, 19.2 / (56 * 1.28)},
		// Where 19.2 is exactly a tenth, the two terms meet.
		{"crossover", 192, 550, 0.10},
		{"bright ecliptic", 1000, 550, 0.10},
		{"bright, ultraviolet", 1000, 250, 0.20},
		{"faint, ultraviolet", 56 * 1.28, 250, 19.2 / (56 * 1.28)},
	} {
		if got := zodiacalRelativeUncertainty(tc.table, tc.lambda); math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("%s: %.6f, want %.6f", tc.name, got, tc.want)
		}
	}

	if got := zodiacalRelativeUncertainty(56*1.28, 550); math.Round(100*got) != 27 {
		t.Errorf("the faintest cell carries %.1f per cent; the provenance says about 27", 100*got)
	}
}

// TestDGLUncertaintyWorkedExample: at 550 nm, a row of Kawara's Table 2, and
// an SFD intensity of 5.8 MJy/sr, so I = 5:
//
//	b = 20.1 * 0.55 / 3000 = 0.003685, sigma_b = 0.6 * 0.55 / 3000 = 0.00011
//	c = 4.4e-5, sigma_c = 0.4e-5
//	D = b*I - c*I^2 = 0.017325, dD/dI = b - 2cI = 0.003245
//	sigma_D^2 = 25(0.00011)^2 + 625(0.4e-5)^2 + 0.003245^2 * 0.21^2
//	          = 3.025e-7 + 1.0e-8 + 4.6437e-7 = 7.7687e-7
//	sigma_D / D = 8.8141e-4 / 0.017325 = 0.050875
func TestDGLUncertaintyWorkedExample(t *testing.T) {
	t.Parallel()

	got, ok := dglRelativeUncertainty(550, 5.8)
	if !ok {
		t.Fatal("no uncertainty at I = 5 MJy/sr, well inside the fitted range")
	}

	if math.Abs(got-0.050875) > 5e-6 {
		t.Errorf("relative uncertainty %.6f, want the hand-worked 0.050875", got)
	}
}

// TestDGLUncertaintyAtTheEdgesOfTheFit: just above the offset the offset's own
// uncertainty dominates, so sigma_D/D tends to 0.21/I; outside the fitted
// regime there is no figure.
func TestDGLUncertaintyAtTheEdgesOfTheFit(t *testing.T) {
	t.Parallel()

	const i100 = 0.001

	got, ok := dglRelativeUncertainty(550, DustEmissionOffsetMJy+i100)
	if !ok || math.Abs(got*i100-dglOffsetUncertaintyMJy) > 0.01*dglOffsetUncertaintyMJy {
		t.Errorf("at I = %g: relative %v (known %t), want about %.0f", i100, got, ok, dglOffsetUncertaintyMJy/i100)
	}

	// At 550 nm the quadratic turns over at b/2c = 41.9 MJy/sr.
	for _, sfd := range []float64{0.5, DustEmissionOffsetMJy, 0.8 + 45, MaxDustIntensityMJy + 5} {
		if _, ok := dglRelativeUncertainty(550, sfd); ok {
			t.Errorf("SFD %v MJy/sr is outside the fitted regime, yet an uncertainty was given", sfd)
		}
	}
}

// TestMoonlightUncertaintyMatchesItsStatedSize: the doc comment's 16 per cent
// in V at Paranal's pressure and 18 at sea level, below Jones et al.'s 0.2 mag.
func TestMoonlightUncertaintyMatchesItsStatedSize(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		pressure float64
		want     float64
	}{{743, 16}, {1013.25, 18}} {
		got, err := moonlightRelativeUncertainty(550, tc.pressure)
		if err != nil {
			t.Fatalf("%v hPa: %v", tc.pressure, err)
		}

		if math.Round(100*got) != tc.want {
			t.Errorf("%v hPa: %.1f per cent in V, but the doc comment says %v", tc.pressure, 100*got, tc.want)
		}

		if jones := math.Pow(10, 0.4*0.2) - 1; got >= jones {
			t.Errorf("%v hPa: %.1f per cent is not below Jones et al.'s 0.2 mag (%.1f)", tc.pressure, 100*got, 100*jones)
		}
	}

	// With no multiple scattering left to disagree about, ROLO's scale is all
	// that remains.
	if got, _ := moonlightRelativeUncertainty(2000, 1e-6); math.Abs(got-roloAbsoluteScale) > 1e-6 {
		t.Errorf("with no Rayleigh depth: %v, want ROLO's %v alone", got, roloAbsoluteScale)
	}
}

// TestExtragalacticUncertaintyIsKoushans: through AddRadiance, the figure is
// Koushan et al.'s total error weighted over the grid, so it lies within the
// range their Table 3 spans and names its source.
func TestExtragalacticUncertaintyIsKoushans(t *testing.T) {
	t.Parallel()

	grid := DefaultOpticalGrid()
	dst := NewSpectralRadiance(grid)

	contribution, err := NewExtragalacticBackground().AddRadiance(context.Background(), dst, grid,
		coord.NewAltAz(angle.Deg(60), angle.Deg(0)), eblScene(t))
	if err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	if !contribution.Known || contribution.Relative < 0.0402 || contribution.Relative > 0.0687 ||
		!strings.Contains(contribution.Source, "Koushan") {
		t.Errorf("contribution = %+v, want a known figure within Table 3's 4.02-6.87 per cent", contribution)
	}

	// Below the horizon nothing is added, and nothing is claimed.
	below, err := NewExtragalacticBackground().AddRadiance(context.Background(), NewSpectralRadiance(grid), grid,
		coord.NewAltAz(angle.Deg(-5), angle.Deg(0)), eblScene(t))
	if err != nil || below.Known {
		t.Errorf("below the horizon: %+v, %v; want nothing claimed", below, err)
	}
}
