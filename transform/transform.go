package transform

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/frame"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// ── Equatorial <-> Horizontal ───────────────────────────────────────────────

// ICRSToAltAz converts ICRS coordinates to local observed AltAz at the given
// time and observer location.
func ICRSToAltAz(c coord.ICRS, t time.Time, site earth.Geodetic) (coord.AltAz, error) {
	jd1, jd2 := t.JDParts()

	// Default conditions for refraction (standard atmosphere)
	const (
		pressure    = 1013.25 // hPa
		temperature = 15.0    // °C
		humidity    = 0.5     // 0-1
		wavelength  = 0.55    // μm
	)

	az, zd, _, _, _, _, _ := gofaext.Atco13(
		c.RA.Radians(), c.Dec.Radians(),
		0, 0, 0, 0, // No proper motion/parallax
		jd1, jd2, 0, // Assuming DUT1=0 for v1
		site.Lon.Radians(), site.Lat.Radians(), site.Height,
		0, 0, // No polar motion for v1
		pressure, temperature, humidity, wavelength,
	)

	return coord.AltAz{
		Alt:  angle.Rad(math.Pi/2 - zd),
		Az:   angle.Rad(az).Wrap360(),
		Dist: c.Dist,
	}, nil
}

// AltAzToICRS converts local observed AltAz to ICRS coordinates at the given
// time and observer location.
func AltAzToICRS(c coord.AltAz, t time.Time, site earth.Geodetic) (coord.ICRS, error) {
	jd1, jd2 := t.JDParts()

	const (
		pressure    = 1013.25
		temperature = 15.0
		humidity    = 0.5
		wavelength  = 0.55
	)

	// Use Atoc13 ("A" = Az/ZD type) for the Observed-to-ICRS transformation.
	// This handles the internal Observed -> CIRS -> ICRS chain correctly.
	ra, dec := gofaext.Atoc13(
		"A",
		c.Az.Radians(), math.Pi/2-c.Alt.Radians(),
		jd1, jd2, 0,
		site.Lon.Radians(), site.Lat.Radians(), site.Height,
		0, 0,
		pressure, temperature, humidity, wavelength,
	)

	return coord.ICRS{
		RA:   angle.Rad(ra).Wrap360(),
		Dec:  angle.Rad(dec),
		Dist: c.Dist,
	}, nil
}

// ── ICRS <-> Galactic ────────────────────────────────────────────────────────

// ICRSToGalactic converts ICRS coordinates to Galactic coordinates.
func ICRSToGalactic(c coord.ICRS) coord.Galactic {
	l, b := gofaext.Icrs2g(c.RA.Radians(), c.Dec.Radians())
	return coord.Galactic{
		L:    angle.Rad(l).Wrap360(),
		B:    angle.Rad(b),
		Dist: c.Dist,
	}
}

// GalacticToICRS converts Galactic coordinates to ICRS coordinates.
func GalacticToICRS(c coord.Galactic) coord.ICRS {
	ra, dec := gofaext.G2icrs(c.L.Radians(), c.B.Radians())
	return coord.ICRS{
		RA:   angle.Rad(ra).Wrap360(),
		Dec:  angle.Rad(dec),
		Dist: c.Dist,
	}
}

// ── ICRS <-> Ecliptic ────────────────────────────────────────────────────────

// ICRSToEcliptic converts ICRS coordinates to Geocentric Ecliptic coordinates
// of the given date.
func ICRSToEcliptic(c coord.ICRS, t time.Time) coord.Ecliptic {
	jd1, jd2 := t.JDParts()
	lon, lat := gofaext.Eceq06(jd1, jd2, c.RA.Radians(), c.Dec.Radians())
	return coord.Ecliptic{
		Lon:  angle.Rad(lon).Wrap360(),
		Lat:  angle.Rad(lat),
		Dist: c.Dist,
	}
}

// EclipticToICRS converts Geocentric Ecliptic coordinates of the given date
// to ICRS coordinates.
func EclipticToICRS(c coord.Ecliptic, t time.Time) coord.ICRS {
	jd1, jd2 := t.JDParts()
	ra, dec := gofaext.Eqec06(jd1, jd2, c.Lon.Radians(), c.Lat.Radians())
	return coord.ICRS{
		RA:   angle.Rad(ra).Wrap360(),
		Dec:  angle.Rad(dec),
		Dist: c.Dist,
	}
}

// ── Infrastructure ───────────────────────────────────────────────────────────

// Transformer converts a Cartesian vector from one reference frame to another
// at a given time.
type Transformer interface {
	// Transform converts v from src to dst at time t.
	Transform(src, dst frame.Frame, v vector.Vec3, t time.Time) (vector.Vec3, error)
}

// IdentityTransformer is a no-op transformer that returns the input vector unchanged.
type IdentityTransformer struct{}

// Transform returns v unchanged.
func (IdentityTransformer) Transform(_, _ frame.Frame, v vector.Vec3, _ time.Time) (vector.Vec3, error) {
	return v, nil
}
