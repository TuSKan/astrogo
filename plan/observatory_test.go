package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/time"
)

func TestNewSite(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	tz, _ := time.LoadLocation("Europe/Rome")
	horizon := angle.Deg(20)

	site, err := NewSite("My Observatory", loc, horizon, tz)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "Name", site.Name(), "My Observatory")
	testutil.AssertEqual(t, "Longitude", site.Longitude().Degrees(), 10.0)
	testutil.AssertEqual(t, "Latitude", site.Latitude().Degrees(), 45.0)
	testutil.AssertEqual(t, "Height", site.HeightMeters(), 500.0)
	testutil.AssertEqual(t, "Horizon", site.Horizon().Degrees(), 20.0)
	testutil.AssertEqual(t, "TimeZone", site.TimeZone().String(), "Europe/Rome")
}

func TestDefaultTimeZone(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	site, _ := NewSite("UTC Site", loc, angle.Zero(), nil)

	testutil.AssertEqual(t, "Default TZ", site.TimeZone().String(), "UTC")
}

func TestInvalidHorizon(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)

	_, err := NewSite("Bad Horizon", loc, angle.Deg(100), nil)
	if err != ErrInvalidHorizon {
		t.Errorf("Expected ErrInvalidHorizon, got %v", err)
	}

	_, err = NewSite("Bad Horizon Low", loc, angle.Deg(-95), nil)
	if err != ErrInvalidHorizon {
		t.Errorf("Expected ErrInvalidHorizon, got %v", err)
	}
}

func TestString(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	site, _ := NewSite("Test", loc, angle.Deg(20), nil)

	s := site.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

func TestSiteEqual(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	a, _ := NewSite("Test", loc, angle.Deg(20), nil)
	b, _ := NewSite("Test", loc, angle.Deg(20), nil)
	c, _ := NewSite("Other", loc, angle.Deg(20), nil)

	if !a.Equal(b) {
		t.Error("identical sites should be equal")
	}
	if a.Equal(c) {
		t.Error("sites with different names should not be equal")
	}
}

func TestWithHorizon(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)

	s2, err := site.WithHorizon(angle.Deg(15))
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "WithHorizon", s2.Horizon().Degrees(), 15.0, 1e-12)

	// Invalid horizon should fail
	_, err = site.WithHorizon(angle.Deg(100))
	testutil.AssertError(t, err)
}

func TestLocalSiderealTime(t *testing.T) {
	// Greenwich (lon=0) at J2000.0 (2000-01-01 12:00:00 UTC = JD 2451545.0)
	// GAST at J2000.0 is approximately 18.697 hours = 280.46 degrees
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(51.5), 0) // Greenwich
	site, _ := NewSite("Greenwich", loc, angle.Zero(), nil)

	tm := time.FromJD(2451545.0, time.UTC)
	lst := site.LocalSiderealTime(tm)

	// Known GAST at J2000.0 ~280.46° ± 0.5°
	expectedDeg := 280.46
	testutil.AssertNear(t, "LST at Greenwich J2000", lst.Degrees(), expectedDeg, 0.5)
}
