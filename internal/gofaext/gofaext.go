package gofaext

import "github.com/hebl/gofa"

// Dtf2d converts a calendar date and time expressed in the given scale
// ("UTC", "TAI", "TT", etc.) into a two-part Julian date (d1, d2).
// It returns an error code matching the gofa convention:
//
//	0  = OK
//	+1 = dubious year (ERFA warning)
//	-1 = bad year
//	-2 = bad month
//	-3 = bad day
//	-4 = bad hour, minute or second
func Dtf2d(scale string, year, month, day, hour, min int, sec float64) (d1, d2 float64, status int) {
	status = gofa.Dtf2d(scale, year, month, day, hour, min, sec, &d1, &d2)
	return d1, d2, status
}

// JdToDate converts a Julian date (supplied as two-part jd1+jd2) to a
// Gregorian calendar date. Returns year, month, day (integer) and the
// fractional day.
func JdToDate(jd1, jd2 float64) (year, month, day int, frac float64, status int) {
	status = gofa.Jd2cal(jd1, jd2, &year, &month, &day, &frac)
	return year, month, day, frac, status
}

// Seps returns the angular separation (in radians) between two directions
// given as (ra1,dec1) and (ra2,dec2), both in radians.
func Seps(ra1, dec1, ra2, dec2 float64) float64 {
	return gofa.Seps(ra1, dec1, ra2, dec2)
}

// Atco13 performs the full ICRS → observed (az, zd, ha, dec, ra)
// transformation including precession-nutation, Earth rotation, polar motion,
// diurnal aberration, and refraction.
//
// All angular inputs and outputs are in radians.
// Pressure phpa in hPa, temperature tc in °C, humidity rh in [0,1],
// wavelength wl in micrometres.
//
// Returns: azimuth aob, zenith distance zob, hour angle hob,
// declination dob, right ascension rob, equation of origins eo, and
// a gofa status code (0 = OK, 1 = dubious year).
func Atco13(
	raRad, decRad float64,
	pmra, pmdec, parallax, rv float64,
	utc1, utc2, dut1 float64,
	elong, phi, hm float64,
	xp, yp float64,
	phpa, tc, rh, wl float64,
) (aob, zob, hob, dob, rob, eo float64, status int) {
	status = gofa.Atco13(
		raRad, decRad,
		pmra, pmdec, parallax, rv,
		utc1, utc2, dut1,
		elong, phi, hm,
		xp, yp,
		phpa, tc, rh, wl,
		&aob, &zob, &hob, &dob, &rob, &eo,
	)
	return aob, zob, hob, dob, rob, eo, status
}

// Atci13 transforms ICRS astrometric coordinates to CIRS (apparent) coordinates.
func Atci13(
	rc, dc float64,
	pr, pd, px, rv float64,
	date1, date2 float64,
) (ri, di, eo float64) {
	gofa.Atci13(rc, dc, pr, pd, px, rv, date1, date2, &ri, &di, &eo)
	return ri, di, eo
}

// Atio13 performs the full observed → ICRS transformation including
// refraction, diurnal aberration, and Earth rotation.
// Atio13 performs the CIRS → observed transformation.
func Atio13(
	ri, di float64, // CIRS RA, Dec (radians)
	utc1, utc2, dut1 float64,
	elong, phi, hm float64,
	xp, yp float64,
	phpa, tc, rh, wl float64,
) (aob, zob, hob, dob, rob float64) {
	gofa.Atio13(
		ri, di,
		utc1, utc2, dut1,
		elong, phi, hm,
		xp, yp,
		phpa, tc, rh, wl,
		&aob, &zob, &hob, &dob, &rob,
	)
	return aob, zob, hob, dob, rob
}

// Atoc13 performs the observed → ICRS transformation for a given coordinate type
// ("A" for Az/ZD, "H" for HA/Dec, "R" for RA/Dec).
func Atoc13(
	typ string,
	ob1, ob2 float64,
	utc1, utc2, dut1 float64,
	elong, phi, hm float64,
	xp, yp float64,
	phpa, tc, rh, wl float64,
) (rc, dc float64) {
	gofa.Atoc13(
		typ, ob1, ob2,
		utc1, utc2, dut1,
		elong, phi, hm,
		xp, yp,
		phpa, tc, rh, wl,
		&rc, &dc,
	)
	return rc, dc
}

// Icrs2g converts ICRS to Galactic coordinates.
func Icrs2g(ra, dec float64) (gl, gb float64) {
	gofa.Icrs2g(ra, dec, &gl, &gb)
	return gl, gb
}

// G2icrs converts Galactic to ICRS coordinates.
func G2icrs(gl, gb float64) (ra, dec float64) {
	gofa.G2icrs(gl, gb, &ra, &dec)
	return ra, dec
}

// Eceq06 converts ICRS to Ecliptic coordinates (IAU 2006).
func Eceq06(date1, date2 float64, ra, dec float64) (elon, elat float64) {
	gofa.Eceq06(date1, date2, ra, dec, &elon, &elat)
	return elon, elat
}

// Eqec06 converts Ecliptic to ICRS coordinates (IAU 2006).
func Eqec06(date1, date2 float64, elon, elat float64) (ra, dec float64) {
	gofa.Eqec06(date1, date2, elon, elat, &ra, &dec)
	return ra, dec
}

// Atic13 converts CIRS to ICRS coordinates.
func Atic13(ri, di, date1, date2 float64) (rc, dc float64) {
	var eo float64
	gofa.Atic13(ri, di, date1, date2, &rc, &dc, &eo)
	return rc, dc
}

