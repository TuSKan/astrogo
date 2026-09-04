package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// defaultLimMagRamp is the soft-ramp half-width, in magnitudes, that
// [LimitingMagnitudeConstraint.ScoreMultiplier] uses when Ramp is zero.
//
// Half a magnitude: wide enough that a target sitting exactly at the limit
// scores near the middle rather than snapping between 0 and 1 on a hundredth
// of a magnitude of sky, and narrow enough that a target two magnitudes clear
// of the limit is not still being penalised for it.
const defaultLimMagRamp = 0.5

// SkyDepth reports how faint a source can be at a pointing and an instant and
// still be detected.
//
// # Why this interface is declared here rather than imported
//
// Because it is the whole of what a constraint needs, and declaring it here
// means the planning engine depends on no sky-brightness code at all — not
// the engine, not its datasets, not a photometric system. An earlier version
// of this constraint took a concrete sky-brightness type and needed a bespoke
// import test to keep that dependency from spreading; this needs no test,
// because there is no import to police.
//
// It also leaves the hard part where it belongs. A limiting magnitude is not
// a property of the sky: it is a threshold against the sky, and it depends on
// the detector or the eye, the exposure, the aperture and the observer. An
// implementation has to decide all of that, and different observers
// legitimately decide differently.
// [github.com/TuSKan/astrogo/skybrightness/plan] supplies one for an imaging
// system; a visual observer's is a different model and is not written yet.
type SkyDepth interface {
	// LimitingMagnitudeAt returns the faintest detectable magnitude looking
	// at (alt, az) at time t. A brighter sky gives a smaller number.
	LimitingMagnitudeAt(t time.Time, alt, az angle.Angle) (float64, error)
}

// LimitingMagnitudeConstraint scores or gates targets by comparing how deep
// the sky is at the target's pointing against the magnitude that target needs
// in order to be detected.
//
// # Soft by default
//
// [LimitingMagnitudeConstraint.Check] never rejects unless Boolean is set.
// Sky brightness is a matter of degree in a way an altitude limit is not —
// half a magnitude of moonlight makes a target harder rather than impossible
// — so the merit is delivered through
// [LimitingMagnitudeConstraint.ScoreMultiplier] as a smooth monotonic ramp
// over the margin, and a hard cutoff is available for a caller who genuinely
// wants one.
//
// # What it will not do
//
// Construct a sky. Sky, the instrument behind it and the detection threshold
// it assumes are all assembled by the application and injected, because every
// one of them is a decision this package cannot make on a caller's behalf.
type LimitingMagnitudeConstraint struct {
	// Sky is how deep the sky is. Required.
	Sky SkyDepth

	// Required reports the magnitude a target must reach to count as
	// detected, and whether it has one at all.
	//
	// Nil falls back to the target's catalog magnitude via
	// [StaticMagnitude]. A target with neither imposes no requirement and
	// always passes, which is deliberate: a constraint that rejected
	// everything it could not measure would silently empty a target list.
	Required func(obj Observable) (float64, bool)

	// Ramp is the soft-ramp half-width in magnitudes. Zero means 0.5.
	Ramp float64

	// Boolean makes Check a hard cutoff that fails when the target is
	// fainter than the sky allows.
	Boolean bool
}

// Compile-time assertion that this implements ConstraintCtx. See the
// identical block in constraint.go for why it matters: a CheckCtx signature
// drift drops the type out of the interface with no compiler error, and the
// only symptom is a missed scheduler fast path at runtime.
var _ ConstraintCtx = LimitingMagnitudeConstraint{}

// Check evaluates the constraint, building a coord.Context for (t, site).
func (c LimitingMagnitudeConstraint) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	ctx := coord.NewContext(t, site.Location(), site.Refraction())

	return c.CheckCtx(obj, t, site, ctx)
}

