package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
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

// TestSlewTimeAtTheEdgeOfADuration is #1121. A time.Duration holds fewer
// than 2^63 nanoseconds, about 292 years, and a 180° slew takes longer below
// 1.95e-8 deg/s. Past that, converting the float is not defined by the
// language: amd64 gave -292 years and arm64 +292, so the same model placed
// the next block before this one ended on one platform and never on the
// other. A slew a Duration cannot hold is now refused on both.
//
// The rates sit well to either side of the edge, 9e9 s and 9.47e9 s against
// 9.22e9, rather than at it, where rounding decides.
func TestSlewTimeAtTheEdgeOfADuration(t *testing.T) {
	t.Parallel()

	halfTurn := slewTransition(10, 0, 10, 180)

	inside, err := (&BasicTransitionModel{SlewRate: unit.DegreesPerSec(2e-8)}).Overhead(halfTurn)
	if err != nil {
		t.Fatalf("a 180° slew at 2e-8 deg/s, 9e9 s, is inside a Duration and was refused: %v", err)
	}

	if want := 2_500_000 * time.Hour; inside < want-time.Second || inside > want+time.Second {
		t.Errorf("a 180° slew at 2e-8 deg/s takes %v, want 2500000h", inside)
	}

	outside, err := (&BasicTransitionModel{SlewRate: unit.DegreesPerSec(1.9e-8)}).Overhead(halfTurn)
	if !errors.Is(err, ErrSlewTime) {
		t.Fatalf("a 180° slew at 1.9e-8 deg/s, 9.47e9 s, gave %v and %v, want %v", outside, err, ErrSlewTime)
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

// TestSchedulerReportsASlewADurationCannotHold: the refusal reaches the
// caller of a schedule, as every overhead failure does since #454, rather
// than placing or dropping the block.
func TestSchedulerReportsASlewADurationCannotHold(t *testing.T) {
	t.Parallel()

	planner, window, b1, b2 := overheadFixture(t)
	b2.Target = NewStar("B, a quarter turn away", angle.Deg(90), angle.Zero())

	slow := &BasicTransitionModel{SlewRate: unit.DegreesPerSec(1e-12)}

	sched, err := NewScheduler(planner, &GreedyStrategy{}, slow).BuildSchedule(window, []*Block{b1, b2})
	if !errors.Is(err, ErrSlewTime) {
		t.Fatalf("BuildSchedule returned %v (schedule %v), want %v", err, sched, ErrSlewTime)
	}
}
