package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// slewTransition is a move between two placed blocks with no filter change,
// from one observed place to another, so the overhead is the slew alone.
func slewTransition(fromAlt, fromAz, toAlt, toAz float64) Transition {
	return Transition{
		FromBlock: &Block{ID: "from"},
		ToBlock:   &Block{ID: "to"},
		FromAltAz: coord.NewAltAz(angle.Deg(fromAlt), angle.Deg(fromAz)),
		ToAltAz:   coord.NewAltAz(angle.Deg(toAlt), angle.Deg(toAz)),
	}
}

// TestASlowSlewIsALongTimeNotARefusal: a slew time is a unit.Duration,
// float64 seconds, so a very slow slew is a long time rather than a refusal.
// Converted to a time.Duration, as it was before plan's durations became
// unit.Duration, a 180° slew below 1.95e-8 deg/s passed the type's 292 years
// and the conversion gave −292 years on amd64 and +292 on arm64 (#1121). Only
// a slew whose time is not a number of seconds is refused now: at 1e-307
// deg/s a half turn overflows to +Inf.
func TestASlowSlewIsALongTimeNotARefusal(t *testing.T) {
	t.Parallel()

	halfTurn := slewTransition(10, 0, 10, 180)

	for _, tc := range []struct {
		rate, want float64 // deg/s, s
	}{
		{2e-8, 9e9},
		{1.9e-8, 180 / 1.9e-8},
	} {
		got, err := (&BasicTransitionModel{SlewRate: unit.DegreesPerSec(tc.rate)}).Overhead(halfTurn)
		if err != nil {
			t.Fatalf("a 180° slew at %g deg/s was refused: %v", tc.rate, err)
		}

		if rel := math.Abs(got.Seconds()-tc.want) / tc.want; rel > 1e-12 {
			t.Errorf("a 180° slew at %g deg/s takes %g s, want %g s", tc.rate, got.Seconds(), tc.want)
		}
	}

	if d, err := (&BasicTransitionModel{SlewRate: unit.DegreesPerSec(1e-307)}).Overhead(halfTurn); !errors.Is(err, ErrSlewTime) {
		t.Errorf("a 180° slew at 1e-307 deg/s, an infinite time, gave %v and %v, want %v", d, err, ErrSlewTime)
	}
}

// TestSlewTimeRefusesNaN: a NaN slew rate used to fail SlewRate > 0 and drop
// the slew as though none were configured, and a NaN slew distance reached
// the same undefined conversion as an overflow (#1121).
func TestSlewTimeRefusesNaN(t *testing.T) {
	t.Parallel()

	nanRate := &BasicTransitionModel{SlewRate: unit.AngularRate(math.NaN())}
	if d, err := nanRate.Overhead(slewTransition(10, 0, 10, 90)); !errors.Is(err, ErrSlewTime) {
		t.Errorf("a NaN slew rate gave %v and %v, want %v", d, err, ErrSlewTime)
	}

	nanPlace := slewTransition(10, 0, math.NaN(), 90)
	if d, err := (&BasicTransitionModel{SlewRate: unit.DegreesPerSec(2)}).Overhead(nanPlace); !errors.Is(err, ErrSlewTime) {
		t.Errorf("a NaN altitude gave %v and %v, want %v", d, err, ErrSlewTime)
	}
}

// TestSchedulerReportsAnInfiniteSlew: the refusal reaches the caller of a
// schedule, as every overhead failure does since #454, rather than placing or
// dropping the block.
func TestSchedulerReportsAnInfiniteSlew(t *testing.T) {
	t.Parallel()

	planner, window, b1, b2 := overheadFixture(t)
	b2.Target = NewStar("B, a quarter turn away", angle.Deg(90), angle.Zero())

	slow := &BasicTransitionModel{SlewRate: unit.DegreesPerSec(1e-307)}

	sched, err := NewScheduler(planner, &GreedyStrategy{}, slow).BuildSchedule(window, []*Block{b1, b2})
	if !errors.Is(err, ErrSlewTime) {
		t.Fatalf("BuildSchedule returned %v (schedule %v), want %v", err, sched, ErrSlewTime)
	}
}
