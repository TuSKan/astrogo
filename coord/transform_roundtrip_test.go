package coord_test

import (
	"math"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	astrotime "github.com/TuSKan/astrogo/time"
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

	epochs := []gotime.Time{
		gotime.Date(2000, 1, 1, 12, 0, 0, 0, gotime.UTC),
		gotime.Date(2026, 8, 21, 0, 0, 0, 0, gotime.UTC),
		gotime.Date(2050, 6, 1, 18, 30, 0, 0, gotime.UTC),
	}

	for _, when := range epochs {
		at := astrotime.FromGo(when)

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

// The known anchors, so the transform is pinned to the sky and not only to
// itself. A pair of mutually inverse transforms can both be wrong.
func TestGalacticAnchors(t *testing.T) {
	t.Parallel()

	// The galactic centre and the north galactic pole, ICRS J2000.
	for _, c := range []struct {
		name         string
		ra, dec      float64
		wantL, wantB float64
		toleranceDeg float64
	}{
		{"galactic centre", 266.405, -28.936, 0, 0, 0.01},
		{"north galactic pole", 192.859, 27.128, 0, 90, 0.01},
	} {
		gal := coord.ICRSToGalactic(coord.NewICRS(angle.Deg(c.ra), angle.Deg(c.dec)))

		// At the pole the longitude is undefined, so compare as directions.
		if sep := separation(gal.L(), gal.B(), angle.Deg(c.wantL), angle.Deg(c.wantB)); sep > c.toleranceDeg {
			t.Errorf("%s: ICRS (%.3f, %+.3f) gave galactic (%.3f, %+.3f), %.4f degrees from (%.1f, %+.1f)",
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
func TestEclipticAnchors(t *testing.T) {
	t.Parallel()

	// J2000, where the equinox and solstice points are at their defining
	// positions and the obliquity is 23.4393 degrees.
	at := astrotime.FromGo(gotime.Date(2000, 1, 1, 12, 0, 0, 0, gotime.UTC))

	const obliquity = 23.4393

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

		if sep := separation(ecl.Lon(), ecl.Lat(), angle.Deg(c.wantLon), angle.Deg(c.wantLat)); sep > 0.01 {
			t.Errorf("%s: ICRS (%.1f, %+.4f) gave ecliptic (%.4f, %+.4f), %.4f degrees from (%.1f, %+.1f)",
				c.name, c.ra, c.dec, ecl.Lon().Degrees(), ecl.Lat().Degrees(), sep, c.wantLon, c.wantLat)
		}
	}

	// The north ecliptic pole sits at right ascension 18h and a declination of
	// ninety degrees less the obliquity. Its longitude is undefined, so the two
	// are compared as directions.
	pole := coord.EclipticToICRS(coord.NewEcliptic(angle.Deg(0), angle.Deg(90)), at)

	if sep := separation(pole.RA(), pole.Dec(), angle.Deg(270), angle.Deg(90-obliquity)); sep > 0.01 {
		t.Errorf("the north ecliptic pole came back at ICRS (%.4f, %+.4f), %.4f degrees from (270, %+.4f)",
			pole.RA().Degrees(), pole.Dec().Degrees(), sep, 90-obliquity)
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

	at := astrotime.FromGo(gotime.Date(2026, 8, 21, 3, 0, 0, 0, gotime.UTC))

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