// Epv00 returns Earth heliocentric and barycentric position/velocity.
// pvh[0], pvh[1] are heliocentric position and velocity [3]float64 in AU, AU/day.
// pvb[0], pvb[1] are barycentric position and velocity [3]float64 in AU, AU/day.
// status: 0=OK.
func Epv00(date1, date2 float64) (pvh, pvb [2][3]float64, status int) {
	status = gofa.Epv00(date1, date2, &pvh, &pvb)
	return pvh, pvb, status
}

// Moon98 returns the geocentric position/velocity of the Moon.
// pv[0] is position [3]float64 in AU.
// pv[1] is velocity [3]float64 in AU/day.
func Moon98(date1, date2 float64) (pv [2][3]float64) {
	gofa.Moon98(date1, date2, &pv)
	return pv
}

// Plan94 returns the heliocentric position and velocity of a major planet.
// np: 1=Mercury, 2=Venus, 3=EMB, 4=Mars, 5=Jupiter, 6=Saturn, 7=Uranus, 8=Neptune.
func Plan94(date1, date2 float64, np int) (pv [2][3]float64, status int) {
	status = gofa.Plan94(date1, date2, np, &pv)
	return pv, status
}

// Dat returns the number of leap seconds for a given UTC date.
func Dat(iy, im, id int, fd float64) (d float64, status int) {
	status = gofa.Dat(iy, im, id, fd, &d)
	return d, status
}

// Gst06a returns the Greenwich Apparent Sidereal Time (GAST) for the given
// UT1 and TT Julian dates. Result is in radians, [0, 2π).
func Gst06a(uta, utb, tta, ttb float64) float64 {
	return gofa.Gst06a(uta, utb, tta, ttb)
}

// C2t06a returns the Earth rotation matrix mapping ICRS to the Terrestrial
// Intermediate Reference System (TIRS). The transpose of this matrix maps TIRS backwards into ICRS natively.
func C2t06a(tta, ttb, uta, utb, xp, yp float64) [3][3]float64 {
	var rc2t [3][3]float64
	gofa.C2t06a(tta, ttb, uta, utb, xp, yp, &rc2t)
	return rc2t
}

// Refco determining the constants A and B in the atmospheric refraction model
// dz = A tan z + B tan^3 z.
// phpa is pressure in hPa, tc is temp in C, rh is relative humidity, wl is wavelength in um.
func Refco(phpa, tc, rh, wl float64) (refa, refb float64) {
	gofa.Refco(phpa, tc, rh, wl, &refa, &refb)
	return refa, refb
}

// ASTROM aliases the GOFA ASTROM structure for caching star-independent
// astrometry parameters.
type ASTROM = gofa.ASTROM

// Apco13 prepares the ASTROM parameters for ICRS <-> observed transformations.
func Apco13(utc1, utc2, dut1 float64, elong, phi, hm, xp, yp float64, phpa, tc, rh, wl float64) (ASTROM, float64) {
	var astrom ASTROM
	var eo float64
	gofa.Apco13(utc1, utc2, dut1, elong, phi, hm, xp, yp, phpa, tc, rh, wl, &astrom, &eo)
	return astrom, eo
}

// Atciq provides quick ICRS to CIRS transformation given precomputed ASTROM parameters.
func Atciq(rc, dc float64, pr, pd, px, rv float64, astrom *ASTROM) (ri, di float64) {
	gofa.Atciq(rc, dc, pr, pd, px, rv, astrom, &ri, &di)
	return ri, di
}

// Atioq provides quick CIRS to observed place transformation utilizing precomputed configurations.
func Atioq(ri, di float64, astrom *ASTROM) (aob, zob, hob, dob, rob float64) {
	gofa.Atioq(ri, di, astrom, &aob, &zob, &hob, &dob, &rob)
	return aob, zob, hob, dob, rob
}

// Atcoq collapses Atciq and Atioq: quick ICRS to observed.
func Atcoq(rc, dc float64, pr, pd, px, rv float64, astrom *ASTROM) (aob, zob, hob, dob, rob float64) {
	ri, di := Atciq(rc, dc, pr, pd, px, rv, astrom)
	return Atioq(ri, di, astrom)
}

// Nut06a returns the IAU 2006/2000A nutation components:
//   - dpsi: nutation in longitude (radians)
//   - deps: nutation in obliquity (radians)
func Nut06a(date1, date2 float64) (dpsi, deps float64) {
	gofa.Nut06a(date1, date2, &dpsi, &deps)
	return dpsi, deps
}

// Obl06 returns the mean obliquity of the ecliptic (IAU 2006) in radians.
func Obl06(date1, date2 float64) float64 {
	return gofa.Obl06(date1, date2)
}

// Pnm06a returns the bias-precession-nutation matrix (IAU 2006/2000A).
// The matrix operates as V(date) = BPN * V(GCRS), where V(date) is
// with respect to the true equatorial triad of date.
// The transpose maps from the true equatorial frame back to GCRS.
func Pnm06a(date1, date2 float64) [3][3]float64 {
	var rbpn [3][3]float64
	gofa.Pnm06a(date1, date2, &rbpn)
	return rbpn
}

// Ee06a returns the equation of the equinoxes (IAU 2006/2000A) in radians
// for the given TT Julian date. This is the difference between Greenwich
// Apparent Sidereal Time and Greenwich Mean Sidereal Time:
//
//	GAST = GMST + Ee06a
//
// Used to rotate from the mean equinox (TEME) to the true equinox of date.
func Ee06a(date1, date2 float64) float64 {
	return gofa.Ee06a(date1, date2)
}
