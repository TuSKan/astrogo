package skybrightness_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// reportingComponent adds a constant radiance and reports a fixed
// Contribution, so the budget's plumbing can be tested without any physics.
type reportingComponent struct {
	id           skybrightness.ComponentID
	value        float64
	contribution skybrightness.Contribution
}

func (c reportingComponent) ID() skybrightness.ComponentID { return c.id }

func (c reportingComponent) AddRadiance(
	_ context.Context,
	dst skybrightness.SpectralRadiance,
	_ unit.SpectralGrid,
	_ coord.AltAz,
	_ *skybrightness.Scene,
) (skybrightness.Contribution, error) {
	for i := range dst {
		dst[i] += c.value
	}

	return c.contribution, nil
}

func (c reportingComponent) Provenance() skybrightness.Provenance {
	return skybrightness.Provenance{Model: "reporting test component"}
}

func estimateOf(t *testing.T, components ...skybrightness.Component) (*skybrightness.Estimate, error) {
	t.Helper()

	m, err := skybrightness.NewModel("test", components...)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	est, err := m.Estimate(context.Background(), skybrightness.Query{Scene: testScene(t), Direction: zenith()})
	if err != nil {
		return nil, fmt.Errorf("estimate: %w", err)
	}

	return est, nil
}

// TestEstimateFillsTheBudgetFromContributions: what a component reports is
// what the budget holds, and the total combines it (#911). A component that
// reports nothing leaves its uncertainty unknown, and the total says so.
func TestEstimateFillsTheBudgetFromContributions(t *testing.T) {
	t.Parallel()

	known := reportingComponent{id: skybrightness.Zodiacal, value: 2e-9, contribution: skybrightness.Contribution{
		Relative: 0.1, Source: "test source", Known: true,
	}}
	silent := reportingComponent{id: skybrightness.AirglowContinuum, value: 1e-9}

	est, err := estimateOf(t, known, silent)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	if got := est.Uncertainty.Components; len(got) != 1 || got[0] != (skybrightness.Uncertainty{
		Component: skybrightness.Zodiacal, Relative: 0.1, Source: "test source",
	}) {
		t.Fatalf("budget = %+v, want only the zodiacal entry", got)
	}

	if _, err := est.TotalUncertainty(); !errors.Is(err, skybrightness.ErrUncertaintyUnknown) ||
		!strings.Contains(err.Error(), "airglow-continuum") {
		t.Errorf("TotalUncertainty error = %v, want ErrUncertaintyUnknown naming airglow-continuum", err)
	}
}

// TestEstimateRefusesAnInvalidUncertainty: a negative or non-finite relative
// uncertainty is a defect in the component, and it would otherwise reach every
// total silently.
func TestEstimateRefusesAnInvalidUncertainty(t *testing.T) {
	t.Parallel()

	for _, r := range []float64{-0.1, math.NaN(), math.Inf(1)} {
		c := reportingComponent{id: skybrightness.Zodiacal, value: 1e-9, contribution: skybrightness.Contribution{
			Relative: r, Known: true,
		}}

		if _, err := estimateOf(t, c); !errors.Is(err, skybrightness.ErrComponentFailed) {
			t.Errorf("relative %v: err = %v, want ErrComponentFailed", r, err)
		}
	}
}

// TestWithUncertaintySuppliesTheCallersFigure: a caller-supplied uncertainty
// reaches the budget for a component that reports none, replaces one that
// does, and leaves the radiance and flags alone.
func TestWithUncertaintySuppliesTheCallersFigure(t *testing.T) {
	t.Parallel()

	silent := reportingComponent{id: skybrightness.AirglowContinuum, value: 1e-9, contribution: skybrightness.Contribution{
		Flags: skybrightness.AssumedSourceSpectrum,
	}}
	own := reportingComponent{id: skybrightness.Zodiacal, value: 3e-9, contribution: skybrightness.Contribution{
		Relative: 0.1, Source: "own", Known: true,
	}}

	airglow, err := skybrightness.WithUncertainty(silent, 0.3, "night-to-night variability")
	if err != nil {
		t.Fatalf("WithUncertainty: %v", err)
	}

	zodiacal, err := skybrightness.WithUncertainty(own, 0.05, "measured")
	if err != nil {
		t.Fatalf("WithUncertainty: %v", err)
	}

	est, err := estimateOf(t, airglow, zodiacal)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	total, err := est.TotalUncertainty()
	if err != nil {
		t.Fatalf("TotalUncertainty: %v", err)
	}

	// Shares 1/4 and 3/4, in quadrature.
	if want := math.Hypot(0.25*0.3, 0.75*0.05); math.Abs(total-want) > 1e-15 {
		t.Errorf("TotalUncertainty = %.17g, want %.17g", total, want)
	}

	if !est.Quality.Has(skybrightness.AssumedSourceSpectrum) {
		t.Error("the wrapped component's flag was lost")
	}

	if got := airglow.ID(); got != skybrightness.AirglowContinuum {
		t.Errorf("wrapped ID = %q, want the component's own", got)
	}
}

// TestWithUncertaintyRefusesWhatCannotBeAFigure: no component, or a figure
// that is negative or not finite.
func TestWithUncertaintyRefusesWhatCannotBeAFigure(t *testing.T) {
	t.Parallel()

	c := reportingComponent{id: skybrightness.Starlight, value: 1e-9}

	for _, tc := range []struct {
		name     string
		c        skybrightness.Component
		relative float64
	}{
		{"no component", nil, 0.1},
		{"negative", c, -0.1},
		{"NaN", c, math.NaN()},
		{"infinite", c, math.Inf(1)},
	} {
		if _, err := skybrightness.WithUncertainty(tc.c, tc.relative, "x"); !errors.Is(err, skybrightness.ErrUncertaintyValue) {
			t.Errorf("%s: err = %v, want ErrUncertaintyValue", tc.name, err)
		}
	}
}
