package ephemeris_test

import (
	"errors"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// Default's doc said no named ID returned ErrUnsupportedBody, which stopped
// being true when IDs became NAIF codes (#809): SOFA's theory places a
// system's barycenter, so the planet centers of Mars through Pluto are refused
// (#1004). Every named ID is held to what the doc now says, so the two cannot
// drift apart again unnoticed.
func TestDefaultAnswersWhatItsDocSays(t *testing.T) {
	t.Parallel()

	answered := map[eph.ID]string{
		0: "SolarSystemBarycenter", 1: "MercuryBarycenter", 2: "VenusBarycenter",
		3: "EarthMoonBarycenter", 4: "MarsBarycenter", 5: "JupiterBarycenter",
		6: "SaturnBarycenter", 7: "UranusBarycenter", 8: "NeptuneBarycenter",
		9: "PlutoBarycenter", 10: "Sun", 199: "Mercury", 299: "Venus",
		301: "Moon", 399: "Earth",
	}
	refused := map[eph.ID]string{
		499: "Mars", 599: "Jupiter", 699: "Saturn", 799: "Uranus", 899: "Neptune", 999: "Pluto",
	}

	in := eph.At(eph.Default(), time.FromJD(2451545.0, time.TT))

	center := func(id eph.ID) eph.ID {
		if id == eph.Earth {
			return eph.Sun
		}

		return eph.Earth
	}

	for id, name := range answered {
		if _, err := in.Geometric(id, center(id)); err != nil {
			t.Errorf("%s (%d): %v, but the doc says Default answers it", name, id, err)
		}
	}

	for id, name := range refused {
		if _, err := in.Geometric(id, center(id)); !errors.Is(err, eph.ErrUnsupportedBody) {
			t.Errorf("%s (%d): err %v, but the doc says it is ErrUnsupportedBody", name, id, err)
		}
	}

	if n := len(answered) + len(refused); n != 21 {
		t.Errorf("the lists hold %d IDs, want all 21 named in ephemeris/core", n)
	}
}
