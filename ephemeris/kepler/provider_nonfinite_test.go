package kepler_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/time"
)

// Locate refuses a NaN or infinite time before choosing between registered
// elements and the base, so both paths are covered by the one check (#1002).
// The SOFA base used to answer with a NaN position and no error.
func TestLocateRefusesANonFiniteTime(t *testing.T) {
	t.Parallel()

	p := kepler.New()
	if err := p.Register(core.PlutoBarycenter, kepler.PlutoElements()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// The Sun and the Moon come from the SOFA base, Pluto's barycenter from
	// the registered elements.
	for _, id := range []core.ID{core.Sun, core.Moon, core.PlutoBarycenter} {
		if _, err := p.Locate(id, time.FromJD(2451545.0, time.TT)); err != nil {
			t.Fatalf("%v at J2000: %v, want an answer", id, err)
		}

		for name, jd := range map[string]float64{"NaN": math.NaN(), "+Inf": math.Inf(1), "-Inf": math.Inf(-1)} {
			st, err := p.Locate(id, time.FromJD(jd, time.TT))
			if !errors.Is(err, kepler.ErrNonFiniteTime) {
				t.Errorf("%v at JD %s: position %v, err %v, want ErrNonFiniteTime", id, name, st.Pos, err)
			}
		}
	}
}
