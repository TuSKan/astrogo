package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// sweptOppositions is what Oppositions did until #787, and what it is held
// to: the apparent places of both bodies sampled at step, each crossing
// rooted to a millisecond.
func sweptOppositions(t *testing.T, step unit.Duration, target, sun Observable, start, end time.Time) []Event {
	t.Helper()

	events, err := NewEventSolver(step, unit.Seconds(0.001)).Find(EventSpec{
		Family: EventFamilyRelativeGeometry, Kind: EventOpposition, Target: target, Other: sun,
	}, start, end)
	if err != nil {
		t.Fatal(err)
	}

	return events
}

// sameOppositions requires got and want to hold the same oppositions, each
// within the solver's one-second tolerance.
func sameOppositions(t *testing.T, name string, got, want []Event) {
	t.Helper()

	if len(got) != len(want) {
		t.Errorf("%s: %d oppositions, the sweep finds %d", name, len(got), len(want))

		return
	}

	for k := range got {
		if d := got[k].Time.Sub(want[k].Time).Abs(); d > unit.Seconds(1) {
			t.Errorf("%s: opposition at %v, the sweep finds it at %v (%v apart)", name, got[k].Time, want[k].Time, d)
		}
	}
}

// TestOppositionsMatchADailyScan holds Oppositions, which scans the alignment
// with the Sun at a step its bound sets (#787), to a daily sweep of the
// apparent places, which it did before: the same oppositions of Mars,
// Jupiter, Saturn, Uranus and Neptune, and Full Moons as oppositions of the
// Moon, each within a second. Short mode keeps one opposition of each.
func TestOppositionsMatchADailyScan(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	sun := NewSun(prov)

	day := func(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.LocationUTC) }

	cases := []struct {
		target     *Planet
		start, end time.Time
	}{
		{NewMars(prov), day(2000, 1, 1), day(2012, 1, 1)},
		{NewJupiter(prov), day(2000, 1, 1), day(2004, 1, 1)},
		{NewSaturn(prov), day(2000, 1, 1), day(2004, 1, 1)},
		{NewUranus(prov), day(2020, 1, 1), day(2024, 1, 1)},
		{NewNeptune(prov), day(2020, 1, 1), day(2024, 1, 1)},
		{NewMoon(prov), day(2026, 1, 1), day(2027, 1, 1)},
	}
	if testing.Short() {
		cases = cases[:0]
		cases = append(cases,
			struct {
				target     *Planet
				start, end time.Time
			}{NewMars(prov), day(2026, 10, 1), day(2027, 4, 1)},
			struct {
				target     *Planet
				start, end time.Time
			}{NewSaturn(prov), day(2026, 8, 1), day(2026, 12, 1)},
			struct {
				target     *Planet
				start, end time.Time
			}{NewMoon(prov), day(2026, 1, 1), day(2026, 4, 1)},
		)
	}

	for _, c := range cases {
		got, err := Oppositions(c.start, c.end, c.target, sun)
		if err != nil {
			t.Fatal(err)
		}

		if len(got) == 0 {
			t.Errorf("%s: no oppositions over %v to %v", c.target.Name(), c.start, c.end)
		}

		sameOppositions(t, c.target.Name(), got, sweptOppositions(t, unit.Days(1), c.target, sun, c.start, c.end))
	}
}

// TestOppositionsOfOrbitsThatKeepPaceWithTheEarth: the scan's step holds for
// orbits whose heliocentric motion can match the Earth's, where the longitude
// difference turns back on itself: an Apollo-type asteroid that crosses the
// Earth's orbit, and a comet inclined 50° with its perihelion at 1.1 AU. Both
// are held to a six-hourly sweep, a daily one in short mode. A fixed star,
// which the Sun passes once a year, is held to a daily one.
func TestOppositionsOfOrbitsThatKeepPaceWithTheEarth(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	years, sweep := 4, unit.Hours(6)
	if testing.Short() {
		years, sweep = 2, unit.Days(1)
	}

	end := epoch.AddDate(years, 0, 0)

	for _, c := range []struct {
		name          string
		a, e          float64
		i, node, argp float64
	}{
		{"Apollo-type", 1.47, 0.56, 6.35, 35.6, 286.0},
		{"comet-like", 11, 0.9, 50, 120, 40},
	} {
		el, err := eph.NewElements(epoch, unit.AU(c.a), c.e, angle.Deg(c.i), angle.Deg(c.node), angle.Deg(c.argp), angle.Deg(0))
		if err != nil {
			t.Fatal(err)
		}

		prov, err := eph.NewFromElements(eph.ID(2900001), el)
		if err != nil {
			t.Fatal(err)
		}

		body, sun := NewGenericBody(c.name, eph.ID(2900001), prov), NewSun(prov)

		got, err := Oppositions(epoch, end, body, sun)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if len(got) == 0 {
			t.Errorf("%s: no oppositions in %d years", c.name, years)
		}

		sameOppositions(t, c.name, got, sweptOppositions(t, sweep, body, sun, epoch, end))
	}

	prov := eph.Default()
	spica, sun := NewStar("Spica", angle.Deg(201.298), angle.Deg(-11.161)), NewSun(prov)

	got, err := Oppositions(epoch, end, spica, sun)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != years {
		t.Errorf("Spica: %d oppositions in %d years", len(got), years)
	}

	sameOppositions(t, "Spica", got, sweptOppositions(t, unit.Days(1), spica, sun, epoch, end))
}

