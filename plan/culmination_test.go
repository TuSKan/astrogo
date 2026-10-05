package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// culminationByFullContexts is findCulmination as it was before #476: a full
// coord.NewContext at every sample. Kept here as the reference the cached
// version is held to.
func culminationByFullContexts(prov eph.Provider, observer *coord.Geodetic, start, end time.Time) (PassEvent, error) {
	step := unit.Seconds(5)
	bestTime := start
	bestEl := -90.0

	sample := func(t time.Time) error {
		altaz, err := LookAngle(prov, 0, coord.NewContext(t, observer, defaultAtm))
		if err != nil {
			return err
		}

		if altaz.Alt().Degrees() > bestEl {
			bestEl = altaz.Alt().Degrees()
			bestTime = t
		}

		return nil
	}

	for t := start; !t.After(end); t = t.Add(step) {
		if err := sample(t); err != nil {
			return PassEvent{}, err
		}
	}

	refineStart, refineEnd := bestTime.Add(-step), bestTime.Add(step)
	if refineStart.Before(start) {
		refineStart = start
	}

	if refineEnd.After(end) {
		refineEnd = end
	}

	for t := refineStart; !t.After(refineEnd); t = t.Add(unit.Seconds(1)) {
		if err := sample(t); err != nil {
			return PassEvent{}, err
		}
	}

	altaz, err := LookAngle(prov, 0, coord.NewContext(bestTime, observer, defaultAtm))
	if err != nil {
		return PassEvent{}, err
	}

	return PassEvent{Time: bestTime, Azimuth: altaz.Az(), Elevation: altaz.Alt(), Range: altaz.Dist()}, nil
}

// TestCulminationThroughTheContextCache holds findCulmination, which samples
// through the pass search's Context cache since #476, to the culminations a
// full Context at every sample finds, over four days of ISS passes.
//
// Measured over a week, all 23 passes agreed exactly: the cache decides
// only which sample wins, and the reported values come from a full Context at
// the winner. The tolerance is one sample step in time, for a near-tie that a
// different floating-point path (an FMA on arm64) could settle the other way.
func TestCulminationThroughTheContextCache(t *testing.T) {
	t.Parallel()

	sat, site := testISS(t)
	start := time.Date(2026, time.April, 17, 0, 0, 0, 0, time.LocationUTC)

	// Four days rather than the week measured: the reference builds a full
	// Context at every sample, and plan's race job has little time to spare.
	passes, err := SatellitePasses(sat, "ISS", start, start.Add(unit.Hours(24*4)), site, angle.Deg(10))
	if err != nil {
		t.Fatalf("SatellitePasses: %v", err)
	}

	if len(passes) < 8 {
		t.Fatalf("only %d passes in four days; the fixture has changed", len(passes))
	}

	for i, p := range passes {
		want, err := culminationByFullContexts(sat, site, p.Rise.Time, p.Set.Time)
		if err != nil {
			t.Fatalf("pass %d: reference: %v", i, err)
		}

		got := p.Culmination

		if dt := math.Abs(got.Time.Sub(want.Time).Seconds()); dt > 1 {
			t.Errorf("pass %d: culmination at %v, a full Context per sample finds %v (%.0f s apart)", i, got.Time, want.Time, dt)
		}

		if d := math.Abs(got.Elevation.Degrees() - want.Elevation.Degrees()); d > 1e-4 {
			t.Errorf("pass %d: culmination elevation %.6f°, a full Context per sample finds %.6f°",
				i, got.Elevation.Degrees(), want.Elevation.Degrees())
		}
	}
}
