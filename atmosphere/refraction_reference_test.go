package atmosphere_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
)

// nauticalAlmanacRefraction is the Nautical Almanac's refraction for stars at
// its standard conditions, 10 °C and 1010 hPa: apparent altitude in degrees
// and minutes, and the refraction in arcminutes, as printed to a tenth. It is
// the almanac for 2014, pp. A2–A3, as Wilson (2018, Table 3.2) transcribes it.
// Since 2004 the almanac computes these from Hohenkerk & Sinclair's ray
// tracing, at 80% humidity and 0.50169 µm. Above 10° the almanac prints a
// critical table, and each row is the altitude at which its value begins.
var nauticalAlmanacRefraction = []struct{ deg, min, arcmin float64 }{
	{0, 0, 33.8}, {0, 3, 33.2}, {0, 6, 32.6}, {0, 9, 32}, {0, 12, 31.5}, {0, 15, 30.9},
	{0, 18, 30.4}, {0, 21, 29.8}, {0, 24, 29.3}, {0, 27, 28.8}, {0, 30, 28.3}, {0, 33, 27.9},
	{0, 36, 27.4}, {0, 39, 26.9}, {0, 42, 26.5}, {0, 45, 26.1}, {0, 48, 25.7}, {0, 51, 25.3},
	{0, 54, 24.9}, {0, 57, 24.5}, {1, 0, 24.1}, {1, 3, 23.7}, {1, 6, 23.4}, {1, 9, 23},
	{1, 12, 22.7}, {1, 15, 22.3}, {1, 18, 22}, {1, 21, 21.7}, {1, 24, 21.4}, {1, 27, 21.1},
	{1, 30, 20.8}, {1, 35, 20.3}, {1, 40, 19.9}, {1, 45, 19.4}, {1, 50, 19}, {1, 55, 18.6},
	{2, 0, 18.2}, {2, 5, 17.8}, {2, 10, 17.4}, {2, 15, 17.1}, {2, 20, 16.7}, {2, 25, 16.4},
	{2, 30, 16.1}, {2, 35, 15.8}, {2, 40, 15.4}, {2, 45, 15.2}, {2, 50, 14.9}, {2, 55, 14.6},
	{3, 0, 14.3}, {3, 5, 14.1}, {3, 10, 13.8}, {3, 15, 13.6}, {3, 20, 13.4}, {3, 25, 13.1},
	{3, 30, 12.9}, {3, 35, 12.7}, {3, 40, 12.5}, {3, 45, 12.3}, {3, 50, 12.1}, {3, 55, 11.9},
	{4, 0, 11.7}, {4, 5, 11.5}, {4, 10, 11.4}, {4, 15, 11.2}, {4, 20, 11}, {4, 25, 10.9},
	{4, 30, 10.7}, {4, 35, 10.6}, {4, 40, 10.4}, {4, 45, 10.3}, {4, 50, 10.1}, {4, 55, 10},
	{5, 0, 9.8}, {5, 5, 9.7}, {5, 10, 9.6}, {5, 15, 9.5}, {5, 20, 9.3}, {5, 25, 9.2},
	{5, 30, 9.1}, {5, 35, 9}, {5, 40, 8.9}, {5, 45, 8.8}, {5, 50, 8.7}, {5, 55, 8.6},
	{6, 0, 8.5}, {6, 10, 8.3}, {6, 20, 8.1}, {6, 30, 7.9}, {6, 40, 7.7}, {6, 50, 7.6},
	{7, 0, 7.4}, {7, 10, 7.2}, {7, 20, 7.1}, {7, 30, 6.9}, {7, 40, 6.8}, {7, 50, 6.7},
	{8, 0, 6.6}, {8, 10, 6.4}, {8, 20, 6.3}, {8, 30, 6.2}, {8, 40, 6.1}, {8, 50, 6},
	{9, 0, 5.9}, {9, 10, 5.8}, {9, 20, 5.7}, {9, 30, 5.6}, {9, 40, 5.5}, {9, 50, 5.4},
	{10, 0, 5.3}, {10, 5, 5.3}, {10, 15, 5.2}, {10, 25, 5.1}, {10, 45, 5}, {10, 55, 4.9},
	{11, 5, 4.8}, {11, 25, 4.7}, {11, 35, 4.6}, {11, 55, 4.5}, {12, 15, 4.4}, {12, 25, 4.3},
	{12, 45, 4.2}, {13, 5, 4.1}, {13, 25, 4}, {13, 45, 3.9}, {14, 5, 3.8}, {14, 25, 3.7},
	{14, 45, 3.6}, {15, 15, 3.5}, {15, 45, 3.4}, {16, 15, 3.3}, {16, 45, 3.2}, {17, 15, 3.1},
	{17, 45, 3}, {18, 25, 2.9}, {18, 55, 2.8}, {19, 35, 2.7}, {20, 25, 2.6}, {21, 5, 2.5},
	{21, 55, 2.4}, {22, 55, 2.3}, {23, 45, 2.2}, {24, 45, 2.1}, {25, 55, 2}, {26, 55, 1.9},
	{28, 15, 1.8}, {29, 35, 1.7}, {31, 15, 1.6}, {32, 55, 1.5}, {34, 45, 1.4}, {36, 35, 1.3},
	{37, 55, 1.2}, {41, 25, 1.1}, {43, 15, 1}, {46, 25, 0.9}, {50, 35, 0.8}, {54, 15, 0.7},
	{58, 15, 0.6}, {62, 45, 0.5}, {67, 35, 0.4}, {73, 5, 0.3}, {78, 25, 0.2}, {84, 5, 0.1},
	{88, 35, 0},
}

