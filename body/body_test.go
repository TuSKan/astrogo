package body_test

import (
	"testing"

	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestIDString(t *testing.T) {
	testutil.AssertEqual(t, "Sun name", body.Sun.String(), "Sun")
	testutil.AssertEqual(t, "Mars name", body.Mars.String(), "Mars")

	// Check the alias
	var p body.ID = body.Jupiter
	testutil.AssertEqual(t, "Planet alias", p.String(), "Jupiter")
}

func TestBodyStruct(t *testing.T) {
	b := body.MarsBody
	testutil.AssertEqual(t, "Body ID", b.ID, body.Mars)
	testutil.AssertEqual(t, "Body Name", b.Name, "Mars")
	testutil.AssertEqual(t, "Body Kind", int(b.Kind), int(body.KindPlanet))
}

func TestBodiesList(t *testing.T) {
	if len(body.Bodies) < 10 {
		t.Errorf("Expected at least 10 major bodies, got %d", len(body.Bodies))
	}
}
