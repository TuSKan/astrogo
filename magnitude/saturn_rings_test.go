package magnitude_test

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
)

// TestSaturnsRingsBrightenItFromEitherFace pins Saturn's magnitude to JPL
// Horizons on dates when each face of the rings is lit (#375).
//
// The ring term of Mallama & Hilton (2018) Eq. 10 depends on how far the rings
// are open, not on which face is seen: its inclination is the geometric mean
// of the saturnicentric sub-solar and sub-observer latitudes, set to zero when
// the two differ in sign, and so never negative. A signed inclination dimmed
// Saturn whenever the south face was lit — by 1.8 mag at the 2002 opposition,
// with the rings wide open — while every north-face date stayed right. Between
// 2009 and the Sun's ring-plane crossing on 2025-05-06 only the north face was
// lit, so a test on any date in that span could not have seen it.
//
// Reference values are the APmag column of a Horizons OBSERVER table queried on
// 2026-09-23: COMMAND='699', CENTER='500@399', QUANTITIES='9,14,15,24', 00:00
// UT, satellite solution sat441l. The latitudes recorded beside each value are
// Horizons' planetodetic sub-observer and sub-solar latitudes; the sign is what
// names the face.
//
// The tolerance is well inside the smallest error the signed inclination made
// — 0.137 mag on 2025-09-21, three months after the crossing, when the rings
// were barely open — and far outside the fixed computation's residual, under a
// thousandth of a magnitude on every date here.
func TestSaturnsRingsBrightenItFromEitherFace(t *testing.T) {
	p := defaultProvider()

	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	const tol = 0.02

	cases := []struct {
		name          string
		y, m, d       int
		obsLat, sunLa float64
		horizons      float64
	}{
		{"south face, 2002 opposition", 2002, 12, 17, -31.573790, -31.738349, -0.535},
		{"south face, 2003 opposition", 2003, 12, 31, -30.400571, -30.502578, -0.512},
		{"north face, 2016 opposition", 2016, 6, 3, 30.991852, 31.193316, -0.042},
		{"north face, before the crossing", 2024, 9, 8, 4.586961, 4.352127, 0.575},
		{"south face, just after the crossing", 2025, 9, 21, -2.247479, -2.518719, 0.583},
		{"south face, away from opposition", 2026, 1, 1, -1.170209, -4.393408, 1.007},
		{"south face, 2026 opposition", 2026, 9, 23, -9.613882, -9.229789, 0.383},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tm := time.Date(tc.y, time.Month(tc.m), tc.d, 0, 0, 0, 0, time.LocationUTC)

			got, err := magnitude.PlanetApparent(p, eph.Saturn, tm)
			if err != nil {
				t.Fatalf("PlanetApparent: %v", err)
			}

			if diff := got - tc.horizons; math.Abs(diff) > tol {
				t.Errorf("V = %+.3f, Horizons gives %+.3f (off by %+.3f; sub-observer %+.2f°, sub-solar %+.2f°)",
					got, tc.horizons, diff, tc.obsLat, tc.sunLa)
			}
		})
	}
}
