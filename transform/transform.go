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

// ── Pipeline Transformations (Astrometric -> Apparent -> Observed) ────────────

// AstrometricToApparent computes the Celestial Intermediate Reference System (CIRS) apparent 
// position of an object from its Astrometric (catalog ICRS) coordinates at a specific time.
// It applies proper motion, parallax, radial velocity, light deflection, and aberration.
func AstrometricToApparent(c coord.Astrometric, t time.Time) coord.Apparent {
	jd1, jd2 := t.JDParts()

	ri, di, _ := gofaext.Atci13(
		c.RA.Radians(), c.Dec.Radians(),
		c.PmRA.Radians(), c.PmDec.Radians(), c.Parallax.Radians(), c.RV,
		jd1, jd2,
	)

	return coord.Apparent{
		RA:  angle.Rad(ri).Wrap360(),
		Dec: angle.Rad(di),
	}
}

// ApparentToObserved converts geocentric CIRS Apparent coordinates to local Observed AltAz
// taking into account Earth rotation, polar motion, and atmospheric refraction.
func ApparentToObserved(c coord.Apparent, t time.Time, site earth.Geodetic, atm earth.Atmosphere) coord.AltAz {
	jd1, jd2 := t.JDParts()
	mjd := (jd1 - 2400000.5) + jd2
	eop, _ := earth.GetModel().EOP(mjd)

	p := atm.Pressure
	_, isSOFA := atm.Model.(earth.RefractionSOFA)
	if atm.Model != nil && !isSOFA {
		p = 0.0 // Bypass internal SOFA refraction
	}

	az, zd, _, _, _ := gofaext.Atio13(
		c.RA.Radians(), c.Dec.Radians(),
		jd1, jd2, eop.DUT1,
		site.Lon.Radians(), site.Lat.Radians(), site.Height,
		eop.XP, eop.YP,
		p, atm.Temperature, atm.Humidity, atm.Wavelength,
	)

	alt := angle.Rad(math.Pi/2 - zd)
	if atm.Model != nil && !isSOFA {
		alt += atm.Model.Refract(alt, atm, site)
	}

	return coord.AltAz{
		Alt: alt,
		Az:  angle.Rad(az).Wrap360(),
	}
}

// AstrometricToObserved collapses the entire apparent pipeline from an Astrometric catalog
// point explicitly to a refracted local AltAz position.
func AstrometricToObserved(c coord.Astrometric, t time.Time, site earth.Geodetic, atm earth.Atmosphere) coord.AltAz {
	jd1, jd2 := t.JDParts()
	mjd := (jd1 - 2400000.5) + jd2
	eop, _ := earth.GetModel().EOP(mjd)

	p := atm.Pressure
	_, isSOFA := atm.Model.(earth.RefractionSOFA)
	if atm.Model != nil && !isSOFA {
		p = 0.0
	}

	az, zd, _, _, _, _, _ := gofaext.Atco13(
		c.RA.Radians(), c.Dec.Radians(),
		c.PmRA.Radians(), c.PmDec.Radians(), c.Parallax.Radians(), c.RV,
		jd1, jd2, eop.DUT1,
		site.Lon.Radians(), site.Lat.Radians(), site.Height,
		eop.XP, eop.YP,
		p, atm.Temperature, atm.Humidity, atm.Wavelength,
	)

	alt := angle.Rad(math.Pi/2 - zd)
	if atm.Model != nil && !isSOFA {
		alt += atm.Model.Refract(alt, atm, site)
	}

	return coord.AltAz{
		Alt: alt,
		Az:  angle.Rad(az).Wrap360(),
	}
}

// ── Equatorial <-> Horizontal (Legacy Implementations) ──────────────────────

// ICRSToAltAz converts purely geometric ICRS coordinates to local observed AltAz 
// relying on standard atmospheric conditions. Parallax and proper motion are 0.
func ICRSToAltAz(c coord.ICRS, t time.Time, site earth.Geodetic) (coord.AltAz, error) {
	astro := coord.Astrometric{RA: c.RA, Dec: c.Dec}
	altaz := AstrometricToObserved(astro, t, site, earth.StandardAtmosphere)
	altaz.Dist = c.Dist // Preserve geometric distance
	return altaz, nil
}

// ICRSToHourAngle converts purely geometric ICRS coordinates to local observed Hour Angle.
func ICRSToHourAngle(c coord.ICRS, t time.Time, site earth.Geodetic) (angle.Angle, error) {
	jd1, jd2 := t.JDParts()
	mjd := (jd1 - 2400000.5) + jd2
	eop, _ := earth.GetModel().EOP(mjd)
	atm := earth.StandardAtmosphere

	_, _, ha, _, _, _, _ := gofaext.Atco13(
		c.RA.Radians(), c.Dec.Radians(),
		0, 0, 0, 0,
		jd1, jd2, eop.DUT1,
		site.Lon.Radians(), site.Lat.Radians(), site.Height,
		eop.XP, eop.YP,
		atm.Pressure, atm.Temperature, atm.Humidity, atm.Wavelength,
	)

	return angle.Rad(ha).Wrap180(), nil
}

// AltAzToICRS converts local observed AltAz back into geometric ICRS assuming
// standard atmospheric refraction.
func AltAzToICRS(c coord.AltAz, t time.Time, site earth.Geodetic) (coord.ICRS, error) {
	jd1, jd2 := t.JDParts()
	mjd := (jd1 - 2400000.5) + jd2
	eop, _ := earth.GetModel().EOP(mjd)
	atm := earth.StandardAtmosphere

	ra, dec := gofaext.Atoc13(
		"A",
		c.Az.Radians(), math.Pi/2-c.Alt.Radians(),
		jd1, jd2, eop.DUT1,
		site.Lon.Radians(), site.Lat.Radians(), site.Height,
		eop.XP, eop.YP,
		atm.Pressure, atm.Temperature, atm.Humidity, atm.Wavelength,
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
	lon, lat := gofaext.Eqec06(jd1, jd2, c.RA.Radians(), c.Dec.Radians())
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
	ra, dec := gofaext.Eceq06(jd1, jd2, c.Lon.Radians(), c.Lat.Radians())
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