// TestRefractionBennettReproducesTheNauticalAlmanac holds RefractionBennett
// to the table it was fitted to, from the horizon to the zenith: within
// 0.15′, the fit's 0.1′ and the table's rounding to a tenth. Measured, 0.12′
// at worst, at 7°30′.
//
// Bennett's own constants, which astrogo used until #588, are 0.7′ high on
// the horizon against this table.
func TestRefractionBennettReproducesTheNauticalAlmanac(t *testing.T) {
	t.Parallel()

	env := atmosphere.Refraction{Pressure: 1010, Temperature: 10, Model: atmosphere.RefractionBennett{}}

	const tolerance = 0.15 // arcminutes

	worst := 0.0

	for _, row := range nauticalAlmanacRefraction {
		alt := angle.Deg(row.deg + row.min/60)
		got := env.RefractFromApparent(alt).Arcminutes()

		d := got - row.arcmin
		if math.Abs(d) > math.Abs(worst) {
			worst = d
		}

		if math.Abs(d) > tolerance {
			t.Errorf("at %g°%02g′: %.3f′, the almanac %.1f′, %+.3f′ apart", row.deg, row.min, got, row.arcmin, d)
		}
	}

	t.Logf("worst %+.3f′ over %d rows", worst, len(nauticalAlmanacRefraction))
}

// sofaRaytrace is the comparison table in iauRefco's own notes: refraction in
// arcseconds against observed zenith distance, at 1005 hPa, 280.15 K, 80%
// humidity and 0.574 µm, for a ray trace through a model atmosphere and for
// iauRefco's A·tan z + B·tan³ z.
var sofaRaytrace = []struct{ zd, raytrace, refco float64 }{
	{10, 10.27, 10.27}, {20, 21.19, 21.20}, {30, 33.61, 33.61}, {40, 48.82, 48.83},
	{45, 58.16, 58.18}, {50, 69.28, 69.30}, {55, 82.97, 82.99}, {60, 100.51, 100.54},
	{65, 124.23, 124.26}, {70, 158.63, 158.68}, {72, 177.32, 177.37}, {74, 200.35, 200.38},
	{76, 229.45, 229.43}, {78, 267.44, 267.29}, {80, 319.13, 318.55},
}

