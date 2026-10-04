package plan

import (
	"errors"
	"fmt"
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestMoonPhasesSeeTheLastPartialStep: a phase between the last whole
// six-hour step and the end of the window is found, as is one in a window
// shorter than a step, and so is the eclipse at such a syzygy (#419). The
// windows are the issue's: Full Moon at 2026-09-26 16:49:02 UTC (Skyfield,
// DE421), and the total lunar eclipse greatest at 2026-03-03 11:33:37 UT
// (NASA's canon, 11:34:52 TD).
func TestMoonPhasesSeeTheLastPartialStep(t *testing.T) {
	prov := eph.Default()
	fullMoon := time.Date(2026, 9, 26, 16, 49, 2, 0, time.LocationUTC)

	for _, w := range []struct {
		name       string
		start, end time.Time
	}{
		{"last partial step", time.Date(2026, 9, 26, 8, 0, 0, 0, time.LocationUTC), time.Date(2026, 9, 26, 17, 0, 0, 0, time.LocationUTC)},
		{"window shorter than a step", time.Date(2026, 9, 26, 16, 0, 0, 0, time.LocationUTC), time.Date(2026, 9, 26, 17, 0, 0, 0, time.LocationUTC)},
	} {
		phases, err := MoonPhases(w.start, w.end, prov)
		if err != nil {
			t.Fatalf("%s: %v", w.name, err)
		}

		if len(phases) != 1 || phases[0].Phase != PhaseFullMoon {
			t.Errorf("%s: %v, want the Full Moon", w.name, phases)

			continue
		}

		if off := math.Abs(phases[0].Time.Sub(fullMoon).Minutes()); off > 2 {
			t.Errorf("%s: Full Moon at %v, %.1f minutes from Skyfield's", w.name, phases[0].Time, off)
		}
	}

	eclipses, err := LunarEclipses(time.Date(2026, 3, 2, 21, 0, 0, 0, time.LocationUTC), time.Date(2026, 3, 3, 14, 0, 0, 0, time.LocationUTC), prov)
	if err != nil {
		t.Fatal(err)
	}

	greatest := time.Date(2026, 3, 3, 11, 33, 37, 0, time.LocationUTC)
	if len(eclipses) != 1 || math.Abs(eclipses[0].Time.Sub(greatest).Minutes()) > 2 {
		t.Errorf("LunarEclipses over [2026-03-02 21:00, 2026-03-03 14:00]: %v, want the total eclipse at %v", eclipses, greatest)
	}
}

// TestMoonPhasesDoNotDependOnWhereTheWindowIsCut: the phases in a span are the
// phases of its pieces, wherever it is cut. Cutting every 7 h 13 min puts a
// cut inside nearly every six-hour step, which is where #419 lost phases.
func TestMoonPhasesDoNotDependOnWhereTheWindowIsCut(t *testing.T) {
	prov := eph.Default()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Days(60))

	whole, err := MoonPhases(start, end, prov)
	if err != nil {
		t.Fatal(err)
	}

	var pieces []MoonPhaseEvent

	for a := start; a.Before(end); {
		b := a.Add(unit.Hours(7) + unit.Minutes(13))
		if b.After(end) {
			b = end
		}

		got, err := MoonPhases(a, b, prov)
		if err != nil {
			t.Fatal(err)
		}

		pieces = append(pieces, got...)
		a = b
	}

	if len(whole) < 8 || len(pieces) != len(whole) {
		t.Fatalf("%d phases over the whole span, %d over its pieces", len(whole), len(pieces))
	}

	for i := range whole {
		if pieces[i].Phase != whole[i].Phase || math.Abs(pieces[i].Time.Sub(whole[i].Time).Seconds()) > 2 {
			t.Errorf("phase %d: %v at %v whole, %v at %v in pieces", i, whole[i].Phase, whole[i].Time, pieces[i].Phase, pieces[i].Time)
		}
	}
}

// TestMoonPhasesReturnEveryProviderFailure fails the provider once, on each
// of its calls in turn, and requires the failure back every time. The
// refinement used to swallow its error and move on, so a phase could go
// missing with a nil error; a provider that failed on every later call too
// would hide that, since the next sample would report it anyway.
func TestMoonPhasesReturnEveryProviderFailure(t *testing.T) {
	start := time.Date(2026, 9, 20, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(2026, 9, 30, 0, 0, 0, 0, time.LocationUTC)

	counting := &failingOnceProvider{Provider: eph.Default(), on: -1}

	phases, err := MoonPhases(start, end, counting)
	if err != nil || len(phases) == 0 {
		t.Fatalf("%d phases, err %v; want the phases in the window", len(phases), err)
	}

	for call := range counting.calls {
		prov := &failingOnceProvider{Provider: eph.Default(), on: call}

		if _, err := MoonPhases(start, end, prov); !errors.Is(err, errFailingOnce) {
			t.Errorf("provider failing on call %d of %d: err %v, want errFailingOnce", call, counting.calls, err)
		}
	}
}

var errFailingOnce = errors.New("failingOnceProvider: this call fails")

// failingOnceProvider fails its call number on (from zero) and answers every
// other.
type failingOnceProvider struct {
	eph.Provider

	calls, on int
}

func (p *failingOnceProvider) State(id eph.ID, t time.Time) (core.State, error) {
	call := p.calls
	p.calls++

	if call == p.on {
		return core.State{}, errFailingOnce
	}

	st, err := p.Provider.State(id, t)
	if err != nil {
		return core.State{}, fmt.Errorf("failingOnceProvider: %w", err)
	}

	return st, nil
}
