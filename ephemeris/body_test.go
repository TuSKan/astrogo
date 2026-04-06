package ephemeris_test

import (
	"testing"

	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestIDString(t *testing.T) {
	testutil.AssertEqual(t, "Sun name", ephemeris.Sun.String(), "Sun")
	testutil.AssertEqual(t, "Mars name", ephemeris.Mars.String(), "Mars")

	// Check the alias
	var p ephemeris.ID = ephemeris.Jupiter
	testutil.AssertEqual(t, "Planet alias", p.String(), "Jupiter")
}

func TestBodyStruct(t *testing.T) {
	b := ephemeris.MarsBody
	testutil.AssertEqual(t, "Body ID", b.ID, ephemeris.Mars)
	testutil.AssertEqual(t, "Body Name", b.Name, "Mars")
	testutil.AssertEqual(t, "Body Kind", int(b.Kind), int(ephemeris.KindPlanet))
}

func TestBodiesList(t *testing.T) {
	if len(ephemeris.Bodies) < 10 {
		t.Errorf("Expected at least 10 major bodies, got %d", len(ephemeris.Bodies))
	}
}
