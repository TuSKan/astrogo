package ephemeris_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestDefaultAnswersTheCodesItHolds is #809's contract for eph.Default: it
// answers the NAIF code it is asked for, or refuses it, and never answers one
// point for another.
func TestDefaultAnswersTheCodesItHolds(t *testing.T) {
	t.Parallel()

	p := eph.Default()
	at := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.LocationUTC)

	const kmPerAU = 149597870.7

	// The geocentre, from the geocentre, is the zero vector. It used to be
	// asked for under Plan94's 3 and came back as the Earth-Moon barycenter.
	earth, err := p.State(eph.Earth, at)
	if err != nil {
		t.Fatalf("Earth: %v", err)
	}

	if earth.Pos.Norm() != 0 || earth.Vel.Norm() != 0 {
		t.Errorf("Earth is %.0f km from the geocentre, want 0", earth.Pos.Norm()*kmPerAU)
	}

	// Mercury and Venus have no moons: each is its own barycenter.
	for _, pair := range [][2]eph.ID{{eph.Mercury, eph.MercuryBarycenter}, {eph.Venus, eph.VenusBarycenter}} {
		a, err := p.State(pair[0], at)
		if err != nil {
			t.Fatalf("%v: %v", pair[0], err)
		}

		b, err := p.State(pair[1], at)
		if err != nil {
			t.Fatalf("%v: %v", pair[1], err)
		}

		if a.Pos != b.Pos {
			t.Errorf("%v and %v differ; they are one point", pair[0], pair[1])
		}
	}

	// From Mars out, the barycenters are answered and the planets' own
	// centers refused, with the barycenter named.
	for _, c := range []struct{ center, bary eph.ID }{
		{eph.Mars, eph.MarsBarycenter},
		{eph.Jupiter, eph.JupiterBarycenter},
		{eph.Saturn, eph.SaturnBarycenter},
		{eph.Uranus, eph.UranusBarycenter},
		{eph.Neptune, eph.NeptuneBarycenter},
		{eph.Pluto, eph.PlutoBarycenter},
	} {
		if _, err := p.State(c.bary, at); err != nil {
			t.Errorf("%v: %v", c.bary, err)
		}

		_, err := p.State(c.center, at)
		if !errors.Is(err, eph.ErrUnsupportedBody) {
			t.Errorf("%v: err = %v, want ErrUnsupportedBody — answering it would be its barycenter's place", c.center, err)
		} else if !strings.Contains(err.Error(), c.bary.String()) {
			t.Errorf("%v: the refusal does not name %v, the code to ask for: %v", c.center, c.bary, err)
		}
	}
}

// TestDefaultPlacesTheEarthMoonBarycenterFromTheMoon: the Earth-Moon
// barycenter is the Moon's share of the pair's mass along the Moon's vector,
// 4,300 to 4,950 km from the geocentre as the Moon's distance varies. Plan94's
// own barycenter less SOFA's Earth was measured 2,620 to 6,790 km out over
// 2026-2027, up to 1,942 km from it (#809).
func TestDefaultPlacesTheEarthMoonBarycenterFromTheMoon(t *testing.T) {
	t.Parallel()

	p := eph.Default()
	mu := constants.Ephemeris.MoonGravitationalParameter.Value /
		constants.Ephemeris.EarthMoonGravitationalParameter.Value

	const kmPerAU = 149597870.7

	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for d := 0; d < 730; d += 7 {
		at := start.Add(unit.Days(float64(d)))

		emb, err := p.State(eph.EarthMoonBarycenter, at)
		if err != nil {
			t.Fatalf("%v: %v", at, err)
		}

		moon, err := p.State(eph.Moon, at)
		if err != nil {
			t.Fatalf("%v: Moon: %v", at, err)
		}

		if d := emb.Pos.Sub(moon.Pos.MulScalar(mu)).Norm() * kmPerAU; d > 1e-6 {
			t.Errorf("%v: the barycenter is %.3g km from the Moon's share of its vector", at, d)
		}

		if r := emb.Pos.Norm() * kmPerAU; r < 4300 || r > 4950 {
			t.Errorf("%v: the barycenter is %.0f km from the geocentre, want 4,300 to 4,950", at, r)
		}
	}
}