// TestRefractionSOFAIsSOFAAboveTheHandOver: down to 10° of altitude
// RefractionSOFA is iauRefco's series, unchanged by the hand-over below it.
// It reproduces the Refco column of SOFA's own table within 0.03″ (measured
// 0.016″), and so the ray trace beside it within 0.6″ (measured 0.57″, at a
// zenith distance of 80°).
func TestRefractionSOFAIsSOFAAboveTheHandOver(t *testing.T) {
	t.Parallel()

	env := atmosphere.Refraction{Pressure: 1005, Temperature: 280.15 - 273.15, Humidity: 0.8, Wavelength: 0.574}

	for _, row := range sofaRaytrace {
		got := atmosphere.RefractionSOFA{}.RefractFromApparent(angle.Deg(90-row.zd), env).Arcseconds()

		if d := got - row.refco; math.Abs(d) > 0.03 {
			t.Errorf("zenith distance %g°: %.3f″, iauRefco's column %.2f″, %+.3f″ apart", row.zd, got, row.refco, d)
		}

		if d := got - row.raytrace; math.Abs(d) > 0.6 {
			t.Errorf("zenith distance %g°: %.3f″, the ray trace %.2f″, %+.3f″ apart", row.zd, got, row.raytrace, d)
		}
	}
}

// hohenkerkSinclair is Hohenkerk & Sinclair's (1985, NAO Technical Note 63)
// ray tracing near the horizon, as Wilson (2018, Table 3.4) reproduces it: in
// arcseconds against observed zenith distance, at 1010 hPa, 283.15 K, no
// humidity, 0.50169 µm and a lapse rate of 0.0065 K/m, at sea level and
// latitude 50°.
var hohenkerkSinclair = []struct{ zd, arcsec float64 }{
	{75, 214.21}, {76, 229.68}, {77, 247.34}, {78, 267.70}, {79, 291.43}, {80, 319.43},
	{81, 352.93}, {82, 393.69}, {83, 444.25}, {84, 508.42}, {85, 592.09}, {86, 704.78},
	{87, 862.54}, {88, 1093.79}, {89, 1451.88}, {90, 2044.24},
}

// TestRefractionSOFAReachesTheHorizon holds RefractionSOFA, the default, to
// Hohenkerk & Sinclair's ray tracing from 15° of altitude to the horizon:
// within 15″, measured 13.4″ at worst, on the horizon itself, and within
// 1.1″ through the hand-over from 10° to 5°.
//
// Until #588 it was SOFA's series all the way down, clamped at 2.87°, which
// is 23″ short at 5° and 23′ short on the horizon: 644″ against 2044″.
func TestRefractionSOFAReachesTheHorizon(t *testing.T) {
	t.Parallel()

	env := atmosphere.Refraction{Pressure: 1010, Temperature: 10, Wavelength: 0.50169}

	worst := 0.0

	for _, row := range hohenkerkSinclair {
		got := atmosphere.RefractionSOFA{}.RefractFromApparent(angle.Deg(90-row.zd), env).Arcseconds()

		d := got - row.arcsec
		if math.Abs(d) > math.Abs(worst) {
			worst = d
		}

		if math.Abs(d) > 15 {
			t.Errorf("zenith distance %g°: %.2f″, the ray trace %.2f″, %+.2f″ apart", row.zd, got, row.arcsec, d)
		}
	}

	t.Logf("worst %+.2f″", worst)
}

// TestRefractionSOFAHandsOverSmoothly: at both ends of the hand-over, 5° and
// 10°, the refraction and its slope are continuous, so a solver stepping an
// altitude through it sees no kink. The slopes either side of each join may
// differ only by what the curve's own bend gives over the step, about
// 0.007″/° here; a weight that switched linearly would put a corner of about
// 1″/° there, and a hard switch a step of several arcseconds.
func TestRefractionSOFAHandsOverSmoothly(t *testing.T) {
	t.Parallel()

	env := atmosphere.StandardRefraction()

	r := func(h float64) float64 { return env.RefractFromApparent(angle.Deg(h)).Arcseconds() }

	for _, join := range []float64{5, 10} {
		if d := math.Abs(r(join+1e-9) - r(join-1e-9)); d > 1e-6 {
			t.Errorf("at %g°: the refraction steps by %.2e″", join, d)
		}

		const e = 1e-4 // degrees

		left := (r(join) - r(join-e)) / e
		right := (r(join+e) - r(join)) / e

		if d := math.Abs(right - left); d > 0.05 {
			t.Errorf("at %g°: the slope turns a corner, %.4f″/° below and %.4f″/° above", join, left, right)
		}
	}
}
