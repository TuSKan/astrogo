package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// separation is the great-circle distance between two directions, in degrees,
// via the haversine form so it stays accurate for small separations and does
// not read a right-ascension wrap as a disagreement.
func separation(lon1, lat1, lon2, lat2 angle.Angle) float64 {
	dLon := (lon2 - lon1).Radians()
	dLat := (lat2 - lat1).Radians()

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1.Radians())*math.Cos(lat2.Radians())*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	return 2 * math.Asin(math.Sqrt(math.Min(1, h))) * 180 / math.Pi
}

// sweep returns directions covering the whole sphere, including both poles and
// both sides of the longitude seam, which is where a transform written to one
// convention and inverted in another shows itself.
func sweep() []struct{ lon, lat angle.Angle } {
	latitudes := []float64{-90, -89.9, -60, -23.4, -0.001, 0, 0.001, 23.4, 60, 89.9, 90}
	longitudes := []float64{0, 0.001, 45, 90, 179.999, 180, 180.001, 270, 359.999}

	out := make([]struct{ lon, lat angle.Angle }, 0, len(latitudes)*len(longitudes))

	for _, lat := range latitudes {
		for _, lon := range longitudes {
			out = append(out, struct{ lon, lat angle.Angle }{angle.Deg(lon), angle.Deg(lat)})
		}
	}

	return out
}

// Galactic and equatorial must be exact inverses.
//
// This is the check that found fits.WCS returning sky positions reflected
// through the reference pixel: two halves of one transform written to different
// conventions agree at the origin and nowhere else, and the wrong answer is an
// ordinary direction rather than an error.
func TestICRSGalacticRoundTrip(t *testing.T) {
	t.Parallel()

	for _, c := range sweep() {
		start := coord.NewICRS(c.lon, c.lat)
		gal := coord.ICRSToGalactic(start)
		back := coord.GalacticToICRS(gal)

		if sep := separation(start.RA(), start.Dec(), back.RA(), back.Dec()); sep > 1e-9 {
			t.Errorf("ICRS (%.3f, %+.3f) -> galactic (%.3f, %+.3f) -> ICRS (%.3f, %+.3f): %.3g degrees away",
				c.lon.Degrees(), c.lat.Degrees(),
				gal.L().Degrees(), gal.B().Degrees(),
				back.RA().Degrees(), back.Dec().Degrees(), sep)
		}
	}
}

// Ecliptic and equatorial, at several epochs so the obliquity's own time
// dependence is exercised rather than held fixed.
func TestICRSEclipticRoundTrip(t *testing.T) {
	t.Parallel()

	epochs := []time.GoTime{
		time.GoDate(2000, 1, 1, 12, 0, 0, 0, time.LocationUTC),
		time.GoDate(2026, 8, 21, 0, 0, 0, 0, time.LocationUTC),
		time.GoDate(2050, 6, 1, 18, 30, 0, 0, time.LocationUTC),
	}

	for _, when := range epochs {
		at := time.FromGo(when)

		for _, c := range sweep() {
			start := coord.NewICRS(c.lon, c.lat)
			ecl := coord.ICRSToEcliptic(start, at)
			back := coord.EclipticToICRS(ecl, at)

			if sep := separation(start.RA(), start.Dec(), back.RA(), back.Dec()); sep > 1e-9 {
				t.Errorf("%s: ICRS (%.3f, %+.3f) -> ecliptic (%.3f, %+.3f) -> ICRS: %.3g degrees away",
					when.Format("2006-01-02"), c.lon.Degrees(), c.lat.Degrees(),
					ecl.Lon().Degrees(), ecl.Lat().Degrees(), sep)
			}
		}
	}
}

