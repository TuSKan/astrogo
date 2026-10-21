package kepler_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// A heliocentric orbit is located about the Sun and meets the Earth only
// where an Instant composes it with the base provider's chain: the base's
// Earth, not a model of the package's own (#810, #803).

// fixedEarthBase answers the Earth about the Sun with one state, counts the
// asks, and answers nothing else.
type fixedEarthBase struct {
	earth core.State
	err   error
	asked int
}

var errNotHere = errors.New("not in this base")

func (b *fixedEarthBase) Locate(id core.ID, _ time.Time) (core.State, error) {
	if id != core.Earth {
		return core.State{}, errNotHere
	}

	b.asked++

	return b.earth, b.err
}

func (*fixedEarthBase) Close() error { return nil }

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

// TestHeliocentricOrbitIsPlacedWithTheBaseEarth: the geocentric state is the
// heliocentric one less whatever Earth the base answers, so a kernel base puts
// the orbit against the kernel's Earth. Before #810 the base was never asked.
func TestHeliocentricOrbitIsPlacedWithTheBaseEarth(t *testing.T) {
	el := asteroid(t)
	base := &fixedEarthBase{earth: core.State{
		Pos:    vector.V3(-0.3, 0.9, 0.4),
		Vel:    vector.V3(-0.016, -0.005, -0.002),
		Frame:  core.FrameICRS,
		Center: core.Sun,
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

		got, err := eph.At(p, at).Geometric(smallBody, core.Earth)
		if err != nil {
			t.Fatalf("State: %v", err)
		}

		if !sameBits(got.Pos, helio.Sub(base.earth.Pos)) || !sameBits(got.Vel, helioV.Sub(base.earth.Vel)) {
			t.Errorf("day %d: %+v %+v, want heliocentric less the base's Earth", 37*d, got.Pos, got.Vel)
		}
	}

	if base.asked != 10 {
		t.Errorf("the base was asked for the Earth %d times, want 10, one per instant", base.asked)
	}
}

// TestDefaultBaseIsUnchangedToTheBit: without WithBase, the Earth is the SOFA
// base's, Epv00's heliocentric Earth, located about the Sun as the orbit is,
// so the Instant subtracts it directly, as State did before #810: the same
// answer to the last bit.
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

		got, err := eph.At(p, at).Geometric(smallBody, core.Earth)
		if err != nil {
			t.Fatalf("State: %v", err)
		}

		if !sameBits(got.Pos, wantPos) || !sameBits(got.Vel, wantVel) {
			t.Errorf("day %.1f: State %+v, the pre-#810 answer %+v", 73.1*float64(d), got.Pos, wantPos)
		}
	}
}

// TestBaseEarthFailureIsReported: a base that cannot place the Earth cannot
// place the orbit about it, and says so rather than falling back to a second
// model.
func TestBaseEarthFailureIsReported(t *testing.T) {
	el := asteroid(t)

	p := kepler.New(kepler.WithBase(&fixedEarthBase{err: errNotHere}))
	if err := p.Register(smallBody, el); err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := eph.At(p, el.Epoch()).Geometric(smallBody, core.Earth); !errors.Is(err, errNotHere) {
		t.Errorf("a base with no Earth: err = %v, want errNotHere", err)
	}
}
