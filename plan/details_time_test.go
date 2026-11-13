package plan

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestStringPrintsTimesWithDateAndZone: String printed each event time as
// "03:04 pm", with neither date nor zone, so a UTC context's times read as
// local ones and a set after midnight read as the same day's (#1133). Each
// time now carries both, in the Context time's display zone.
func TestStringPrintsTimesWithDateAndZone(t *testing.T) {
	loc, err := coord.NewEarthLocation(-23.5505, -46.6333, 760)
	testutil.AssertNoError(t, err)

	saoPaulo, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Skipf("time zone database lacks America/Sao_Paulo: %v", err)
	}

	star := NewStar("Sirius", angle.Deg(101.287155), angle.Deg(-16.716116))

	for _, c := range []struct {
		zone *time.Location
		name string // the zone's name as the layout's MST prints it
	}{
		{time.LocationUTC, "UTC"},
		{saoPaulo, "-03"},
	} {
		tm := time.Date(2026, time.April, 25, 20, 0, 0, 0, time.LocationUTC).In(c.zone)
		ctx := coord.NewContext(tm, loc, atmosphere.Refraction{})

		d, err := star.GetDetails(ctx, DetailOverrides{})
		testutil.AssertNoError(t, err)

		if d.RiseTime == nil || d.TransitTime == nil || d.SetTime == nil {
			t.Fatalf("%s: Sirius from São Paulo should rise, transit and set: %+v", c.name, d)
		}

		s := d.String()

		for label, at := range map[string]*time.Time{
			"Rise time:":                 d.RiseTime,
			"Time of maximum elevation:": d.TransitTime,
			"Set time:":                  d.SetTime,
		} {
			want := label + "\t" + at.In(c.zone).Format("2006-01-02 15:04") + " " + c.name
			if !strings.Contains(s, want) {
				t.Errorf("%s: String() lacks %q:\n%s", c.name, want, s)
			}
		}
	}
}
