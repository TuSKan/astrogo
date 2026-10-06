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
