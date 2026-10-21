package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestFirstOrderApparentPlaceAgrees holds firstOrderApparentICRS to the
// apparent place it approximates for the Sun and the Moon every five hours
// through a lunation: within a milliarcsecond, which at the Moon's 0.5″ a
// second against the Sun is 2 ms of a phase. Measured, it is 0.02 mas for the
// Sun and 0.56 mas for the Moon: the Moon's is aberration's second-order term,
// (v/c)² sin θ cos θ, which the shortcut leaves out and iauAb carries; against
// the first-order apparent place astrogo used before #803 it was 0.01 mas. The
// geometric place it replaces in moonElongation is 20″ off for the Sun.
func TestFirstOrderApparentPlaceAgrees(t *testing.T) {
	prov := eph.Default()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.LocationUTC)

	worst := map[eph.ID]float64{}

	for h := 0.0; h < 30*24; h += 5 {
		at := start.Add(unit.Hours(h))

		for _, id := range []eph.ID{eph.Sun, eph.Moon} {
			full, err := apparentICRS(prov, id, at)
			if err != nil {
				t.Fatal(err)
			}

			first, err := firstOrderApparentICRS(prov, id, at)
			if err != nil {
				t.Fatal(err)
			}

			worst[id] = max(worst[id], coord.Separation(full, first).Arcseconds())
		}
	}

	for id, w := range worst {
		if w > 0.001 {
			t.Errorf("%v: first-order apparent place %.5f″ from the apparent place, want within 0.001″", id, w)
		}

		t.Logf("%v: within %.5f″", id, w)
	}
}
