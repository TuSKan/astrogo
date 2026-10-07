package coord_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// leapStepEOP is Earth orientation across the leap second that ended 2016:
// DUT1 = UT1 − UTC jumps by exactly one second at 2017-01-01 (MJD 57754),
// because UTC steps back a second and UT1 does not. The values either side are
// the bulletin's to a tenth of a second; what matters is the step, and that it
// is here regardless of whether the runner has the bulletin.
type leapStepEOP struct{}

func (leapStepEOP) EOP(mjd float64) (time.EOP, error) {
	dut1 := -0.408
	if mjd >= 57754 {
		dut1 = 0.592 // one second more: UT1 − UTC after UTC stepped back
	}

	return time.EOP{DUT1: dut1, XP: angle.Arcsec(0.077).Radians(), YP: angle.Arcsec(0.264).Radians()}, nil
}

// TestAtTimeAcrossALeapSecond is #489. AtTime reused its base's DUT1, and DUT1
// jumps by a second at a leap second, so a Context derived across one was
// rotated a second away from the one NewContext builds: 13″ for this star.
// AtTime now takes t's own DUT1. What remains is what it holds fixed on any
// hour, bounded by its doc comment at 0.1″ per hour of separation.
func TestAtTimeAcrossALeapSecond(t *testing.T) {
	// Not parallel: the EOP model is process-wide.
	t.Cleanup(time.ResetEOP)
	time.RegisterModel(leapStepEOP{})

	site, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.StandardRefraction()
	star := coord.NewICRS(angle.Hour(5.5), angle.Deg(-30))

	before := time.Date(2016, time.December, 31, 23, 30, 0, 0, time.LocationUTC)
	after := time.Date(2017, time.January, 1, 0, 15, 0, 0, time.LocationUTC)

	for _, c := range []struct {
		name     string
		base, at time.Time
	}{
		{"forward across the leap", before, after},
		{"backward across the leap", after, before},
	} {
		derived, err := coord.NewContext(c.base, site, atm).AtTime(c.at).ICRSToAltAz(star)
		if err != nil {
			t.Fatalf("%s: derived ICRSToAltAz: %v", c.name, err)
		}

		full, err := coord.NewContext(c.at, site, atm).ICRSToAltAz(star)
		if err != nil {
			t.Fatalf("%s: full ICRSToAltAz: %v", c.name, err)
		}

		// 45 minutes apart, so AtTime's own bound is 0.075″; 0.1″ allows the
		// hour it is stated for.
		if sep := altAzSeparationArcsec(derived, full); sep > 0.1 {
			t.Errorf("%s: AtTime is %.3f″ from NewContext; a reused DUT1 is a second of Earth rotation", c.name, sep)
		}
	}
}
