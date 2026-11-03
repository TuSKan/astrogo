package skybrightness_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// ks91Observation is one row of Krisciunas & Schaefer (1991), PASP 103, 1033,
// Table 1: the V-band brightness of the moonlit sky, measured from the 2800 m
// level of Mauna Kea.
//
// It is the measured data, not the KS91 model. The model is prohibited as the
// production Moon; the measurements are the published reference the moonlight
// component is held to (#935, #938). Angles are as tabulated, rounded to the
// nearest degree.
type ks91Observation struct {
	when          time.GoTime
	skyAz, skyZ   float64 // the observed sky position, degrees
	moonAz, moonZ float64 // the Moon, degrees
	rho           float64 // the Moon's distance from the sky position, degrees
	alpha         float64 // the phase angle, degrees; negative between full and new
	v             float64 // the sky observed, mag/arcsec^2: moonlight and dark sky together
	bMoon         float64 // the moonlight alone, nanoLamberts
}

func ks91At(year int, month time.Month, day, hour, minute int) time.GoTime {
	return time.GoDate(year, month, day, hour, minute, 0, 0, time.LocationUTC)
}

// ks91Table1 is Table 1, all twelve rows, read from the ADS scan.
// TestKS91TableIsTranscribedCorrectly holds every numeric column to an
// independent relation, so a mistyped digit fails there.
var ks91Table1 = []ks91Observation{
	{ks91At(1990, time.February, 12, 7, 11), 280, 9, 94, 79, 88, -30, 19.813, 308},
	{ks91At(1990, time.December, 28, 7, 5), 89, 18, 300, 4, 22, 49, 18.907, 833},
	{ks91At(1991, time.January, 4, 8, 7), 125, 17, 85, 82, 68, -48, 20.141, 206},
	{ks91At(1991, time.January, 4, 8, 15), 63, 38, 85, 80, 45, -48, 19.605, 380},
	{ks91At(1991, time.January, 4, 8, 23), 55, 30, 86, 78, 53, -48, 19.684, 354},
	{ks91At(1991, time.January, 4, 8, 30), 82, 52, 87, 77, 24, -49, 18.615, 1087},
	{ks91At(1991, time.January, 4, 8, 34), 143, 13, 87, 76, 69, -49, 19.902, 281},
	{ks91At(1991, time.January, 4, 8, 42), 99, 61, 87, 74, 16, -49, 18.456, 1255},
	{ks91At(1991, time.January, 4, 8, 49), 79, 68, 88, 72, 9, -49, 18.228, 1557},
	{ks91At(1991, time.January, 4, 8, 54), 165, 11, 88, 71, 69, -49, 19.771, 329},
	{ks91At(1991, time.January, 21, 6, 26), 263, 49, 265, 62, 14, 119, 20.572, 71},
	{ks91At(1991, time.January, 21, 6, 33), 113, 24, 266, 64, 85, 119, 21.031, 30},
}

// ks91Night returns the V-band extinction, in magnitudes per airmass, and the
// dark-sky zenith brightness, in mag/arcsec^2, that KS91 §2 adopt for o's
// night.
//
// 1991 January 4 is the one night both were measured: k = 0.116 +/- 0.019,
// and V = 21.444 an hour before moonrise. The other nights take the site's
// median extinction, 0.172, with the 1990 yearly average dark sky, 21.377, or
// for 1991 January 21 the value KS91 adopt, 21.388.
func ks91Night(o ks91Observation) (k, darkZenithV float64) {
	switch y, m, d := o.when.Date(); {
	case y == 1991 && m == time.January && d == 4:
		return 0.116, 21.444
	case y == 1991:
		return 0.172, 21.388
	default:
		return 0.172, 21.377
	}
}

// ks91NanoLamberts converts a surface brightness in V mag/arcsec^2 to
// nanoLamberts by KS91 Eq. 1.
func ks91NanoLamberts(v float64) float64 {
	return 34.08 * math.Exp(20.7233-0.92104*v)
}

