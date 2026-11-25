package coord

import (
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// BarycentricDelay returns how much later a wavefront from target reaches the
// solar system barycenter than it reaches the observer, at ctx's instant: the
// observer's barycentric position projected on the direction of target, over
// the speed of light (the Rømer delay). It is positive when the observer is
// on target's side of the barycenter, and never more than about 8.3 minutes.
//
// An arrival the observer times at ctx's instant reaches the barycenter at
// that instant plus the delay, both in TDB, which is the BJD_TDB a transit
// ephemeris states: barycentric = ctx.Time().TDB().Add(delay).
// [Context.ArrivalAtObserver] is the inverse.
//
// target is the direction of the source at ctx's instant, proper motion
// applied by the caller. The source is taken to be infinitely far away, and
// the Shapiro delay is left out: each is below a millisecond for any star.
// Earth's barycentric position is evaluated at ctx's instant exactly, by the
// same SOFA series the transforms use, rather than read from the state they
// hold between evaluations. That state serves directions, and as a position
// it is off by the Earth's motion since it was evaluated: read for the delay,
// it was 58 ms out ten minutes after an evaluation and 4 ms between two.
func (ctx *Context) BarycentricDelay(target ICRS) unit.Duration {
	tdb1, tdb2 := ctx.t.TDB().JDParts()

	// The status reports a date outside 1900-2100, where the series is
	// extrapolated; its position is still the best this model has.
	_, earth, _ := gofaext.Epv00(tdb1, tdb2)

	observer := vector.V3(earth[0][0], earth[0][1], earth[0][2]).Add(ctx.obsVec)

	return unit.Seconds(observer.Dot(target.ToUnitVector()) * lightSecondsPerAU)
}

// ArrivalAtObserver returns the instant at which a wavefront from target
// that reaches the solar system barycenter at barycentric arrives at the
// observer, and moves ctx to it: the inverse of [Context.BarycentricDelay],
// for which ctx is the observer's site. It turns a barycentric (BJD_TDB)
// prediction into the instant to watch for it, which .UTC() renders as the
// site's clock does.
//
// The delay depends on where Earth is when the light arrives, so the
// instant is found by iteration: each step shrinks the error by the ratio of
// Earth's speed to light's, about 1e-4, and three take it below a
// nanosecond. target is as BarycentricDelay takes it.
func (ctx *Context) ArrivalAtObserver(target ICRS, barycentric epoch.Time) epoch.Time {
	bary := barycentric.TDB()
	t := bary

	for range 3 {
		ctx.SetTime(t)
		t = bary.Add(-ctx.BarycentricDelay(target))
	}

	ctx.SetTime(t)

	return t
}

// lightSecondsPerAU is how long light takes to cross one astronomical unit,
// about 499.005 seconds.
//
// var, not const, for the reason kmPerAU is one.
var lightSecondsPerAU = constants.IAU.AstronomicalUnit.Value / constants.SI2019.SpeedOfLight.Value