// The defining anchors, so the transform is pinned to the sky and not only to
// itself. A pair of mutually inverse transforms can both be wrong.
//
// The galactic system is defined by three angles in the Hipparcos Catalogue
// (ESA SP-1200, 1997), given to five decimals and, in SOFA's words, "regarded
// as exact": the north galactic pole at ICRS (P, Q) = (192.85948, 27.12825)
// degrees, and the galactic longitude R = 32.93192 degrees of the ascending
// node of the galactic equator on the ICRS equator, which lies at right
// ascension P + 90 degrees. Being definitions, they hold to the arithmetic,
// and the bound is float64's: 1e-9 degrees, 3.6 microarcseconds. Until #672
// these were written to three decimals and held to 0.01 degrees, which a
// rotation wrong by half an arcminute passes.
func TestGalacticAnchors(t *testing.T) {
	t.Parallel()

	const (
		poleRA  = 192.85948
		poleDec = 27.12825
		node    = 32.93192
		tolDeg  = 1e-9
	)

	// The pole's longitude is undefined, so its latitude alone is checked.
	pole := coord.ICRSToGalactic(coord.NewICRS(angle.Deg(poleRA), angle.Deg(poleDec)))
	if d := 90 - pole.B().Degrees(); d > tolDeg {
		t.Errorf("the north galactic pole, ICRS (%.5f, %+.5f), came out at b = %.12f, %.3g degrees short of 90",
			poleRA, poleDec, pole.B().Degrees(), d)
	}

	for _, c := range []struct {
		name         string
		ra, dec      float64
		wantL, wantB float64
	}{
		// On the galactic equator and on the ICRS equator at once.
		{"ascending node", poleRA + 90, 0, node, 0},
		// The ICRS pole, which by the same rotation sits at the node's
		// longitude plus 90 degrees and at the galactic pole's declination.
		{"north celestial pole", 0, 90, node + 90, poleDec},
	} {
		gal := coord.ICRSToGalactic(coord.NewICRS(angle.Deg(c.ra), angle.Deg(c.dec)))

		if sep := separation(gal.L(), gal.B(), angle.Deg(c.wantL), angle.Deg(c.wantB)); sep > tolDeg {
			t.Errorf("%s: ICRS (%.5f, %+.5f) gave galactic (%.9f, %+.9f), %.3g degrees from (%.5f, %+.5f)",
				c.name, c.ra, c.dec, gal.L().Degrees(), gal.B().Degrees(),
				sep, c.wantL, c.wantB)
		}
	}
}

// The ecliptic pair needs its own anchors, for the same reason the galactic
// pair does: two mutually inverse transforms can both be wrong, and a round
// trip closes either way.
//
// It is not hypothetical here. The gofaext wrappers these go through had their
// two directions documented backwards - iauEceq06 is ecliptic to equatorial
// and iauEqec06 is equatorial to ecliptic, and the comments and parameter
// names said the reverse. coord called them correctly, having evidently been
// written against SOFA rather than against the wrapper, but nothing in the
// tests would have noticed if it had not: every argument is a float64 in
// radians, so a longitude and a right ascension are indistinguishable to the
// compiler, and the round trip above passes under a consistent swap.
//
// At J2000.0 TT the ecliptic of date is the IAU 2006 mean ecliptic, inclined
// to the mean equator by epsilon_0 = 84381.406 arcseconds exactly (Capitaine
// et al. 2003, adopted by IAU 2006 Resolution B1). ICRS is not quite that
// equator and equinox: the frame bias between them, SOFA's iauBi00, is
// -0.0418 arcseconds in longitude, -0.0068 in obliquity and -0.0146 in the
// equinox, and that, not the obliquity, is what separates the anchors from
// their nominal places. The bound is 0.05 arcseconds, above every one of
// them. Until #672 the obliquity was written as 23.4393 and the anchors held
// to 0.01 degrees, 36 arcseconds.
func TestEclipticAnchors(t *testing.T) {
	t.Parallel()

	at := time.J2000()

	const (
		obliquity = 84381.406 / 3600
		tolDeg    = 0.05 / 3600
	)

	for _, c := range []struct {
		name             string
		ra, dec          float64
		wantLon, wantLat float64
	}{
		// The four cardinal points of the ecliptic, which is where the two
		// frames are related by the obliquity alone.
		{"vernal equinox", 0, 0, 0, 0},
		{"summer solstice point", 90, obliquity, 90, 0},
		{"autumnal equinox", 180, 0, 180, 0},
		{"winter solstice point", 270, -obliquity, 270, 0},
	} {
		ecl := coord.ICRSToEcliptic(coord.NewICRS(angle.Deg(c.ra), angle.Deg(c.dec)), at)

		if sep := separation(ecl.Lon(), ecl.Lat(), angle.Deg(c.wantLon), angle.Deg(c.wantLat)); sep > tolDeg {
			t.Errorf("%s: ICRS (%.1f, %+.7f) gave ecliptic (%.7f, %+.7f), %.4f arcseconds from (%.1f, %+.1f)",
				c.name, c.ra, c.dec, ecl.Lon().Degrees(), ecl.Lat().Degrees(), sep*3600, c.wantLon, c.wantLat)
		}
	}

	// The north ecliptic pole sits at right ascension 18h and a declination of
	// ninety degrees less the obliquity. Its longitude is undefined, so the two
	// are compared as directions.
	pole := coord.EclipticToICRS(coord.NewEcliptic(angle.Deg(0), angle.Deg(90)), at)

	if sep := separation(pole.RA(), pole.Dec(), angle.Deg(270), angle.Deg(90-obliquity)); sep > tolDeg {
		t.Errorf("the north ecliptic pole came back at ICRS (%.7f, %+.7f), %.4f arcseconds from (270, %+.7f)",
			pole.RA().Degrees(), pole.Dec().Degrees(), sep*3600, 90-obliquity)
	}

	// And the direction of the tilt, which is the part a swap would invert: a
	// point on the equator a quarter turn from the equinox is north of the
	// ecliptic, not south.
	north := coord.ICRSToEcliptic(coord.NewICRS(angle.Deg(90), angle.Deg(0)), at)
	if north.Lat().Degrees() >= 0 {
		t.Errorf("ICRS (90, 0) is %+.4f degrees from the ecliptic; it lies south of it",
			north.Lat().Degrees())
	}

	south := coord.ICRSToEcliptic(coord.NewICRS(angle.Deg(270), angle.Deg(0)), at)
	if south.Lat().Degrees() <= 0 {
		t.Errorf("ICRS (270, 0) is %+.4f degrees from the ecliptic; it lies north of it",
			south.Lat().Degrees())
	}
}

