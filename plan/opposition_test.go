package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TestOppositionsAreInEclipticLongitude finds the issue's three oppositions
// (#413) and checks each against JPL Horizons' ecliptic-longitude opposition
// (CENTER='500@399', QUANTITIES='1,31'; Skyfield agrees to a minute), and
// that at the returned instant the two ecliptic longitudes are 180° apart —
// which is what an opposition is, whatever the ephemeris.
//
// Solved on right ascension, as before #413, these came out 51, 29 and 26
// hours late. On the analytical ephemeris they are within 8.1 minutes; with
// DE440s, within 0.9.
func TestOppositionsAreInEclipticLongitude(t *testing.T) {
	const tolMinutes = 15

	prov := eph.Default()
	sun := NewSun(prov)

	for _, c := range []struct {
		body    *Planet
		year    int
		horizon time.Time
	}{
		{NewMars(prov), 2003, time.Date(2003, 8, 28, 17, 58, 0, 0, time.LocationUTC)},
		{NewMars(prov), 2027, time.Date(2027, 2, 19, 15, 50, 0, 0, time.LocationUTC)},
		{NewSaturn(prov), 2026, time.Date(2026, 10, 4, 12, 29, 0, 0, time.LocationUTC)},
	} {
		start := time.Date(c.year, 1, 1, 0, 0, 0, 0, time.LocationUTC)

		events, err := Oppositions(start, start.AddDate(1, 0, 0), c.body, sun)
		if err != nil {
			t.Fatalf("%s %d: %v", c.body.Name(), c.year, err)
		}

		if len(events) != 1 {
			t.Errorf("%s %d: %d oppositions, want 1", c.body.Name(), c.year, len(events))

			continue
		}

		at := events[0].Time
		if off := at.Sub(c.horizon).Minutes(); math.Abs(off) > tolMinutes {
			t.Errorf("%s %d: opposition at %v, %+.1f minutes from Horizons'", c.body.Name(), c.year, at, off)
		}

		p, err := c.body.Position(at)
		if err != nil {
			t.Fatal(err)
		}

		q, err := sun.Position(at)
		if err != nil {
			t.Fatal(err)
		}

		dLon := wrap180(coord.ICRSToEcliptic(p, at).Lon().Degrees() - coord.ICRSToEcliptic(q, at).Lon().Degrees())
		if math.Abs(math.Abs(dLon)-180) > 1e-3 {
			t.Errorf("%s %d: ecliptic longitudes %.4f° apart at the opposition, want 180°", c.body.Name(), c.year, math.Abs(dLon))
		}
	}
}

// TestFullMoonOppositionsAreFullMoons: the Moon–Sun opposition in ecliptic
// longitude is the Full Moon. Before #413 it was solved on right ascension
// and came out up to 3.25 hours off in 2026; the Full Moon of 2026-09-26 is
// 16:49:02 UTC by Skyfield (DE421).
func TestFullMoonOppositionsAreFullMoons(t *testing.T) {
	events, err := FullMoonOppositions(time.Date(2026, 9, 26, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2026, 9, 27, 0, 0, 0, 0, time.LocationUTC), eph.Default())
	if err != nil {
		t.Fatal(err)
	}

	skyfield := time.Date(2026, 9, 26, 16, 49, 2, 0, time.LocationUTC)
	if len(events) != 1 || math.Abs(events[0].Time.Sub(skyfield).Minutes()) > 2 {
		t.Errorf("FullMoonOppositions on 2026-09-26: %v, want the Full Moon at %v", events, skyfield)
	}
}

func TestWrap180(t *testing.T) {
	for in, want := range map[float64]float64{0: 0, 180: 180, -180: 180, 181: -179, -181: 179, 540: 180, 725: 5} {
		if got := wrap180(in); got != want {
			t.Errorf("wrap180(%v) = %v, want %v", in, got, want)
		}
	}
}
