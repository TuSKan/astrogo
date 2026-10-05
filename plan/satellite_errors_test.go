package plan

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestSatellitePassesReturnEveryPropagationFailure fails the satellite's
// provider once, at calls spread through one pass — its samples, the rise and
// set refinements, the pass events and the culmination search — and requires
// the failure back. SatellitePasses dropped the pass when a refinement failed,
// reported it without a culmination when the culmination search did, and
// filled a pass event with zeros when its look angle did, each with a nil
// error (#454).
func TestSatellitePassesReturnEveryPropagationFailure(t *testing.T) {
	t.Parallel()

	sat, site := testISS(t)
	minEl := angle.Deg(10)

	// The first pass in six hours, and a window around just it, so the calls
	// are few enough to fail in turn.
	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)

	passes, err := SatellitePasses(sat, "ISS", start, start.Add(unit.Hours(6)), site, minEl)
	if err != nil || len(passes) == 0 {
		t.Fatalf("%d passes, err %v; want at least one", len(passes), err)
	}

	from := passes[0].Rise.Time.Add(unit.Minutes(-2))
	to := passes[0].Set.Time.Add(unit.Minutes(2))

	counting := &failingOnceProvider{Provider: sat, on: -1}
	if got, err := SatellitePasses(counting, "ISS", from, to, site, minEl); err != nil || len(got) != 1 {
		t.Fatalf("%d passes around the first, err %v; want 1", len(got), err)
	}

	// Every third call, and the last, which is the culmination's own look
	// angle. Each failed call reruns the search.
	var calls []int

	for call := 0; call < counting.calls; call += 3 {
		calls = append(calls, call)
	}

	calls = append(calls, counting.calls-1)

	missed := 0

	for _, call := range calls {
		prov := &failingOnceProvider{Provider: sat, on: call}

		if _, err := SatellitePasses(prov, "ISS", from, to, site, minEl); !errors.Is(err, errFailingOnce) {
			if missed++; missed <= 3 {
				t.Errorf("provider failing on call %d of %d: err %v, want errFailingOnce", call, counting.calls, err)
			}
		}
	}

	if missed > 0 {
		t.Errorf("%d of %d provider failures dropped, of %d calls", missed, len(calls), counting.calls)
	}
}