// ks91Site is Hale Pohaku, the 2800 m level of Mauna Kea. KS91 give only the
// elevation; the position needs to be good to the table's one-degree rounding,
// which TestKS91GeometryMatchesTheEphemeris checks.
func ks91Site(t *testing.T) *coord.Geodetic {
	t.Helper()

	loc, err := coord.NewEarthLocation(19.761, -155.456, 2800)
	if err != nil {
		t.Fatalf("NewEarthLocation: %v", err)
	}

	return loc
}

// ks91Air is an atmosphere whose V-band extinction is k.
//
// KS91 measured the total extinction and nothing of its parts. The molecular
// part follows from the standard atmosphere's pressure at 2800 m and the ozone
// from a subtropical column, so the aerosol optical depth is what is left of k.
// Its type is the clean high-altitude aerosol moonlightScene uses. #938 records
// how little the comparison moves with the asymmetry parameter, the Angstrom
// exponent or the pressure.
func ks91Air(t *testing.T, k float64) *atmosphere.Atmosphere {
	t.Helper()

	build := func(aod float64) *atmosphere.Atmosphere {
		air, err := atmosphere.NewBuilder().
			SurfaceAtAltitude(unit.Meters(2800)).
			Ozone(atmosphere.SubtropicalOzoneDU).
			Aerosol(aod, 550, 1.3, 0.95, 0.65).
			Build()
		if err != nil {
			t.Fatalf("atmosphere Build: %v", err)
		}

		return air
	}

	clean, err := build(0).Extinction(550)
	if err != nil {
		t.Fatalf("Extinction: %v", err)
	}

	if clean >= k {
		t.Fatalf("the aerosol-free air already extinguishes %.4f mag/airmass, at or above k = %.3f", clean, k)
	}

	// Extinction is 2.5 log10(e) magnitudes per unit optical depth, so the
	// aerosol optical depth at 550 nm is the missing magnitudes over that.
	return build((k - clean) / (2.5 * math.Log10(math.E)))
}

// ks91Moonlight evaluates m at o's time and sky position, over the night's
// air, and returns the moonlight in nanoLamberts with the contribution's
// flags.
func ks91Moonlight(t *testing.T, m *skybrightness.ScatteredMoonlight, o ks91Observation) (float64, skybrightness.Flag) {
	t.Helper()

	k, _ := ks91Night(o)

	scene := &skybrightness.Scene{
		Observer:   ks91Site(t),
		Time:       o.when,
		Atmosphere: ks91Air(t, k),
		Ephemeris:  eph.Default(),
	}

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	dir := coord.NewAltAz(angle.Deg(90-o.skyZ), angle.Deg(o.skyAz))

	contribution, err := m.AddRadiance(context.Background(), dst, grid, dir, scene)
	if err != nil {
		t.Fatalf("%v: AddRadiance: %v", o.when, err)
	}

	// The 554 nm grid point, nearest the 553.8 nm ROLO band and so to
	// Johnson V, against the V zero point. Across these rows the radiance at
	// 500 and 600 nm is 1.1-1.34 and 0.77-0.87 times that at 554 nm, a slope
	// smooth enough that one point stands for the band to a few per cent.
	return ks91NanoLamberts(toMagPerArcsec(dst[gridIndex(t, grid, 554)])), contribution.Flags
}

// ks91SeparationUnderRounding returns the least and greatest separation of
// o's sky position from its Moon over every combination of the four printed
// coordinates moved by half a degree, their rounding. Over a box this small the
// separation is monotonic in each coordinate, so its extremes are at the
// corners.
func ks91SeparationUnderRounding(o ks91Observation) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)

	for corner := range 16 {
		shift := func(bit int) float64 {
			if corner&(1<<bit) != 0 {
				return 0.5
			}

			return -0.5
		}

		rho := metrology.AngularSeparation(
			angle.Deg(o.skyAz+shift(0)), angle.Deg(90-o.skyZ-shift(1)),
			angle.Deg(o.moonAz+shift(2)), angle.Deg(90-o.moonZ-shift(3))).Degrees()

		lo, hi = min(lo, rho), max(hi, rho)
	}

	return lo, hi
}

