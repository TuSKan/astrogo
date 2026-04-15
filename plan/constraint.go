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
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"

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

// Altitude passes if the target's altitude is >= a threshold.
type Altitude struct {
	Threshold angle.Angle
}

func (c Altitude) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	aa, err := skyAltAzOf(obj, t, site)
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
	am, err := skyAirmassOf(obj, t, site)
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
	sun := Body{
		ID:       ephemeris.Sun,
		Provider: ephemeris.Default(),
	}

	aa, err := skyAltAzOf(sun, t, site)
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

func (c MoonSep) Check(obj Observable, t time.Time, _ *Site) (Result, error) {
	pos, err := obj.Position(t)
	if err != nil {
		return Result{}, err
	}

	moon := Body{
		ID:       ephemeris.Moon,
		Provider: ephemeris.Default(),
	}
	moonPos, err := moon.Position(t)
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

func skyAltAzOf(obj Observable, t time.Time, site *Site) (*coord.AltAz, error) {
	pos, err := obj.Position(t)
	if err != nil {
		return nil, err
	}
	ctx := coord.NewContext(t, site.Location(), atmosphere.StandardAtmosphere)
	return ctx.ICRSToAltAz(pos)
}

func skyAirmassOf(obj Observable, t time.Time, site *Site) (float64, error) {
	aa, err := skyAltAzOf(obj, t, site)
	if err != nil {
		return 0, err
	}
	return atmosphere.Airmass(aa.Alt())
}