// TestOppositionsOfAnInferiorPlanetAreNone: Mercury and Venus are never
// opposite the Sun. Their alignments with it are inferior conjunctions, which
// the scan finds and must not report.
func TestOppositionsOfAnInferiorPlanetAreNone(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	for _, p := range []*Planet{NewMercury(prov), NewVenus(prov)} {
		got, err := Oppositions(start, start.AddDate(2, 0, 0), p, NewSun(prov))
		if err != nil || len(got) != 0 {
			t.Errorf("%s: %d oppositions, err %v; want none", p.Name(), len(got), err)
		}
	}
}

// TestOppositionsTakeTheSunEitherWay: an opposition is of a body to the Sun,
// and the Sun may come first. Without the Sun, or with it twice, there is
// none to search: ErrOppositionNeedsSun.
func TestOppositionsTakeTheSunEitherWay(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	sun, mars := NewSun(prov), NewMars(prov)
	start := time.Date(2027, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.AddDate(0, 3, 0)

	one, err := Oppositions(start, end, mars, sun)
	if err != nil {
		t.Fatal(err)
	}

	other, err := Oppositions(start, end, sun, mars)
	if err != nil {
		t.Fatal(err)
	}

	if len(one) != 1 || len(other) != 1 || !one[0].Time.Equal(other[0].Time) {
		t.Errorf("Mars to the Sun: %v; the Sun to Mars: %v", one, other)
	}

	for _, pair := range [][2]Observable{{mars, NewJupiter(prov)}, {sun, sun}} {
		if _, err := Oppositions(start, end, pair[0], pair[1]); !errors.Is(err, ErrOppositionNeedsSun) {
			t.Errorf("%s and %s: err %v, want ErrOppositionNeedsSun", pair[0].Name(), pair[1].Name(), err)
		}
	}
}

// TestOppositionsKeepToTheInterval: an opposition two seconds inside either
// end of the interval is found, and one two seconds outside it is not, though
// the scan's own alignment can fall a fraction of a second away from it.
func TestOppositionsKeepToTheInterval(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	sun, mars := NewSun(prov), NewMars(prov)

	found, err := Oppositions(time.Date(2027, 1, 1, 0, 0, 0, 0, time.LocationUTC), time.Date(2027, 4, 1, 0, 0, 0, 0, time.LocationUTC), mars, sun)
	if err != nil || len(found) != 1 {
		t.Fatalf("Mars 2027: %v, err %v", found, err)
	}

	at := found[0].Time
	edge := unit.Seconds(2)

	for _, c := range []struct {
		start, end time.Time
		want       int
	}{
		{at.Add(-unit.Days(1)), at.Add(edge), 1},
		{at.Add(-unit.Days(1)), at.Add(-edge), 0},
		{at.Add(-edge), at.Add(unit.Days(1)), 1},
		{at.Add(edge), at.Add(unit.Days(1)), 0},
	} {
		got, err := Oppositions(c.start, c.end, mars, sun)
		if err != nil || len(got) != c.want {
			t.Errorf("[%v, %v]: %d oppositions, err %v; want %d", c.start, c.end, len(got), err, c.want)
		}
	}

	if got, err := Oppositions(at, at, mars, sun); err != nil || len(got) > 1 {
		t.Errorf("an empty interval: %v, err %v", got, err)
	}
}

// TestALongSweepReportsNoWrapAsAnOpposition: EventSolver.Find samples at the
// step it is given. At a step longer than the value takes to run from an
// opposition to the wrap at conjunction, a bracket can hold both and pass for
// a crossing, and the solver then closed on the wrap: at 224 days, 22
// "oppositions" of Jupiter in 2000–2040 that were conjunctions (#787). Such a
// step can miss oppositions, but what it reports must be oppositions.
func TestALongSweepReportsNoWrapAsAnOpposition(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	sun, jupiter := NewSun(prov), NewJupiter(prov)
	start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(2040, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	events, err := NewEventSolver(unit.Days(224), unit.Seconds(1)).Find(EventSpec{
		Family: EventFamilyRelativeGeometry, Kind: EventOpposition, Target: jupiter, Other: sun,
	}, start, end)
	if err != nil {
		t.Fatal(err)
	}

	for _, ev := range events {
		if math.Abs(ev.Value) > 0.01 {
			t.Errorf("an opposition at %v, %.3f° from 180°", ev.Time, ev.Value)
		}
	}
}