// CheckCtx evaluates the constraint using a pre-built coord.Context.
func (c LimitingMagnitudeConstraint) CheckCtx(
	obj Observable, _ time.Time, _ *Site, ctx *coord.Context,
) (Result, error) {
	limMag, required, err := c.evaluate(obj, ctx)
	if err != nil {
		return Result{}, err
	}

	margin := limMag - required

	// Nothing to require: a target with no photometry is not evidence of a
	// bad sky.
	if math.IsInf(required, -1) {
		return Result{Pass: true, Value: math.Inf(1)}, nil
	}

	if !c.Boolean {
		return Result{Pass: true, Value: margin}, nil
	}

	if margin >= 0 {
		return Result{Pass: true, Value: margin}, nil
	}

	return Result{
		Pass:  false,
		Value: margin,
		Reason: fmt.Sprintf("the sky reaches %.2f and the target needs %.2f, short by %.2f mag",
			limMag, required, -margin),
	}, nil
}

// ScoreMultiplier returns the sky-brightness merit in [0,1]: a logistic ramp
// over the margin between what the sky reaches and what the target needs.
//
// Monotonic by construction — a deeper sky never lowers the merit — and
// intended to multiply a base observability score rather than replace one.
func (c LimitingMagnitudeConstraint) ScoreMultiplier(
	obj Observable, ctx *coord.Context,
) (float64, error) {
	limMag, required, err := c.evaluate(obj, ctx)
	if err != nil {
		return 0, err
	}

	if math.IsInf(required, -1) {
		return 1, nil
	}

	margin := limMag - required
	if math.IsInf(margin, -1) {
		return 0, nil
	}

	return 1 / (1 + math.Exp(-margin/c.ramp())), nil
}

// ramp is the configured half-width, or the default.
func (c LimitingMagnitudeConstraint) ramp() float64 {
	if c.Ramp > 0 && !math.IsInf(c.Ramp, 0) {
		return c.Ramp
	}

	return defaultLimMagRamp
}

// evaluate resolves how deep the sky is where the target is, and how deep it
// needs to be.
//
// A target below the horizon yields -Inf: it is not that the sky there is
// bright, it is that there is no sky there. Returning a finite number would
// let a below-horizon target out-score a visible one under a bright sky.
func (c LimitingMagnitudeConstraint) evaluate(
	obj Observable, ctx *coord.Context,
) (limMag, required float64, err error) {
	if c.Sky == nil {
		return 0, 0, fmt.Errorf("constraint: limiting magnitude: %w", ErrNoSkyDepth)
	}

	pos, err := obj.Position(ctx.Time())
	if err != nil {
		return 0, 0, fmt.Errorf("constraint: limiting magnitude position: %w", err)
	}

	altaz, err := observedAltAz(obj, ctx.Time(), ctx, pos)
	if err != nil {
		return 0, 0, fmt.Errorf("constraint: limiting magnitude pointing: %w", err)
	}

	if altaz.Alt() <= 0 {
		return math.Inf(-1), c.requiredFor(obj, ctx), nil
	}

	limMag, err = c.Sky.LimitingMagnitudeAt(ctx.Time(), altaz.Alt(), altaz.Az())
	if err != nil {
		return 0, 0, fmt.Errorf("constraint: limiting magnitude: %w", err)
	}

	return limMag, c.requiredFor(obj, ctx), nil
}

// requiredFor is the magnitude a target must reach, or -Inf when it declares
// none.
func (c LimitingMagnitudeConstraint) requiredFor(obj Observable, ctx *coord.Context) float64 {
	if c.Required != nil {
		if m, ok := c.Required(obj); ok {
			return m
		}

		return math.Inf(-1)
	}

	// The time-varying magnitude first, since a planet's brightness is not a
	// catalog constant and using one would be wrong by magnitudes.
	if mc, ok := obj.(MagnitudeComputer); ok {
		if m, err := mc.ApparentMagnitudeCtx(ctx.Time(), ctx); err == nil {
			return m
		}
	}

	if sm, ok := obj.(StaticMagnitude); ok {
		if m, has := sm.StaticMagnitude(); has {
			return m
		}
	}

	return math.Inf(-1)
}
