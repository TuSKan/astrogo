package plan

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
)

// defaultLimMagRamp is the default soft-ramp half-width (magnitudes) used by
// LimitingMagnitudeConstraint.ScoreMultiplier.
const defaultLimMagRamp = 0.5

// LimitingMagnitudeConstraint scores or gates targets by comparing the sky's
// limiting magnitude at the target's pointing and time against the magnitude
// the target requires to be detectable.
//
// By default it is a SOFT scoring modifier: Check never rejects, and the
// observability merit is delivered through [LimitingMagnitudeConstraint.ScoreMultiplier]
// as a smooth, monotonic logistic ramp over the margin (limMag − required).
// Set Boolean to make Check a hard cutoff that fails when the margin is negative.
type LimitingMagnitudeConstraint struct {
	// Model supplies the total sky surface brightness toward the pointing.
	Model skybrightness.Model
	// Conversion turns sky brightness + airmass into a limiting magnitude.
	Conversion skybrightness.LimitingMagModel
	// Required returns the minimum limiting magnitude needed to observe the
	// target. If nil, the target's static catalog magnitude is used; targets
	// without a known magnitude impose no requirement.
	Required func(Observable) float64
	// Ramp is the soft-ramp half-width in magnitudes for ScoreMultiplier
	// (default 0.5). Ignored when Boolean is true.
	Ramp float64
	// Boolean switches Check from a soft (never-rejecting) modifier to a hard
	// cutoff that fails when limMag < required.
	Boolean bool
}

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

// ScoreMultiplier returns the sky-brightness observability merit in [0,1]: a
// logistic ramp over the margin (limMag − required). It is monotonic — a darker
// sky (deeper limiting magnitude) never lowers the merit — and is intended to
// multiply a base observability score (see [ScoreObservableSky]).
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

// evaluate returns the limiting magnitude at the target's pointing and the
// magnitude the target requires. A below-horizon target yields limMag = −Inf.
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

	sky, err := c.Model.SurfaceBrightness(aa, ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("constraint: sky brightness: %w", err)
	}

	limMag, err = c.Conversion.LimitingMagnitude(sky, airmass)
	if err != nil {
		return 0, 0, fmt.Errorf("constraint: limiting magnitude: %w", err)
	}

	return limMag, required, nil
}

// requiredFor returns the limiting magnitude the target requires. A nil Required
// falls back to the target's static catalog magnitude; targets without one
// impose no requirement (−Inf).
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

// softRamp is a logistic [0,1] ramp over margin with the given half-width. A
// width <= 0 degrades to a hard step at margin = 0.
func softRamp(margin, width float64) float64 {
	if width <= 0 {
		if margin >= 0 {
			return 1
		}

		return 0
	}

	return 1 / (1 + math.Exp(-margin/width))
}

// ScoreObservableSky scores obj with [ScoreObservable] and multiplies the result
// by the sky-brightness merit from c, giving a soft, monotonic demotion as the
// limiting magnitude approaches the target's requirement.
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
