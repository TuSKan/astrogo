package transform

import (
	"fmt"
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

// GeocentricToObserved natively tracks Solar System planets mathematically
// by bypassing SOFA's BARYCENTRIC stellar parallax limitations.
// It algebraically displaces the true local Observer using rigorous Earth matrices natively.
func GeocentricToObserved(v vector.Vec3, t time.Time, site earth.Geodetic, atm earth.Atmosphere) coord.AltAz {
	jd1, jd2 := t.JDParts()
	ut1, ut2 := t.UT1().JDParts()
	tt1, tt2 := t.TT().JDParts()

	mjd := (jd1 - 2400000.5) + jd2
	eop, _ := earth.GetModel().EOP(mjd)

	// 1. Get SOFA ICRS-to-TIRS matrix
	mat := gofaext.C2t06a(tt1, tt2, ut1, ut2, eop.XP, eop.YP)

	// 2. WGS84 simplistic radius displacement in strictly Terrestrial Frame (TIRS)
	const au = 149597870.7
	const rEq = 6378.137
	const f = 1.0 / 298.257223563

	sinLat := math.Sin(site.Lat.Radians())
	cosLat := math.Cos(site.Lat.Radians())

	c_earth := 1.0 / math.Sqrt(cosLat*cosLat+(1.0-f)*(1.0-f)*sinLat*sinLat)
	s_earth := (1.0 - f) * (1.0 - f) * c_earth

	xTIRS := (rEq*c_earth + site.Height/1000.0) * cosLat * math.Cos(site.Lon.Radians()) / au
	yTIRS := (rEq*c_earth + site.Height/1000.0) * cosLat * math.Sin(site.Lon.Radians()) / au
	zTIRS := (rEq*s_earth + site.Height/1000.0) * sinLat / au

	// 3. Multiply TIRS Vector by TRANSPOSE of ICRS->TIRS Matrix to produce Observer ICRS Vector
	obsX := mat[0][0]*xTIRS + mat[1][0]*yTIRS + mat[2][0]*zTIRS
	obsY := mat[0][1]*xTIRS + mat[1][1]*yTIRS + mat[2][1]*zTIRS
	obsZ := mat[0][2]*xTIRS + mat[1][2]*yTIRS + mat[2][2]*zTIRS

	obsVec := vector.Vec3{X: obsX, Y: obsY, Z: obsZ}
	fmt.Printf("Engine obsVec: %v\n", obsVec)

	// 4. Absolute Cartesian Shift
	topoVec := v.Sub(obsVec)

	// 5. Transform Topocentric ICRS vector directly back into the body-fixed ITRS (Terrestrial) frame.
	// Since `mat` rotates ICRS to ITRS, we multiply the ICRS Topocentric vector by `mat`.
	tx := mat[0][0]*topoVec.X + mat[0][1]*topoVec.Y + mat[0][2]*topoVec.Z
	ty := mat[1][0]*topoVec.X + mat[1][1]*topoVec.Y + mat[1][2]*topoVec.Z
	tz := mat[2][0]*topoVec.X + mat[2][1]*topoVec.Y + mat[2][2]*topoVec.Z

	// 6. Convert ITRS to Local Horizon ENU (East, North, Up)
	lonRad := site.Lon.Radians()
	sinLon, cosLon := math.Sincos(lonRad)

	E := -sinLon*tx + cosLon*ty
	N := -sinLat*cosLon*tx - sinLat*sinLon*ty + cosLat*tz
	U := cosLat*cosLon*tx + cosLat*sinLon*ty + sinLat*tz

	// 7. Calculate Geometric Altitude and Azimuth!
	// Azimuth is angle from North (N) towards East (E)
	azimuth := math.Atan2(E, N)
	if azimuth < 0 {
		azimuth += 2 * math.Pi
	}
	altitude := math.Asin(U / topoVec.Norm())

	// 8. Apply Atmospheric Refraction (if specified in the model)
	observedAlt := angle.Rad(altitude)
	if atm.Model != nil {
		observedAlt += atm.Model.Refract(angle.Rad(altitude), atm, site)
	}

	return coord.AltAz{
		Alt: observedAlt,
		Az:  angle.Rad(azimuth),
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