// The ecliptic of date is defined on TT, so one instant must give one ecliptic
// whatever scale it is held in. Until #672 both transforms read the caller's
// Julian Date as TT, and a UTC instant was taken 69 seconds early, moving the
// result by about 3e-8 degrees.
func TestEclipticTakesTheInstantNotItsScale(t *testing.T) {
	t.Parallel()

	utc := time.Date(2026, 8, 21, 0, 0, 0, 0, time.LocationUTC)
	tt := utc.TT()

	icrs := coord.NewICRS(angle.Deg(123.4), angle.Deg(-45.6))

	a := coord.ICRSToEcliptic(icrs, utc)
	b := coord.ICRSToEcliptic(icrs, tt)

	if sep := separation(a.Lon(), a.Lat(), b.Lon(), b.Lat()); sep > 1e-12 {
		t.Errorf("ICRSToEcliptic: the same instant as UTC and as TT differs by %.3g degrees", sep)
	}

	ecl := coord.NewEcliptic(angle.Deg(210.5), angle.Deg(12.3))

	c := coord.EclipticToICRS(ecl, utc)
	d := coord.EclipticToICRS(ecl, tt)

	if sep := separation(c.RA(), c.Dec(), d.RA(), d.Dec()); sep > 1e-12 {
		t.Errorf("EclipticToICRS: the same instant as UTC and as TT differs by %.3g degrees", sep)
	}
}

// Horizontal and equatorial must invert, which exercises the whole astrometric
// reduction rather than a rotation matrix.
//
// Refraction bends the light, so the pair is only exactly invertible with it
// switched off; with it on, the two directions differ by the refraction itself,
// which is the physically right answer and not a round-trip error. The
// tolerance below is loose for that reason and tightens as the altitude rises.
func TestAltAzICRSRoundTrip(t *testing.T) {
	t.Parallel()

	site, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	at := time.FromGo(time.GoDate(2026, 8, 21, 3, 0, 0, 0, time.LocationUTC))

	// No refraction, so the transform is a pure geometric inverse.
	ctx := coord.NewContext(at, site, atmosphere.Refraction{})

	for _, alt := range []float64{5, 15, 30, 45, 60, 80, 89} {
		for _, az := range []float64{0, 45, 90, 135, 180, 225, 270, 315, 359.9} {
			start := coord.NewAltAz(angle.Deg(alt), angle.Deg(az))

			icrs, err := ctx.AltAzToICRS(start)
			if err != nil {
				t.Fatalf("AltAzToICRS(%.1f, %.1f): %v", alt, az, err)
			}

			back, err := ctx.ICRSToAltAz(icrs)
			if err != nil {
				t.Fatalf("ICRSToAltAz: %v", err)
			}

			sep := separation(start.Az(), start.Alt(), back.Az(), back.Alt())
			if sep > 1.0/3600 { // one arcsecond
				t.Errorf("alt %.1f az %.1f round-tripped %.4f arcsec away (via RA %.4f dec %+.4f)",
					alt, az, sep*3600, icrs.RA().Degrees(), icrs.Dec().Degrees())
			}
		}
	}
}
