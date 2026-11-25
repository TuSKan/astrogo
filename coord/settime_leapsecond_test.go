package coord_test

import (
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
)

// leapStepEOP is Earth orientation across the leap second that ended 2016:
// DUT1 = UT1 − UTC jumps by exactly one second at 2017-01-01 (MJD 57754),
// because UTC steps back a second and UT1 does not. The values either side are
// the bulletin's to a tenth of a second; what matters is the step, and that it
// is here regardless of whether the runner has the bulletin.
type leapStepEOP struct{}

func (leapStepEOP) EOP(mjd float64) (epoch.EOP, error) {
	dut1 := -0.408
	if mjd >= 57754 {
		dut1 = 0.592 // one second more: UT1 − UTC after UTC stepped back
	}

	return epoch.EOP{DUT1: dut1, XP: angle.Arcsec(0.077).Radians(), YP: angle.Arcsec(0.264).Radians()}, nil
}

// TestSetTimeAcrossALeapSecond is #489. AtTime, which SetTime replaces,
// reused its base's DUT1, and DUT1 jumps by a second at a leap second, so a
// Context derived across one was rotated a second away from the one
// NewContext builds: 13″ for this star. SetTime takes t's own DUT1. What
// remains is its interpolation of the series' state, bounded by its doc
// comment at 0.007″.
func TestSetTimeAcrossALeapSecond(t *testing.T) {
	// Not parallel: the EOP model is process-wide.
	keepEOP(t)
	epoch.RegisterModel(leapStepEOP{})

	site, err := coord.NewGeodetic(angle.LatDeg(-24.6), angle.LonDeg(-70.4), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.StandardRefraction()
	star := coord.NewICRS(angle.Hour(5.5), angle.Deg(-30))

	before := epoch.Date(2016, time.December, 31, 23, 30, 0, 0, time.UTC)
	after := epoch.Date(2017, time.January, 1, 0, 15, 0, 0, time.UTC)

	for _, c := range []struct {
		name     string
		base, at epoch.Time
	}{
		{"forward across the leap", before, after},
		{"backward across the leap", after, before},
	} {
		ctx := mustContext(t, c.base, site, atm)

		ctx.SetTime(c.at)

		derived, err := ctx.ICRSToAltAz(star)
		if err != nil {
			t.Fatalf("%s: derived ICRSToAltAz: %v", c.name, err)
		}

		full, err := mustContext(t, c.at, site, atm).ICRSToAltAz(star)
		if err != nil {
			t.Fatalf("%s: full ICRSToAltAz: %v", c.name, err)
		}

		// 45 minutes apart, inside the hour SetTime moves without rebuilding,
		// so its own bound is 0.075″; 0.1″ allows the hour it is stated for.
		if sep := altAzSeparationArcsec(derived, full); sep > 0.1 {
			t.Errorf("%s: SetTime is %.3f″ from NewContext; a reused DUT1 is a second of Earth rotation", c.name, sep)
		}
	}
}
