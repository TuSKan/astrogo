package plan

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
)

// defaultLimMagRamp is the default soft-ramp half-width (magnitudes) used
// by LimitingMagnitudeConstraint.ScoreMultiplier.
const defaultLimMagRamp = 0.5

// LimitingMagnitudeConstraint scores or gates targets by comparing the
// sky's limiting magnitude at the target's pointing and time against the
// magnitude the target requires to be detectable.
//
// It is built on top of a skybrightness.Engine, plan's only dependency on
// skybrightness (never a subpackage — see
// skybrightness/importgraph_test.go's TestPlanImportsOnlyCoreSkybrightness).
// The engine, its components, and its transmission model are assembled by
// the application (an example's main, or a caller's own setup) and
// injected here — plan never constructs one itself
// (docs/skybrightness.md §4).
//
// By default it is a SOFT scoring modifier: Check never rejects, and the
// observability merit is delivered through
// LimitingMagnitudeConstraint.ScoreMultiplier as a smooth, monotonic
// logistic ramp over the margin (limMag − required). Set Boolean to make
// Check a hard cutoff that fails when the margin is negative.
type LimitingMagnitudeConstraint struct {
	// Engine computes the sky's spectral radiance.
	Engine skybrightness.Engine
	// Passband is the passband the limiting magnitude is evaluated in —
	// there is no unqualified "V magnitude" any more (docs/skybrightness.md
	// §15); name the passband explicitly. A nil Passband degrades
	// Radiance/Luminance-only output — Conversion still needs one to
	// produce a meaningful magnitude.
	Passband *skybrightness.Passband
	// Grid is the spectral grid to evaluate on. The zero value substitutes
	// skybrightness.DefaultOpticalGrid().
	Grid skybrightness.SpectralGrid
	// Mode selects the Engine's operating mode.
	Mode skybrightness.Mode
	// Atmosphere is the atmospheric state to evaluate under; nil lets the
	// Engine substitute its own default for Mode (e.g.
	// ClimatologyDefaultAtmosphere under ModeClimatology).
	Atmosphere *atmosphere.State
	// Conversion turns the sky background + airmass into a limiting
	// magnitude. A nil Conversion defaults to
	// skybrightness.NewSchaeferNELM().
	Conversion skybrightness.LimitingMagModel
	// Required returns the minimum limiting magnitude needed to observe
	// the target. If nil, the target's static catalog magnitude is used;
	// targets without a known magnitude impose no requirement.
	Required func(Observable) float64
	// Ramp is the soft-ramp half-width in magnitudes for ScoreMultiplier
	// (default 0.5). Ignored when Boolean is true.
	Ramp float64
	// Boolean switches Check from a soft (never-rejecting) modifier to a
	// hard cutoff that fails when limMag < required.
	Boolean bool
}

// Compile-time assertion that LimitingMagnitudeConstraint implements
// ConstraintCtx (see the identical assertion block in constraint.go for
// why this matters: a CheckCtx signature drift drops a type out of the
// interface with no compiler error, only a missed scheduler fast path).
var _ ConstraintCtx = LimitingMagnitudeConstraint{}

// Check evaluates the constraint, building a coord.Context for (t, site).
func (c LimitingMagnitudeConstraint) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	ctx := coord.NewContext(t, site.Location(), site.Atmosphere())

	return c.CheckCtx(obj, t, site, ctx)
}

// CheckCtx evaluates the constraint using a pre-built coord.Context.
func (c LimitingMagnitudeConstraint) CheckCtx(obj Observable, t time.Time, _ *Site, ctx *coord.Context) (Result, error) {
	limMag, required, err := c.evaluate(obj, t, ctx)
	if err != nil {
		return Result{}, err
	}

	if !c.Boolean {
		// Soft mode: never gate observability; demotion happens via ScoreMultiplier.
		return Result{Pass: true, Value: limMag}, nil
	}

	pass := limMag >= required

	reason := ""
	if !pass {
		reason = fmt.Sprintf("limiting magnitude %.2f below required %.2f", limMag, required)
	}

	return Result{Pass: pass, Value: limMag, Reason: reason}, nil
}

