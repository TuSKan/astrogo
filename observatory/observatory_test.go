package observatory_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/time"
)

func TestNewSite(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	tz, _ := time.LoadLocation("Europe/Rome")
	horizon := angle.Deg(20)

	site, err := observatory.NewSite("My Observatory", loc, horizon, tz)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "Name", site.Name(), "My Observatory")
	testutil.AssertEqual(t, "Longitude", site.Longitude().Degrees(), 10.0)
	testutil.AssertEqual(t, "Latitude", site.Latitude().Degrees(), 45.0)
	testutil.AssertEqual(t, "Height", site.HeightMeters(), 500.0)
	testutil.AssertEqual(t, "Horizon", site.Horizon().Degrees(), 20.0)
	testutil.AssertEqual(t, "TimeZone", site.TimeZone().String(), "Europe/Rome")
}

func TestDefaultTimeZone(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	site, _ := observatory.NewSite("UTC Site", loc, angle.Zero(), nil)

	testutil.AssertEqual(t, "Default TZ", site.TimeZone().String(), "UTC")
}

func TestInvalidHorizon(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)

	_, err := observatory.NewSite("Bad Horizon", loc, angle.Deg(100), nil)
	if err != observatory.ErrInvalidHorizon {
		t.Errorf("Expected ErrInvalidHorizon, got %v", err)
	}

	_, err = observatory.NewSite("Bad Horizon Low", loc, angle.Deg(-95), nil)
	if err != observatory.ErrInvalidHorizon {
		t.Errorf("Expected ErrInvalidHorizon, got %v", err)
	}
}

func TestString(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	site, _ := observatory.NewSite("Test", loc, angle.Deg(20), nil)

	s := site.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}
