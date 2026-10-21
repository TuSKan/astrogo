package kepler_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// A heliocentric orbit is made geocentric with its base provider's Sun, not
// a model of the package's own (#810).

// fixedSunBase answers the Sun with one state, counts the asks, and answers
// nothing else.
type fixedSunBase struct {
	sun   core.State
	err   error
	asked int
}

var errNotHere = errors.New("not in this base")

func (b *fixedSunBase) State(id core.ID, _ time.Time) (core.State, error) {
	if id != core.Sun {
		return core.State{}, errNotHere
	}

	b.asked++

	return b.sun, b.err
}

func (*fixedSunBase) Close() error { return nil }

// smallBody is a registered ID no base answers for itself.
const smallBody = core.ID(core.SmallBodyBase + 4179)

func asteroid(t *testing.T) kepler.Elements {
	t.Helper()

	el, err := kepler.NewElements(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		unit.AU(2.53), 0.11, angle.Deg(0.45), angle.Deg(124.3), angle.Deg(277.6), angle.Deg(18.2))
	if err != nil {
		t.Fatalf("elements: %v", err)
	}

	return el
}

func sameBits(a, b vector.Vec3) bool {
	return math.Float64bits(a.X) == math.Float64bits(b.X) &&
		math.Float64bits(a.Y) == math.Float64bits(b.Y) &&
		math.Float64bits(a.Z) == math.Float64bits(b.Z)
}

// TestHeliocentricOrbitIsPlacedWithTheBaseSun: the geocentric state is the
// heliocentric one plus whatever Sun the base answers, so a kernel base puts
// the orbit against the kernel's Earth. Before #810 the base was never asked.
func TestHeliocentricOrbitIsPlacedWithTheBaseSun(t *testing.T) {
	el := asteroid(t)
	base := &fixedSunBase{sun: core.State{
		Pos:    vector.V3(0.3, -0.9, -0.4),
		Vel:    vector.V3(0.016, 0.005, 0.002),
		Frame:  core.FrameICRS,
		Center: core.CenterGeocenter,
	}}

	p := kepler.New(kepler.WithBase(base))
	if err := p.Register(smallBody, el); err != nil {
		t.Fatalf("register: %v", err)
	}

	for d := range 10 {
		at := el.Epoch().Add(unit.Days(37 * float64(d)))

		helio, helioV, err := el.StateAt(at)
		if err != nil {
			t.Fatalf("StateAt: %v", err)
		}

		got, err := p.State(smallBody, at)
		if err != nil {
			t.Fatalf("State: %v", err)
		}

		if !sameBits(got.Pos, helio.Add(base.sun.Pos)) || !sameBits(got.Vel, helioV.Add(base.sun.Vel)) {
			t.Errorf("day %d: State %+v %+v, want heliocentric plus the base's Sun", 37*d, got.Pos, got.Vel)
		}
	}

	if base.asked != 10 {
		t.Errorf("the base was asked for the Sun %d times, want 10", base.asked)
	}
}

// TestDefaultBaseIsUnchangedToTheBit: without WithBase, the Sun is the SOFA
// base's, minus Epv00's heliocentric Earth — what State used to subtract
// directly. Adding its negation is the same arithmetic, so a default Provider
// answers what it did before #810 to the last bit.
func TestDefaultBaseIsUnchangedToTheBit(t *testing.T) {
	el := asteroid(t)

	p := kepler.New()
	if err := p.Register(smallBody, el); err != nil {
		t.Fatalf("register: %v", err)
	}

	for d := range 50 {
		at := el.Epoch().Add(unit.Days(73.1 * float64(d)))

		helio, helioV, err := el.StateAt(at)
		if err != nil {
			t.Fatalf("StateAt: %v", err)
		}

		d1, d2 := at.TDB().JDParts()

		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			t.Fatalf("Epv00 status %d", status)
		}

		wantPos := helio.Sub(vector.V3(pvh[0][0], pvh[0][1], pvh[0][2]))
		wantVel := helioV.Sub(vector.V3(pvh[1][0], pvh[1][1], pvh[1][2]))

		got, err := p.State(smallBody, at)
		if err != nil {
			t.Fatalf("State: %v", err)
		}

		if !sameBits(got.Pos, wantPos) || !sameBits(got.Vel, wantVel) {
			t.Errorf("day %.1f: State %+v, the pre-#810 answer %+v", 73.1*float64(d), got.Pos, wantPos)
		}
	}
}

// TestBaseSunFailureIsReported: a base that cannot place the Sun cannot
// place the orbit, and says so rather than falling back to a second model.
func TestBaseSunFailureIsReported(t *testing.T) {
	el := asteroid(t)

	failing := &fixedSunBase{err: errNotHere}
	heliocentric := &fixedSunBase{sun: core.State{Center: core.CenterHeliocenter}}

	for _, c := range []struct {
		name string
		base *fixedSunBase
		want error
	}{
		{"a base with no Sun", failing, errNotHere},
		{"a base whose Sun is not geocentric", heliocentric, core.ErrWrongCenter},
	} {
		p := kepler.New(kepler.WithBase(c.base))
		if err := p.Register(smallBody, el); err != nil {
			t.Fatalf("register: %v", err)
		}

		if _, err := p.State(smallBody, el.Epoch()); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}