// moonEastOfSun reports whether the Moon is east of the Sun, at the greater
// right ascension of the two within half a turn: waxing, from new to full.
// The vectors are on equatorial axes, so the sign of the cross product's
// polar component is the sign of the difference in right ascension.
func moonEastOfSun(moon, sun vector.Vec3) bool {
	return sun.Cross(moon).Z > 0
}

// ks91InROLODomain reports whether o's phase angle is one ROLO is defined at,
// 1.55 to 97 degrees. The two 1991 January 21 rows, at 119, are not.
func ks91InROLODomain(o ks91Observation) bool {
	a := math.Abs(o.alpha)

	return a >= 1.55 && a <= 97
}

// TestKS91TableIsTranscribedCorrectly recomputes every row's moonlight from its
// observed V by KS91's own Eqs. 1-3, with the extinction and dark sky §2
// adopts for that night, and requires the tabulated B_moon.
//
// That holds four columns of each row to the paper at once: V, B_moon, the sky
// position's zenith distance, and the night. The tolerance is the table's own
// rounding: B_moon is printed to the nanoLambert, and V to a thousandth of a
// magnitude, which is 0.046% of B. A digit mistyped in any of them misses by
// more.
func TestKS91TableIsTranscribedCorrectly(t *testing.T) {
	t.Parallel()

	for _, o := range ks91Table1 {
		k, darkZenithV := ks91Night(o)

		// KS91 Eqs. 2 and 3: the dark sky at zenith distance Z.
		sinZ := math.Sin(o.skyZ * math.Pi / 180)
		x := 1 / math.Sqrt(1-0.96*sinZ*sinZ)
		dark := ks91NanoLamberts(darkZenithV) * math.Pow(10, -0.4*k*(x-1)) * x

		observed := ks91NanoLamberts(o.v)
		moon := observed - dark

		if tol := 0.5 + 0.00046*observed; math.Abs(moon-o.bMoon) > tol {
			t.Errorf("%v: V = %.3f gives B_moon = %.1f nL, but the row says %.0f (tolerance %.1f)",
				o.when, o.v, moon, o.bMoon, tol)
		}
	}
}

