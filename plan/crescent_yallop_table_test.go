package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// yallopObservation is one evening row of Table 4 of Yallop (1997), NAO
// Technical Note No. 69: the site of an attempted first sighting, the new moon
// it followed, and the quantities Yallop computed at its best time.
type yallopObservation struct {
	no       int
	jdNew    float64 // Julian Date of the new moon − 2 400 000 (column 6)
	lat, lon float64 // degrees, east positive
	arcl     float64 // degrees
	arcv     float64 // degrees
	daz      float64 // degrees
	age      float64 // hours, at the best time
	lag      float64 // minutes
	w        float64 // W′, arcminutes
	q        float64
	zone     string // Yallop's zone for q, from his section 6
}

// yallopTable4 is one modern evening row from each of Yallop's six zones (two
// from A), copied from Table 4.
//
// The evening is fixed by columns 6 and 12, the new moon plus the age at the
// best time, rather than by the date columns: for No. 285 the date is the UT
// date of the best time, a local evening earlier, as Yallop's own note on its
// "following day" records.
var yallopTable4 = []yallopObservation{
	{285, 48390.692, 39.0, -76.8, 26.1, 23.1, 12.3, 44.6, 133.6, 1.7, 2.035, "A"},
	{248, 48006.686, 37.7, -121.5, 14.6, 14.5, 0.5, 22.9, 76.8, 0.54, 0.591, "A"},
	{290, 49896.534, -30.1, -71.0, 10.9, 10.9, -0.2, 21.5, 51.5, 0.27, 0.075, "B"},
	{289, 49718.956, 33.0, -106.0, 9.1, 9.0, -0.3, 13.5, 43.2, 0.20, -0.153, "C"},
	{294, 50103.036, 34.1, -118.3, 8.9, 8.8, -1.6, 12.6, 41.0, 0.20, -0.184, "D"},
	{226, 46175.724, 37.2, -84.1, 8.7, 7.9, 3.6, 19.2, 37.5, 0.17, -0.287, "E"},
	{231, 47268.001, 37.2, -84.1, 7.7, 7.6, -1.2, 12.4, 35.9, 0.15, -0.330, "F"},
}

func TestCrescentVisibilityReproducesYallopTable4(t *testing.T) {
	t.Parallel()

	for _, o := range yallopTable4 {
		loc, err := coord.NewGeodetic(angle.Deg(o.lon), angle.Deg(o.lat), 0)
		if err != nil {
			t.Fatalf("NewGeodetic: %v", err)
		}

		site, err := NewSite("yallop", loc)
		if err != nil {
			t.Fatalf("NewSite: %v", err)
		}

		// Three hours before Yallop's best time: after the day's previous
		// sunset, before this one.
		best := 2400000 + o.jdNew + o.age/24
		evening := time.FromJD(best-3.0/24, time.UTC)

		r, err := CrescentVisibility(evening, site, eph.Default())
		if err != nil {
			t.Fatalf("No %d: %v", o.no, err)
		}

		g := r.Geocentric

		// The table gives angles to 0.1°, so 0.05° of each bound is its
		// rounding; the rest is Yallop's ephemeris and his best time, which
		// he rounded to the minute. A topocentric ARCV, the convention this
		// replaced, is lower by most of a degree.
		for _, c := range []struct {
			name      string
			got, want float64
			tol       float64
		}{
			{"ARCL", g.ArcL, o.arcl, 0.1},
			{"ARCV", g.ArcV, o.arcv, 0.1},
			{"DAZ", g.DAZ, math.Abs(o.daz), 0.1},
			{"Age", g.Age, o.age, 0.1},
			{"Lag", g.LT, o.lag, 1},
			{"W′", g.W, o.w, 0.01},
			{"q", r.Yallop.Value, o.q, 0.005},
		} {
			if math.Abs(c.got-c.want) > c.tol {
				t.Errorf("No %d %s = %.4f, Yallop's Table 4 has %.4f (tolerance %g)", o.no, c.name, c.got, c.want, c.tol)
			}
		}

		if r.Yallop.Code != o.zone {
			t.Errorf("No %d zone = %s, Yallop's Table 4 puts q = %+.3f in %s", o.no, r.Yallop.Code, o.q, o.zone)
		}
	}
}
