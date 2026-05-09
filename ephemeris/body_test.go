package ephemeris_test

import (
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestIDString(t *testing.T) {
	testutil.AssertEqual(t, "Sun name", eph.Sun.String(), "Sun")
	testutil.AssertEqual(t, "Mars name", eph.Mars.String(), "Mars")

	// Check the alias
	p := eph.Jupiter
	testutil.AssertEqual(t, "Planet alias", p.String(), "Jupiter")
}

func TestBodyStruct(t *testing.T) {
	b := eph.MarsBody
	testutil.AssertEqual(t, "Body ID", b.ID, eph.Mars)
	testutil.AssertEqual(t, "Body Name", b.Name, "Mars")
	testutil.AssertEqual(t, "Body Kind", int(b.Kind), int(eph.KindPlanet))
}

func TestBodiesList(t *testing.T) {
	if len(eph.Bodies) < 10 {
		t.Errorf("Expected at least 10 major bodies, got %d", len(eph.Bodies))
	}
}
