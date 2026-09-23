package magnitude_test

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
)

// TestPlanetMagnitudesAgreeWithHorizons pins every planet but Saturn to JPL
// Horizons' APmag; Saturn has its own test, TestSaturnsRingsBrightenItFromEitherFace.
//
// TestPlanetApparent_AllPlanets checks one date against ranges up to ten
// magnitudes wide, and a range that wide passed Saturn's south face at 1.9 mag
// too faint (#375). This asserts values.
//
// Reference values are the APmag column of Horizons OBSERVER tables queried on
// 2026-09-23: CENTER='500@399', QUANTITIES='9,24', 00:00 UT. Dates were chosen
// to span each planet's phase angle (Horizons' S-T-O, recorded beside each
// value), and for Mercury and Venus to cross the regime boundaries in their
// phase curves: Mercury to 177°, Venus on both sides of 163.7°.
//
// Mars is held to a looser bound, on purpose. Mallama & Hilton (2018) correct
// Mars for its rotation and its orbital longitude; astrogo omits both
// corrections, as Skyfield does, and Horizons applies them. The difference
// changes sign from date to date and reaches 0.076 mag on these dates. The
// bound says what the omission costs, and tightens when the corrections exist.
func TestPlanetMagnitudesAgreeWithHorizons(t *testing.T) {
	p := defaultProvider()

	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	const (
		// Every planet here but Mars agrees to 0.007 mag or better.
		tol = 0.02
		// Mars without the rotation and orbital-longitude corrections.
		tolMars = 0.1
	)

	cases := []struct {
		planet   eph.ID
		y, m, d  int
		sto      float64
		horizons float64
	}{
		{eph.Mercury, 2020, 1, 1, 12.2374, -0.903},
		{eph.Mercury, 2024, 3, 10, 31.270, -1.427},
		{eph.Mercury, 2023, 8, 13, 96.448, 0.472},
		{eph.Mercury, 2022, 6, 15, 109.3835, 0.628},
		{eph.Mercury, 2025, 3, 23, 167.480, 5.155},
		{eph.Mercury, 2025, 11, 20, 176.779, 6.440},

		{eph.Venus, 2021, 3, 15, 4.352, -3.898},
		{eph.Venus, 2023, 1, 20, 29.816, -3.901},
		{eph.Venus, 2026, 9, 23, 123.841, -4.803},
		{eph.Venus, 2025, 3, 23, 168.316, -4.220},
		{eph.Venus, 2023, 8, 13, 169.292, -4.120},

		{eph.Mars, 2025, 11, 20, 8.842, 1.421},
		{eph.Mars, 2023, 8, 13, 18.420, 1.768},
		{eph.Mars, 2020, 1, 1, 24.259, 1.548},
		{eph.Mars, 2023, 1, 20, 28.923, -0.683},
		{eph.Mars, 2026, 9, 23, 35.294, 1.086},
		{eph.Mars, 2022, 6, 15, 43.126, 0.577},

		{eph.Jupiter, 2020, 1, 1, 0.625, -1.838},
		{eph.Jupiter, 2021, 3, 15, 6.441, -2.008},
		{eph.Jupiter, 2024, 3, 10, 9.132, -2.138},
		{eph.Jupiter, 2023, 8, 13, 11.734, -2.445},

		{eph.Uranus, 2025, 11, 20, 0.088, 5.617},
		{eph.Uranus, 2020, 1, 1, 2.620, 5.777},
		{eph.Uranus, 2023, 8, 13, 2.955, 5.770},
		{eph.Uranus, 2026, 9, 23, 2.699, 5.665},

		{eph.Neptune, 2021, 3, 15, 0.128, 7.831},
		{eph.Neptune, 2020, 1, 1, 1.724, 7.789},
		{eph.Neptune, 2022, 6, 15, 1.946, 7.776},
		{eph.Neptune, 2026, 9, 23, 0.120, 7.679},

		// Pluto with Charon, from the Explanatory Supplement's V(1,0) = -1.01;
		// Horizons uses -1.00, so these sit 0.010-0.012 mag bright (#407).
		// Both implement the same historical law, so this is a consistency
		// check on astrogo's geometry and arithmetic, not on the law, which
		// can be tenths of a magnitude from Pluto's real brightness (see
		// plutoMag).
		{eph.Pluto, 2020, 1, 1, 0.3608, 14.384},
		{eph.Pluto, 2024, 7, 1, 0.6269, 14.412},
		{eph.Pluto, 2026, 12, 20, 0.9357, 14.608},
		{eph.Pluto, 2026, 9, 23, 1.3494, 14.536},
		{eph.Pluto, 2022, 4, 1, 1.5852, 14.460},
	}

	for _, tc := range cases {
		tm := time.Date(tc.y, time.Month(tc.m), tc.d, 0, 0, 0, 0, time.LocationUTC)

		got, err := magnitude.PlanetApparent(p, tc.planet, tm)
		if err != nil {
			t.Errorf("%v %s: %v", tc.planet, tm, err)
			continue
		}

		limit := tol
		if tc.planet == eph.Mars {
			limit = tolMars
		}

		if diff := got - tc.horizons; math.Abs(diff) > limit {
			t.Errorf("%v on %04d-%02d-%02d (phase %.1f°): V = %+.3f, Horizons gives %+.3f (off by %+.3f, limit %.2f)",
				tc.planet, tc.y, tc.m, tc.d, tc.sto, got, tc.horizons, diff, limit)
		}
	}
}
