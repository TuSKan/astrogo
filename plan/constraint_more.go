package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// Four more constraints, each a few lines against the existing interface and
// each answering a question somebody actually asks a scheduler. See #129 for
// the inventory they come from, and for the two that are not here.
//
// PhaseConstraint is absent because [Observable] carries no ephemeris — an
// epoch and a period — so a periodic target cannot be expressed at all yet;
// that is a type change rather than a constraint. LocalTime is absent because
// [TimeWindow] answers the same question in the scale the rest of this package
// works in, and a local-clock window is a display convention rather than a
// physical one.

var (
	_ ConstraintCtx = SunSep{}
	_ ConstraintCtx = GalacticLatitude{}
	_ ConstraintCtx = TimeWindow{}
	_ ConstraintCtx = Horizon{}
)

// SunSep passes if the angular separation between the target and the Sun is
// at or above a threshold.
//
// The companion to [MoonSep] for the other bright source, and the one that
// decides whether a daytime or twilight observation is possible at all: scattered
// sunlight near the Sun defeats any exposure, and a solar-adjacent pointing is
// an instrument-safety question before it is a photometric one.
//
// A target that *is* the Sun passes automatically, on the same reasoning
// [MoonSep] applies to the Moon: its separation from itself is not a
// constraint on observing it.
type SunSep struct {
	// Threshold is the minimum acceptable separation.
	Threshold angle.Angle

	// Provider supplies the Sun's position. Nil means [eph.Default], the
	// convention [MoonIllum] documents and this package has settled on.
	Provider eph.Provider
}

// Check evaluates Sun separation for a given target and time.
func (c SunSep) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	ctx := coord.NewContext(t, site.Location(), site.Refraction())
	return c.CheckCtx(obj, t, site, ctx)
}

// CheckCtx evaluates Sun separation using a pre-built coord.Context.
func (c SunSep) CheckCtx(obj Observable, _ time.Time, _ *Site, ctx *coord.Context) (Result, error) {
	if p, ok := obj.(*Planet); ok && p.EphID() == eph.Sun {
		return Result{Pass: true, Value: 180}, nil
	}

	pos, err := obj.Position(ctx.Time())
	if err != nil {
		return Result{}, fmt.Errorf("constraint: sun separation position: %w", err)
	}

	sunPos, err := NewSun(c.Provider).Position(ctx.Time())
	if err != nil {
		return Result{}, fmt.Errorf("constraint: sun position: %w", err)
	}

	val := coord.Separation(pos, sunPos).Degrees()
	thresh := c.Threshold.Degrees()
	pass := val >= thresh

	reason := ""
	if !pass {
		reason = fmt.Sprintf("sun separation %.2f is below threshold %.2f", val, thresh)
	}

	return Result{Pass: pass, Value: val, Reason: reason}, nil
}

// GalacticLatitude passes if the target lies at least Threshold from the
// Galactic plane.
//
// Which way this cuts depends on the programme, and that is why it is stated
// as a distance from the plane rather than as a latitude range. An
// extragalactic survey avoids the plane because stellar crowding and dust
// extinction there are severe; a Galactic-structure programme wants exactly
// that region and expresses it by inverting the sense of the constraint in
// its own scoring rather than by a second type here.
//
// The value reported is |b| in degrees, so it is comparable across both uses.
type GalacticLatitude struct {
	// Threshold is the minimum acceptable |b|.
	Threshold angle.Angle
}

// Check evaluates Galactic latitude for a given target and time.
func (c GalacticLatitude) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	ctx := coord.NewContext(t, site.Location(), site.Refraction())
	return c.CheckCtx(obj, t, site, ctx)
}

// CheckCtx evaluates Galactic latitude using a pre-built coord.Context.
//
// The Context supplies the epoch and nothing else: the rotation from ICRS to
// Galactic is a fixed frame rotation with no observer and no time in it. A
// moving target still moves, which is why the position is taken at the
// Context's own instant rather than once.
func (c GalacticLatitude) CheckCtx(
	obj Observable, _ time.Time, _ *Site, ctx *coord.Context,
) (Result, error) {
	pos, err := obj.Position(ctx.Time())
	if err != nil {
		return Result{}, fmt.Errorf("constraint: galactic latitude position: %w", err)
	}

	b := coord.ICRSToGalactic(pos).B().Degrees()

	val := math.Abs(b)
	thresh := c.Threshold.Degrees()
	pass := val >= thresh

	reason := ""
	if !pass {
		reason = fmt.Sprintf("galactic latitude |b| %.2f is below threshold %.2f", val, thresh)
	}

	return Result{Pass: pass, Value: val, Reason: reason}, nil
}

