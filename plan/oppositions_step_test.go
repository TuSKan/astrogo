package plan

import (
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestOppositionsAtADailyStep: Oppositions samples once a day (#432), and
// must find what a 6-hour sweep of the same spec finds, the step it used
// before: the oppositions of Mars in 2027, Jupiter in 2026 and 2027 and Saturn
// in 2026, and the first three Full Moons of 2026 as oppositions of the Moon
// to the Sun, each to a second.
func TestOppositionsAtADailyStep(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	sun := NewSun(prov)

	for _, c := range []struct {
		target     Observable
		start, end time.Time
		want       int
	}{
		{NewMars(prov), time.Date(2026, 10, 1, 0, 0, 0, 0, time.LocationUTC), time.Date(2027, 4, 1, 0, 0, 0, 0, time.LocationUTC), 1},
		{NewJupiter(prov), time.Date(2025, 12, 1, 0, 0, 0, 0, time.LocationUTC), time.Date(2027, 3, 1, 0, 0, 0, 0, time.LocationUTC), 2},
		{NewSaturn(prov), time.Date(2026, 8, 1, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.LocationUTC), 1},
		{NewMoon(prov), time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 4, 1, 0, 0, 0, 0, time.LocationUTC), 3},
	} {
		name := c.target.Name()

		daily, err := Oppositions(c.start, c.end, c.target, sun)
		if err != nil {
			t.Fatal(err)
		}

		sixHourly, err := NewEventSolver(unit.Hours(6), unit.Seconds(1)).Find(EventSpec{
			Family: EventFamilyRelativeGeometry, Kind: EventOpposition, Target: c.target, Other: sun,
		}, c.start, c.end)
		if err != nil {
			t.Fatal(err)
		}

		if len(daily) != c.want || len(sixHourly) != c.want {
			t.Errorf("%s: %d oppositions at a daily step, %d at 6 hours, want %d", name, len(daily), len(sixHourly), c.want)

			continue
		}

		for k := range daily {
			if d := daily[k].Time.Sub(sixHourly[k].Time).Abs(); d > unit.Seconds(1) {
				t.Errorf("%s: opposition %v at a daily step, %v at 6 hours", name, daily[k].Time, sixHourly[k].Time)
			}
		}
	}
}
