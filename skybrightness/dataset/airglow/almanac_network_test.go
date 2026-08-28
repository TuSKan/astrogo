//go:build network

package airglow_test

import (
	"context"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
)

// The almanac's solar flux tracks the real solar cycle.
//
// # Why these two dates
//
// Because the value is meant to be Natural Resources Canada's published
// monthly mean, and the cheapest way to tell a real lookup from a constant is
// to ask either side of a solar cycle. June 2020 sits in the cycle 24/25
// minimum and March 2014 at the cycle 24 maximum, so a working lookup returns
// something near 70 for the first and near 150 for the second. A stub, a
// cached default or a units error gives the same number twice.
//
// The bounds are wide because the point is the ratio, not the digits: NRCan
// may revise a monthly mean, and this test should survive that while still
// failing if the lookup stops working.
func TestAlmanacFluxFollowsTheSolarCycle(t *testing.T) {
	testutil.RequireReachable(t, "etimecalret-002.eso.org:443")

	quiet, err := airglow.AlmanacAt(context.Background(),
		gotime.Date(2020, gotime.June, 15, 2, 0, 0, 0, gotime.UTC), airglow.Paranal)
	if err != nil {
		t.Fatalf("AlmanacAt at solar minimum: %v", err)
	}

	active, err := airglow.AlmanacAt(context.Background(),
		gotime.Date(2014, gotime.March, 10, 2, 0, 0, 0, gotime.UTC), airglow.Paranal)
	if err != nil {
		t.Fatalf("AlmanacAt at solar maximum: %v", err)
	}

	t.Logf("June 2020 (minimum): %.1f sfu, season %d, third %d",
		quiet.SolarFluxSFU, quiet.Season, quiet.TimeOfNight)
	t.Logf("March 2014 (maximum): %.1f sfu, season %d, third %d",
		active.SolarFluxSFU, active.Season, active.TimeOfNight)

	if quiet.SolarFluxSFU < 60 || quiet.SolarFluxSFU > 85 {
		t.Errorf("June 2020 reads %.1f sfu; the cycle 24/25 minimum sat near 70",
			quiet.SolarFluxSFU)
	}

	if active.SolarFluxSFU < 120 || active.SolarFluxSFU > 180 {
		t.Errorf("March 2014 reads %.1f sfu; the cycle 24 maximum sat near 150",
			active.SolarFluxSFU)
	}

	if active.SolarFluxSFU <= quiet.SolarFluxSFU {
		t.Errorf("solar maximum reads %.1f and minimum %.1f; a lookup returning the same "+
			"number for both is not a lookup", active.SolarFluxSFU, quiet.SolarFluxSFU)
	}

	// Both dates are mid-month, so the season is the bimonthly period
	// containing them: June is period 4 (jun/jul), March is period 2
	// (feb/mar), counting from 1 = dec/jan.
	if quiet.Season != 4 {
		t.Errorf("June falls in season %d, want 4 (jun/jul)", quiet.Season)
	}

	if active.Season != 2 {
		t.Errorf("March falls in season %d, want 2 (feb/mar)", active.Season)
	}
}

// A month with no published mean yet comes back as unset rather than as -1.
//
// The current month is always in this state, because the figure is a monthly
// mean and the month is not over. Asking for one is not an error and must not
// produce a negative solar flux.
func TestAlmanacReportsAnUnpublishedMonthAsUnset(t *testing.T) {
	testutil.RequireReachable(t, "etimecalret-002.eso.org:443")

	got, err := airglow.AlmanacAt(context.Background(), gotime.Now().UTC(), airglow.Paranal)
	if err != nil {
		t.Fatalf("AlmanacAt for the current month: %v", err)
	}

	if got.SolarFluxSFU < 0 {
		t.Errorf("the current month reads %.1f sfu; the service's -1 must be translated, "+
			"never passed on", got.SolarFluxSFU)
	}

	// Season and third of night are computed rather than looked up, so they
	// are always present even when the flux is not.
	if got.Season < 1 || got.Season > 6 {
		t.Errorf("season is %d, want 1-6 even when the flux is unpublished", got.Season)
	}
}
