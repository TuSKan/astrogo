package plan

import (
	"errors"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestEventSearchesReturnEveryProviderFailure fails the provider once, on
// each of its calls in turn, and requires the failure back every time, from
// three searches that dropped it somewhere (#437):
//
//   - SunEvents: the rise, set and transit display fields came from
//     altitudesAt, whose failure only skipped the event;
//   - lunar phases through EventSolver: a refinement that failed skipped the
//     phase;
//   - GreatestElongations: the east-or-west test read a failed position as the
//     zero one.
//
// Each was a missing or wrong event with a nil error. Failing once, rather
// than from some call on, is what exposes it: a provider that kept failing
// would be reported by the next sample anyway (see #419's test).
func TestEventSearchesReturnEveryProviderFailure(t *testing.T) {
	site, err := NewSiteEarthLocation("Barcelona", 41.39, 2.17, 0)
	if err != nil {
		t.Fatal(err)
	}

	searches := map[string]func(prov eph.Provider) (int, error){
		// Each window holds the one event it is about, so the provider's calls
		// are few enough to fail each in turn.
		//
		// Sunrise at Barcelona, 05:40:41 UTC.
		"SunEvents, a rise": func(prov eph.Provider) (int, error) {
			events, err := SunEvents(time.Date(2026, 9, 24, 5, 30, 0, 0, time.LocationUTC),
				time.Date(2026, 9, 24, 5, 50, 0, 0, time.LocationUTC), site, prov)

			return len(events), err
		},
		// The Sun's transit there, 11:43:20 UTC.
		"SunEvents, a transit": func(prov eph.Provider) (int, error) {
			events, err := SunEvents(time.Date(2026, 9, 24, 11, 35, 0, 0, time.LocationUTC),
				time.Date(2026, 9, 24, 11, 55, 0, 0, time.LocationUTC), site, prov)

			return len(events), err
		},
		// The First Quarter of 2026-09-18.
		"lunar phases": func(prov eph.Provider) (int, error) {
			events, err := NewEventSolver(unit.Hours(6), unit.Seconds(1)).Find(EventSpec{
				Family: EventFamilyIllumination, Kind: EventAnyPhase, Target: NewMoon(prov),
			}, time.Date(2026, 9, 18, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 9, 19, 12, 0, 0, 0, time.LocationUTC))

			return len(events), err
		},
		// Mercury's greatest eastern elongation, 2026-06-15 19:59 UTC.
		"GreatestElongations": func(prov eph.Provider) (int, error) {
			events, err := GreatestElongations(time.Date(2026, 6, 15, 12, 0, 0, 0, time.LocationUTC),
				time.Date(2026, 6, 16, 6, 0, 0, 0, time.LocationUTC), NewMercury(prov), NewSun(prov))

			return len(events), err
		},
	}

	for name, search := range searches {
		counting := &failingOnceProvider{Provider: eph.Default(), on: -1}

		if n, err := search(counting); err != nil || n == 0 {
			t.Fatalf("%s: %d events, err %v; want the events in the window", name, n, err)
		}

		missed, tried := 0, callsToFail(counting.calls)

		for _, call := range tried {
			if _, err := search(&failingOnceProvider{Provider: eph.Default(), on: call}); !errors.Is(err, errFailingOnce) {
				if missed++; missed <= 3 {
					t.Errorf("%s, provider failing on call %d of %d: err %v, want errFailingOnce",
						name, call, counting.calls, err)
				}
			}
		}

		if missed > 0 {
			t.Errorf("%s: %d of %d provider failures dropped, of its %d calls", name, missed, len(tried), counting.calls)
		}
	}
}

// callsToFail is every call of a search that makes up to 150, and otherwise
// the first ten, every seventh and the last twelve. Failing each call reruns
// the whole search, so GreatestElongations' 540 calls would cost 540 runs; its
// refinement is the same Brent iteration the others exercise, and the
// east-or-west positions this test is after come last.
func callsToFail(n int) []int {
	var out []int

	for call := range n {
		if n <= 150 || call < 10 || call%7 == 0 || call >= n-12 {
			out = append(out, call)
		}
	}

	return out
}
