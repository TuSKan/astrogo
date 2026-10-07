package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TestGeocentricToObservedCarriesTheTopocentricDistance is #495:
// GeocentricToObserved takes a position vector, distance and all, and returned
// an AltAz whose Dist() was zero. It now carries |v − observer|, which for the
// Moon differs from the geocentric distance by up to an Earth radius.
func TestGeocentricToObservedCarriesTheTopocentricDistance(t *testing.T) {
	t.Parallel()

	site, err := coord.NewGeodetic(angle.Deg(39.83), angle.Deg(21.42), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	at := time.Date(2026, time.March, 20, 15, 45, 0, 0, time.LocationUTC)

	moon, err := eph.Position(eph.Default(), eph.Moon, at)
	if err != nil {
		t.Fatalf("Moon position: %v", err)
	}

	ctx := coord.NewContext(at, site, atmosphere.StandardRefraction())

	got := ctx.GeocentricToObserved(moon).Dist().AU()
	want := moon.Sub(ctx.ObsVec()).Norm()

	if got == 0 || math.Abs(got-want) > 1e-15 {
		t.Errorf("observed distance %.12g AU, want the topocentric %.12g AU", got, want)
	}

	// And it is topocentric rather than geocentric: the observer is thousands
	// of kilometers off the geocenter, so the two must differ.
	if math.Abs(got-moon.Norm())*149597870.7 < 1000 {
		t.Errorf("observed distance is within 1000 km of the geocentric one; the observer was not subtracted")
	}
}
