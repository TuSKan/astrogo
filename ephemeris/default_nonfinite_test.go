package ephemeris_test

import (
	"errors"
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/time"
)

// Default answered the Sun and the Moon at a NaN or infinite time with a NaN
// position and no error: SOFA's theories compute NaN from a NaN date, and
// Moon98 has no status to report it (#1002). Every body Default answers must
// now refuse such a time, and still answer a finite one.
func TestDefaultRefusesANonFiniteTime(t *testing.T) {
	t.Parallel()

	// The named IDs Default answers: the Solar System barycenter, the nine
	// system barycenters, the Sun, Mercury, Venus, the Moon and the Earth.
	answered := []eph.ID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 199, 299, 301, 399}

	center := func(id eph.ID) eph.ID {
		if id == eph.Earth {
			return eph.Sun
		}

		return eph.Earth
	}

	for _, id := range answered {
		if _, err := eph.At(eph.Default(), time.FromJD(2451545.0, time.TT)).Geometric(id, center(id)); err != nil {
			t.Fatalf("%v at J2000: %v, want an answer", id, err)
		}

		for name, jd := range map[string]float64{"NaN": math.NaN(), "+Inf": math.Inf(1), "-Inf": math.Inf(-1)} {
			st, err := eph.At(eph.Default(), time.FromJD(jd, time.TT)).Geometric(id, center(id))
			if !errors.Is(err, kepler.ErrNonFiniteTime) {
				t.Errorf("%v at JD %s: position %v, err %v, want kepler.ErrNonFiniteTime", id, name, st.Pos, err)
			}
		}
	}
}
