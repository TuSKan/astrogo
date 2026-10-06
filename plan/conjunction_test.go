package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestConjunctionsAreInRightAscensionOfDate checks what a conjunction in
// right ascension is, at the instant Conjunctions returns: the two apparent
// right ascensions of date are equal.
//
// The check reads them through the equinox-based bias-precession-nutation
// matrix, Pnm06a, while Conjunctions solves on CIRS through C2i06a. The two
// right ascensions differ by the equation of the origins, which is common to
// both bodies, so their difference vanishes at the same instant — and an
// error in either rotation would show here.
//
// Until #545 the difference was taken on ICRS axes, along the J2000 equator.
// At the instants that found, the right ascensions of date were still apart
// by the amount the "J2000 RA" column below prints, which is what makes this
// a test of the frame rather than of the solver.
func TestConjunctionsAreInRightAscensionOfDate(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	raOfDate := func(p coord.ICRS, tm time.Time) float64 {
		tt1, tt2 := tm.TT().JDParts()
		bpn := gofaext.Pnm06a(tt1, tt2)
		v := p.ToUnitVector()

		return math.Atan2(
			bpn[1][0]*v.X+bpn[1][1]*v.Y+bpn[1][2]*v.Z,
			bpn[0][0]*v.X+bpn[0][1]*v.Y+bpn[0][2]*v.Z,
		) * 180 / math.Pi
	}

	for _, c := range []struct {
		name       string
		a, b       eph.ID
		start, end time.Time
	}{
		{"Venus-Jupiter 2023", eph.Venus, eph.Jupiter,
			time.Date(2023, 2, 25, 0, 0, 0, 0, time.LocationUTC), time.Date(2023, 3, 8, 0, 0, 0, 0, time.LocationUTC)},
		{"Jupiter-Saturn 2020", eph.Jupiter, eph.Saturn,
			time.Date(2020, 12, 15, 0, 0, 0, 0, time.LocationUTC), time.Date(2020, 12, 28, 0, 0, 0, 0, time.LocationUTC)},
		{"Moon-Venus 2026", eph.Moon, eph.Venus,
			time.Date(2026, 1, 10, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 1, 25, 0, 0, 0, 0, time.LocationUTC)},
	} {
		a := NewPlanet(c.name, c.a, prov)
		b := NewPlanet(c.name, c.b, prov)

		events, err := Conjunctions(c.start, c.end, a, b)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if len(events) != 1 {
			t.Fatalf("%s: %d conjunctions, want 1", c.name, len(events))
		}

		at := events[0].Time

		pa, err := a.Position(at)
		if err != nil {
			t.Fatal(err)
		}

		pb, err := b.Position(at)
		if err != nil {
			t.Fatal(err)
		}

		diffAt := func(tm time.Time) float64 {
			pa, err := a.Position(tm)
			if err != nil {
				t.Fatal(err)
			}

			pb, err := b.Position(tm)
			if err != nil {
				t.Fatal(err)
			}

			return wrap180(raOfDate(pa, tm)-raOfDate(pb, tm)) * 3600
		}

		// The residual is read as time, dividing by how fast the two close
		// in right ascension: the Moon gains half an arcsecond a second on
		// Venus, Jupiter about 0.004″ a second on Saturn, so one angular
		// bound cannot suit both. One second is the solver's own tolerance.
		rate := (diffAt(at.Add(unit.Minutes(1))) - diffAt(at.Add(unit.Minutes(-1)))) / 120
		ofDate := diffAt(at) / rate
		j2000 := wrap180(pa.RA().Degrees()-pb.RA().Degrees()) * 3600 / rate

		t.Logf("%s at %v: equal RA of date %.3f s away, equal J2000 RA %.1f s away", c.name, at, ofDate, j2000)

		if math.Abs(ofDate) > 1 {
			t.Errorf("%s: the right ascensions of date meet %.2f s from the reported conjunction", c.name, ofDate)
		}
	}
}