// TestKS91GeometryMatchesTheEphemeris places the Moon where Hale Pohaku saw
// it at each row's time, as ScatteredMoonlight does, and requires the
// tabulated position and phase. It checks the time, the site and the angle
// columns, and that the table's azimuth is the one coord uses, from north
// through east.
//
// # Tolerances
//
//   - The Moon's azimuth and zenith distance: a degree. Half is the table's
//     rounding, and half is left for KS91's ephemeris, which the paper does not
//     name.
//   - The phase angle: a degree. Half is the rounding, and half is Meeus's
//     simplified formula, which KS91 used and which ignores the Moon's ecliptic
//     latitude: at up to 5.1 degrees of latitude that moves a phase angle near
//     50 degrees by about 0.2. Its sign, negative from full to new, is checked
//     on its own.
//   - The separation is not compared with this ephemeris. KS91 computed it from
//     positions they then rounded, so it is checked against those positions:
//     some point within half a degree of each of the four printed coordinates
//     must lie within half a degree of the printed separation. A fixed degree
//     is not the right bound. Rounding four coordinates moves a separation by
//     up to 1.2 degrees, and the 08:07 row of January 4 is 1.03 away.
func TestKS91GeometryMatchesTheEphemeris(t *testing.T) {
	t.Parallel()

	loc := ks91Site(t)

	// KS91 tabulate computed positions, so the Moon is placed without
	// refraction, which at the lowest row would lift it about a tenth of a
	// degree.
	geometric := atmosphere.Refraction{Model: atmosphere.RefractionNone{}}

	for _, o := range ks91Table1 {
		at := time.FromGo(o.when)
		in := eph.At(eph.Default(), at)
		ctx := coord.NewContext(at, loc, geometric)

		apparent, err := in.Apparent(eph.Moon, eph.Site(ctx))
		if err != nil {
			t.Fatalf("%v: Apparent: %v", o.when, err)
		}

		moon := ctx.TopocentricToObserved(apparent.Pos)
		moonZ := 90 - moon.Alt().Degrees()

		phase, err := magnitude.PhaseAngle(eph.Default(), eph.Moon, at)
		if err != nil {
			t.Fatalf("%v: PhaseAngle: %v", o.when, err)
		}

		t.Logf("%v: Moon az %.2f (%v), Z %.2f (%v); phase %.2f (%v)",
			o.when, moon.Az().Degrees(), o.moonAz, moonZ, o.moonZ, phase.Degrees(), o.alpha)

		if d := math.Abs(math.Remainder(moon.Az().Degrees()-o.moonAz, 360)); d > 1 {
			t.Errorf("%v: Moon azimuth %.2f, the row says %v", o.when, moon.Az().Degrees(), o.moonAz)
		}

		if math.Abs(moonZ-o.moonZ) > 1 {
			t.Errorf("%v: Moon zenith distance %.2f, the row says %v", o.when, moonZ, o.moonZ)
		}

		if lo, hi := ks91SeparationUnderRounding(o); o.rho+0.5 < lo || o.rho-0.5 > hi {
			t.Errorf("%v: the printed positions give a separation of %.2f-%.2f under their rounding, "+
				"but the row says %v", o.when, lo, hi, o.rho)
		}

		if math.Abs(phase.Degrees()-math.Abs(o.alpha)) > 1 {
			t.Errorf("%v: phase angle %.2f, the row says %v", o.when, phase.Degrees(), o.alpha)
		}

		// Waxing, from new to full, the Moon is east of the Sun.
		sun, err := in.Apparent(eph.Sun, eph.Site(ctx))
		if err != nil {
			t.Fatalf("%v: Apparent Sun: %v", o.when, err)
		}

		waxing := moonEastOfSun(apparent.Pos, sun.Pos)
		if waxing != (o.alpha > 0) {
			t.Errorf("%v: the Moon is waxing = %v, but the row's phase angle is %v", o.when, waxing, o.alpha)
		}
	}
}

// TestScatteredMoonlightAgainstKS91 runs the component at every row of Table 1
// over the night's measured extinction.
//
// # What it asserts, and what it does not yet
//
// Each row inside ROLO's phase range must land within two magnitudes of the
// measured moonlight. That is the bound the test this replaces held, for the
// reason it gave: an error by a factor of ten anywhere in the chain, ROLO's
// reflectance, the inverse squares, the transfer, the phase function, shows
// as two and a half magnitudes. That test compared one geometry against an
// uncited "roughly 18" (#935). This one compares ten against measurements.
//
// The component's uncertainty budget says 16-18%, which this does not assert.
// With the CALSPEC solar spectrum, TestScatteredMoonlightAgainstKS91WithCALSPEC
// measures the component 0.30 mag faint on average, and #938 is finding why.
// The budget's assertion comes with that fix. The blackbody spectrum here
// keeps the check offline, for every CI run, at the cost of about 20%.
//
// The two rows at a phase angle of 119 degrees are outside ROLO, and the
// component must say so.
func TestScatteredMoonlightAgainstKS91(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	for _, o := range ks91Table1 {
		moon, flags := ks91Moonlight(t, m, o)

		if flags&skybrightness.ApproximateMultipleScattering == 0 {
			t.Errorf("%v: the multiple-scattering treatment is not flagged as approximate", o.when)
		}

		if !ks91InROLODomain(o) {
			if flags&skybrightness.ExtrapolatedModel == 0 {
				t.Errorf("%v: phase angle %v is outside ROLO, but the component did not flag it", o.when, o.alpha)
			}

			continue
		}

		dmag := -2.5 * math.Log10(moon/o.bMoon)

		t.Logf("%v: rho %v, Moon Z %v: %.0f nL against %.0f measured (%+.2f mag)",
			o.when, o.rho, o.moonZ, moon, o.bMoon, dmag)

		if math.Abs(dmag) > 2 {
			t.Errorf("%v: %.0f nL against %.0f measured, %+.2f mag", o.when, moon, o.bMoon, dmag)
		}
	}
}