// TimeWindow passes while the evaluated instant lies within [From, To],
// inclusive at both ends.
//
// The constraint that has nothing to do with the sky: a target of opportunity
// with a coordination window, an instrument available for part of the night, a
// programme whose allocation ends at a stated time. Without it those have to be
// expressed by slicing the schedule's own span, which conflates "this target
// cannot be observed then" with "nobody observes then".
//
// Zero From or To means unbounded on that side, so a one-sided deadline needs
// only the field it is about.
type TimeWindow struct {
	// From is the earliest acceptable instant. Zero means no lower bound.
	From time.Time

	// To is the latest acceptable instant. Zero means no upper bound.
	To time.Time
}

// Check evaluates the window for a given time.
func (c TimeWindow) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	ctx := coord.NewContext(t, site.Location(), site.Refraction())
	return c.CheckCtx(obj, t, site, ctx)
}

// CheckCtx evaluates the window using the Context's instant.
//
// Value is hours from the nearer edge: positive inside the window, negative
// outside, so a scorer can rank "well inside" above "about to close" without a
// second call. A window unbounded on both sides reports zero, since there is no
// edge to measure from.
func (c TimeWindow) CheckCtx(_ Observable, _ time.Time, _ *Site, ctx *coord.Context) (Result, error) {
	t := ctx.Time()

	beforeStart := !c.From.IsZero() && t.Before(c.From)
	afterEnd := !c.To.IsZero() && t.After(c.To)

	switch {
	case beforeStart:
		return Result{
			Value:  -c.From.Sub(t).Hours(),
			Reason: fmt.Sprintf("%.2f h before the window opens", c.From.Sub(t).Hours()),
		}, nil

	case afterEnd:
		return Result{
			Value:  -t.Sub(c.To).Hours(),
			Reason: fmt.Sprintf("%.2f h after the window closed", t.Sub(c.To).Hours()),
		}, nil
	}

	return Result{Pass: true, Value: hoursToNearerEdge(t, c.From, c.To)}, nil
}

// hoursToNearerEdge is how much of the window is left on the tighter side.
func hoursToNearerEdge(t, from, to time.Time) float64 {
	switch {
	case from.IsZero() && to.IsZero():
		return 0
	case from.IsZero():
		return to.Sub(t).Hours()
	case to.IsZero():
		return t.Sub(from).Hours()
	}

	return math.Min(t.Sub(from).Hours(), to.Sub(t).Hours())
}

// Horizon passes if the target stands above the local terrain at its own
// azimuth.
//
// The constraint [Altitude] cannot express. A single threshold describes a site
// with a clean horizon in every direction, and almost no real site has one: a
// ridge to the east delays every rise, a dome or a building blocks a sector
// outright, and a target at 15 degrees is observable in one azimuth and behind
// rock in another.
//
// The profile comes from the [Site], through [Site.HorizonAt], which is where
// it belongs — a horizon is a property of the place, not of the question being
// asked about it. [WithHorizonProfile] sets one; a site without one answers
// with its scalar [Site.Horizon] at every azimuth, so this constraint is
// meaningful either way and degrades to [Altitude] rather than to nothing.
//
// Where the profile comes from is the caller's problem and there are several
// answers — a surveyed table, a digital elevation model, a fisheye photograph.
// [HorizonProfile] is one function, so any of them fits behind it.
type Horizon struct {
	// Margin is added to the horizon before comparing, for the clearance a
	// caller wants above the ridge line rather than exactly on it. Zero means
	// the ridge itself.
	Margin angle.Angle
}

// Check evaluates the terrain horizon for a given target and time.
func (c Horizon) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	ctx := coord.NewContext(t, site.Location(), site.Refraction())
	return c.CheckCtx(obj, t, site, ctx)
}

// CheckCtx evaluates the terrain horizon using a pre-built coord.Context.
//
// The site is read here rather than captured in the struct, which is why this
// is the one constraint in the file that uses its site parameter: two sites can
// share a constraint set and must not share a horizon.
func (c Horizon) CheckCtx(
	obj Observable, t time.Time, site *Site, ctx *coord.Context,
) (Result, error) {
	aa, err := skyAltAzCtx(obj, t, ctx)
	if err != nil {
		return Result{}, err
	}

	terrain := site.HorizonAt(aa.Az()).Degrees() + c.Margin.Degrees()
	alt := aa.Alt().Degrees()

	// Clearance rather than altitude, so the value means the same thing in
	// every azimuth — which is the whole reason this is not Altitude.
	val := alt - terrain
	pass := val >= 0

	reason := ""
	if !pass {
		reason = fmt.Sprintf("altitude %.2f is %.2f below the horizon at azimuth %.1f",
			alt, -val, aa.Az().Degrees())
	}

	return Result{Pass: pass, Value: val, Reason: reason}, nil
}