// ScoreMultiplier returns the sky-brightness observability merit in
// [0,1]: a logistic ramp over the margin (limMag − required). It is
// monotonic — a darker sky (deeper limiting magnitude) never lowers the
// merit — and is intended to multiply a base observability score (see
// ScoreObservableSky).
func (c LimitingMagnitudeConstraint) ScoreMultiplier(obj Observable, t time.Time, _ *Site, ctx *coord.Context) (float64, error) {
	limMag, required, err := c.evaluate(obj, t, ctx)
	if err != nil {
		return 0, err
	}

	return softRamp(limMag-required, c.ramp()), nil
}

func (c LimitingMagnitudeConstraint) ramp() float64 {
	if c.Ramp > 0 {
		return c.Ramp
	}

	return defaultLimMagRamp
}

// evaluate returns the limiting magnitude at the target's pointing and
// the magnitude the target requires. A below-horizon target yields
// limMag = -Inf.
func (c LimitingMagnitudeConstraint) evaluate(obj Observable, t time.Time, ctx *coord.Context) (limMag, required float64, err error) {
	aa, err := skyAltAzCtx(obj, t, ctx)
	if err != nil {
		return 0, 0, err
	}

	required = c.requiredFor(obj)

	airmass, err := atmosphere.Airmass(aa.Alt())
	if err != nil {
		if errors.Is(err, atmosphere.ErrBelowHorizon) {
			// Below the horizon: nothing is observable.
			return math.Inf(-1), required, nil
		}

		return 0, 0, fmt.Errorf("constraint: airmass: %w", err)
	}

	grid := c.Grid
	if grid.Len() == 0 {
		grid = skybrightness.DefaultOpticalGrid()
	}

	pr, err := skybrightness.Point(context.Background(), c.Engine, skybrightness.PointQuery{
		Astro: ctx, Direction: aa, Passband: c.Passband, Mode: c.Mode, Atmosphere: c.Atmosphere, Grid: grid,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("constraint: sky brightness: %w", err)
	}

	conv := c.Conversion
	if conv == nil {
		conv = skybrightness.NewSchaeferNELM()
	}

	pbID := skybrightness.PassbandID("")
	if c.Passband != nil {
		pbID = c.Passband.ID
	}

	limMag, err = conv.LimitingMagnitude(skybrightness.LimitingMagInput{
		Passband: pbID, SkyVega: pr.Vega, SkyAB: pr.AB, Airmass: airmass,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("constraint: limiting magnitude: %w", err)
	}

	return limMag, required, nil
}

// requiredFor returns the limiting magnitude the target requires. A nil
// Required falls back to the target's static catalog magnitude; targets
// without one impose no requirement (-Inf).
func (c LimitingMagnitudeConstraint) requiredFor(obj Observable) float64 {
	if c.Required != nil {
		return c.Required(obj)
	}

	if sm, ok := obj.(StaticMagnitude); ok {
		if mag, has := sm.StaticMagnitude(); has {
			return mag
		}
	}

	return math.Inf(-1)
}

// softRamp is a logistic [0,1] ramp over margin with the given
// half-width. A width <= 0 degrades to a hard step at margin = 0.
func softRamp(margin, width float64) float64 {
	if width <= 0 {
		if margin >= 0 {
			return 1
		}

		return 0
	}

	return 1 / (1 + math.Exp(-margin/width))
}

// ScoreObservableSky scores obj with ScoreObservable and multiplies the
// result by the sky-brightness merit from c, giving a soft, monotonic
// demotion as the limiting magnitude approaches the target's requirement.
func ScoreObservableSky(
	obj Observable,
	t time.Time,
	site *Site,
	cfg *ScoreConfig,
	ctx *coord.Context,
	c LimitingMagnitudeConstraint,
	constraints ...Constraint,
) (float64, error) {
	base, err := ScoreObservable(obj, t, site, cfg, ctx, constraints...)
	if err != nil {
		return 0, err
	}

	if base == 0 {
		return 0, nil
	}

	merit, err := c.ScoreMultiplier(obj, t, site, ctx)
	if err != nil {
		return 0, err
	}

	return base * merit, nil
}
