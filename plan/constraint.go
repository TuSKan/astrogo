// Package constraint provides a system for evaluating astronomical observing constraints.
//
// It allows checking whether an observable target is suitable for observation
// at a given time and site based on criteria such as minimum altitude,
// maximum airmass, and solar/lunar positions.
package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"

	"github.com/TuSKan/astrogo/time"
)

// Result represents the outcome of a constraint check.
type Result struct {
	// Pass is true if the constraint was satisfied.
	Pass bool
	// Value is the numerical value evaluated (e.g., actual altitude).
	Value float64
	// Reason is an optional human-readable explanation of the result.
	Reason string
}

func (r Result) String() string {
	status := "FAIL"
	if r.Pass {
		status = "PASS"
	}
	if r.Reason != "" {
		return fmt.Sprintf("%s: %s (value=%.2f)", status, r.Reason, r.Value)
	}
	return fmt.Sprintf("%s (value=%.2f)", status, r.Value)
}

// Constraint defines the interface for an observing requirement.
type Constraint interface {
	// Check evaluates the constraint for a given target, time, and site.
	Check(obj Observable, t time.Time, site *Site) (Result, error)
}

// ConstraintCtx is an optional extension of Constraint that accepts a
// pre-built coord.Context. When evaluating multiple constraints at the
// same (time, site), sharing a single Context avoids redundant SOFA
// matrix computations (~91 µs per NewContext call).
//
// Constraints that implement this interface will receive cached Contexts
// in the scheduling hot path. Those that don't will fall back to Constraint.Check.
type ConstraintCtx interface {
	Constraint
	// CheckCtx is like Check but uses a pre-built coord.Context.
	CheckCtx(obj Observable, t time.Time, site *Site, ctx *coord.Context) (Result, error)
}

// Altitude passes if the target's altitude is >= a threshold.
type Altitude struct {
	Threshold angle.Angle
}

func (c Altitude) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
	return c.CheckCtx(obj, t, site, ctx)
}

// CheckCtx evaluates altitude using a pre-built coord.Context.
func (c Altitude) CheckCtx(obj Observable, t time.Time, site *Site, ctx *coord.Context) (Result, error) {
	aa, err := skyAltAzCtx(obj, t, ctx)
	if err != nil {
		return Result{}, err
	}

	val := aa.Alt().Degrees()
	thresh := c.Threshold.Degrees()
	pass := val >= thresh

	reason := ""
	if !pass {
		reason = fmt.Sprintf("altitude %.2f is below threshold %.2f", val, thresh)
	}

	return Result{
		Pass:   pass,
		Value:  val,
		Reason: reason,
	}, nil
}

// Airmass passes if the target's airmass is <= a threshold.
type Airmass struct {
	Threshold float64
}

func (c Airmass) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
	return c.CheckCtx(obj, t, site, ctx)
}

// CheckCtx evaluates airmass using a pre-built coord.Context.
func (c Airmass) CheckCtx(obj Observable, t time.Time, site *Site, ctx *coord.Context) (Result, error) {
	am, err := skyAirmassCtx(obj, t, ctx)
	if err != nil {
		if err == atmosphere.ErrBelowHorizon {
			return Result{
				Pass:   false,
				Reason: "target is below the horizon",
			}, nil
		}
		return Result{}, err
	}

	pass := am <= c.Threshold
	reason := ""
	if !pass {
		reason = fmt.Sprintf("airmass %.2f exceeds threshold %.2f", am, c.Threshold)
	}

	return Result{
		Pass:   pass,
		Value:  am,
		Reason: reason,
	}, nil
}

// Sun passes if the Sun's altitude is <= a threshold (e.g., twilight).
type Sun struct {
	Threshold angle.Angle
}

func (c Sun) Check(_ Observable, t time.Time, site *Site) (Result, error) {
	ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
	return c.CheckCtx(nil, t, site, ctx)
}

// CheckCtx evaluates Sun altitude using a pre-built coord.Context.
// The obj parameter is ignored — the Sun position is always computed internally.
func (c Sun) CheckCtx(obj Observable, t time.Time, _ *Site, ctx *coord.Context) (Result, error) {
	sun := NewTarget(catalog.Target{ID: "11", Name: "Sun", Kind: resolve.KindStar}, eph.Default())

	// If we are checking the Sun itself against a Sun constraint, don't penalize
	// (Though normally you wouldn't constrain the Sun against the Sun)
	if b, ok := obj.(Target); ok && b.Catalog.ID == "11" {
		return Result{Pass: true, Value: 0}, nil
	}

	aa, err := skyAltAzCtx(sun, t, ctx)
	if err != nil {
		return Result{}, err
	}

	val := aa.Alt().Degrees()
	thresh := c.Threshold.Degrees()
	pass := val <= thresh

	reason := ""
	if !pass {
		reason = fmt.Sprintf("sun altitude %.2f is above threshold %.2f", val, thresh)
	}

	return Result{
		Pass:   pass,
		Value:  val,
		Reason: reason,
	}, nil
}

// MoonSep passes if the angular separation between the target and
// the Moon is >= a threshold.
type MoonSep struct {
	Threshold angle.Angle
}

func (c MoonSep) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
	return c.CheckCtx(obj, ctx)
}

// CheckCtx evaluates Moon separation using a pre-built coord.Context.
func (c MoonSep) CheckCtx(obj Observable, ctx *coord.Context) (Result, error) {
	if b, ok := obj.(Target); ok && b.Catalog.ID == "10" {
		return Result{Pass: true, Value: 180}, nil
	}

	pos, err := obj.Position(ctx.Time())
	if err != nil {
		return Result{}, err
	}

	moon := NewTarget(catalog.Target{ID: "10", Name: "Moon", Kind: resolve.KindMoon}, eph.Default())
	moonPos, err := moon.Position(ctx.Time())
	if err != nil {
		return Result{}, err
	}

	sep := coord.Separation(pos, moonPos)
	val := sep.Degrees()
	thresh := c.Threshold.Degrees()
	pass := val >= thresh

	reason := ""
	if !pass {
		reason = fmt.Sprintf("moon separation %.2f is below threshold %.2f", val, thresh)
	}

	return Result{
		Pass:   pass,
		Value:  val,
		Reason: reason,
	}, nil
}

// ── Private Sky Helpers ──────────────────────────────────────────────────────

// skyAltAzCtx computes alt/az using a pre-built coord.Context.
func skyAltAzCtx(obj Observable, t time.Time, ctx *coord.Context) (coord.AltAz, error) {
	pos, err := obj.Position(t)
	if err != nil {
		return coord.AltAz{}, err
	}
	return ctx.ICRSToAltAz(pos)
}

// skyAirmassCtx computes airmass using a pre-built coord.Context.
func skyAirmassCtx(obj Observable, t time.Time, ctx *coord.Context) (float64, error) {
	aa, err := skyAltAzCtx(obj, t, ctx)
	if err != nil {
		return 0, err
	}
	return atmosphere.Airmass(aa.Alt())
}
