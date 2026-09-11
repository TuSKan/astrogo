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
func Dtf2d(scale string, year, month, day, hour, minute int, second float64) (d1, d2 float64, status int) {
	status = gofa.Dtf2d(scale, year, month, day, hour, minute, second, &d1, &d2)
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

// Atio13 performs the CIRS → observed transformation, applying refraction,
// diurnal aberration and Earth rotation.
//
// Observed → ICRS is [Atoc13], not this: an earlier version of this comment
// claimed both directions in consecutive sentences.
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

// Eceq06 converts Ecliptic to ICRS equatorial coordinates (IAU 2006).
//
// The direction is the one SOFA's name states - ecliptic to equatorial - and
// not the reverse. This comment and the parameter names said the reverse until
// they were checked against iauEceq06 itself. Every argument here is a
// float64 in radians, so nothing but the names distinguishes a longitude from
// a right ascension, and a caller reading the wrapper rather than SOFA would
// have called this pair backwards with no complaint from anything.
func Eceq06(date1, date2, elon, elat float64) (ra, dec float64) {
	gofa.Eceq06(date1, date2, elon, elat, &ra, &dec)
	return ra, dec
}

// Eqec06 converts ICRS equatorial to Ecliptic coordinates (IAU 2006). See
// [Eceq06] on the direction of the pair.
func Eqec06(date1, date2, ra, dec float64) (elon, elat float64) {
	gofa.Eqec06(date1, date2, ra, dec, &elon, &elat)
	return elon, elat
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

// Era00 returns the Earth Rotation Angle (IAU 2000) in radians for the given
// UT1 two-part Julian date — the one orientation quantity that changes
// materially between nearby instants (as opposed to precession, nutation,
// and polar motion, which drift sub-arcsecond per day).
func Era00(uta, utb float64) float64 {
	return gofa.Era00(uta, utb)
}

// Aper updates only astrom.Eral (= theta + astrom.Along) in place, leaving
// every other ASTROM field untouched — an O(1) alternative to rebuilding the
// whole ASTROM via Apco13 when only the Earth Rotation Angle has changed.
func Aper(theta float64, astrom *ASTROM) {
	gofa.Aper(theta, astrom)
}

// C2i06a returns the celestial-to-intermediate (precession-nutation, IAU
// 2006/2000A) matrix for the given TT two-part Julian date — the slow factor
// of C2t06a, safe to cache across a short time window and reuse with a
// freshly computed Era00/Pom00 via C2tcio.
func C2i06a(tta, ttb float64) [3][3]float64 {
	var rc2i [3][3]float64
	gofa.C2i06a(tta, ttb, &rc2i)

	return rc2i
}

// Sp00 returns the TIO locator s' (radians) for the given TT two-part
// Julian date.
func Sp00(tta, ttb float64) float64 {
	return gofa.Sp00(tta, ttb)
}

// Pom00 returns the polar-motion matrix for polar coordinates xp, yp
// (radians) and TIO locator sp (radians) — the other slow factor of C2t06a.
func Pom00(xp, yp, sp float64) [3][3]float64 {
	var rpom [3][3]float64
	gofa.Pom00(xp, yp, sp, &rpom)

	return rpom
}

// C2tcio assembles the celestial-to-terrestrial matrix from a cached
// celestial-to-intermediate matrix, a fresh Earth Rotation Angle, and a
// cached polar-motion matrix. Composing C2i06a + Era00 + Pom00 this way is
// bit-identical to C2t06a at the same instant, but lets a caller hold rc2i
// and rpom fixed across a series of nearby instants and recompute only era —
// the fast path C2t06a's own SOFA documentation describes for exactly this.
func C2tcio(rc2i [3][3]float64, era float64, rpom [3][3]float64) [3][3]float64 {
	var rc2t [3][3]float64
	gofa.C2tcio(rc2i, era, rpom, &rc2t)

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
func Apco13(utc1, utc2, dut1, elong, phi, hm, xp, yp, phpa, tc, rh, wl float64) (ASTROM, float64) {
	var (
		astrom ASTROM
		eo     float64
	)
	gofa.Apco13(utc1, utc2, dut1, elong, phi, hm, xp, yp, phpa, tc, rh, wl, &astrom, &eo)

	return astrom, eo
}

// Atciq provides quick ICRS to CIRS transformation given precomputed ASTROM parameters.
func Atciq(rc, dc, pr, pd, px, rv float64, astrom *ASTROM) (ri, di float64) {
	gofa.Atciq(rc, dc, pr, pd, px, rv, astrom, &ri, &di)
	return ri, di
}

// Atioq provides quick CIRS to observed place transformation utilizing precomputed configurations.
func Atioq(ri, di float64, astrom *ASTROM) (aob, zob, hob, dob, rob float64) {
	gofa.Atioq(ri, di, astrom, &aob, &zob, &hob, &dob, &rob)
	return aob, zob, hob, dob, rob
}

// Atcoq collapses Atciq and Atioq: quick ICRS to observed.
func Atcoq(rc, dc, pr, pd, px, rv float64, astrom *ASTROM) (aob, zob, hob, dob, rob float64) {
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

// Pmsafe applies stellar space motion (proper motion, parallax, radial
// velocity) to propagate a catalog position from one epoch to another,
// guarding against the near-zero-parallax case that would otherwise make
// the underlying relativistic iteration fail outright.
//
// ra1, dec1 are in radians; pmr1, pmd1 are coordinate proper motions in
// radians/Julian-year (already ×cos(dec)); px1 is parallax in arcseconds;
// rv1 is radial velocity in km/s (positive receding). ep1a/ep1b and
// ep2a/ep2b are the "from" and "to" epochs as two-part TDB Julian dates.
//
// status: 0 = OK; 1 = parallax overridden (was zero/negative/too small);
// 2 = proper motion implied >1% c, treated as zero; 4 = did not converge;
// -1 = system error (bad parallax data, e.g. NaN). Bits combine (3 = both
// 1 and 2); treat any status >= 0 as "propagated, inputs adjusted", only
// -1 as a hard failure.
func Pmsafe(ra1, dec1, pmr1, pmd1, px1, rv1, ep1a, ep1b, ep2a, ep2b float64) (ra2, dec2, pmr2, pmd2, px2, rv2 float64, status int) {
	status = gofa.Pmsafe(ra1, dec1, pmr1, pmd1, px1, rv1, ep1a, ep1b, ep2a, ep2b, &ra2, &dec2, &pmr2, &pmd2, &px2, &rv2)

	return ra2, dec2, pmr2, pmd2, px2, rv2, status
}

// Epb2jd converts a Besselian epoch (e.g. 1875.0) to a two-part Julian
// date (djm0, djm); djm0 is always 2400000.5. Needed for classical
// (pre-IAU 2006) epochs like B1875.0, the equinox the official IAU
// constellation boundaries are defined against.
func Epb2jd(epb float64) (djm0, djm float64) {
	gofa.Epb2jd(epb, &djm0, &djm)
	return djm0, djm
}

// Pmat76 returns the IAU 1976 precession rotation matrix from J2000.0 to
// the TT epoch given by the two-part Julian date (date1, date2).
func Pmat76(date1, date2 float64) [3][3]float64 {
	var rmatp [3][3]float64
	gofa.Pmat76(date1, date2, &rmatp)

	return rmatp
}

// Rxp rotates the vector p by the matrix r, returning r·p.
func Rxp(r [3][3]float64, p [3]float64) [3]float64 {
	var rp [3]float64
	gofa.Rxp(r, p, &rp)

	return rp
}

// Fk425 converts B1950.0 FK4 star data to J2000.0 FK5, with the full
// six-element transformation: the E-terms of aberration are removed and FK4's
// fictitious proper motion — the drift of its non-inertial equinox — is
// subtracted. Proper motions are radians per Julian year, parallax in arcsec,
// radial velocity in km/s.
func Fk425(r1950, d1950, dr1950, dd1950, p1950, v1950 float64) (r2000, d2000, dr2000, dd2000, p2000, v2000 float64) {
	gofa.Fk425(r1950, d1950, dr1950, dd1950, p1950, v1950,
		&r2000, &d2000, &dr2000, &dd2000, &p2000, &v2000)

	return r2000, d2000, dr2000, dd2000, p2000, v2000
}

// Fk524 is the inverse of [Fk425]: J2000.0 FK5 to B1950.0 FK4.
func Fk524(r2000, d2000, dr2000, dd2000, p2000, v2000 float64) (r1950, d1950, dr1950, dd1950, p1950, v1950 float64) {
	gofa.Fk524(r2000, d2000, dr2000, dd2000, p2000, v2000,
		&r1950, &d1950, &dr1950, &dd1950, &p1950, &v1950)

	return r1950, d1950, dr1950, dd1950, p1950, v1950
}

// Fk45z converts a B1950.0 FK4 position with *no* proper motion to J2000.0
// FK5, given the Besselian epoch the FK4 position was determined at.
//
// Not the same as [Fk425] with zero proper motion, and the difference is not
// small: a star at rest in FK4 is moving in FK5, because FK4's equinox drifts.
// This routine supplies that fictitious motion; passing zero to Fk425 asserts
// the star really has none in the inertial sense, which is a different claim.
func Fk45z(r1950, d1950, bepoch float64) (r2000, d2000 float64) {
	gofa.Fk45z(r1950, d1950, bepoch, &r2000, &d2000)

	return r2000, d2000
}

// Fk54z is the inverse of [Fk45z]: a J2000.0 FK5 position with no proper
// motion to B1950.0 FK4 at the given Besselian epoch. It returns the
// fictitious proper motion the FK4 position acquires.
func Fk54z(r2000, d2000, bepoch float64) (r1950, d1950, dr1950, dd1950 float64) {
	gofa.Fk54z(r2000, d2000, bepoch, &r1950, &d1950, &dr1950, &dd1950)

	return r1950, d1950, dr1950, dd1950
}

// Fk52h converts J2000.0 FK5 star data to the Hipparcos frame, which is the
// ICRS as realised by the Hipparcos catalogue.
func Fk52h(r5, d5, dr5, dd5, px5, rv5 float64) (rh, dh, drh, ddh, pxh, rvh float64) {
	gofa.Fk52h(r5, d5, dr5, dd5, px5, rv5, &rh, &dh, &drh, &ddh, &pxh, &rvh)

	return rh, dh, drh, ddh, pxh, rvh
}

// H2fk5 is the inverse of [Fk52h]: Hipparcos (ICRS) star data to J2000.0 FK5.
func H2fk5(rh, dh, drh, ddh, pxh, rvh float64) (r5, d5, dr5, dd5, px5, rv5 float64) {
	gofa.H2fk5(rh, dh, drh, ddh, pxh, rvh, &r5, &d5, &dr5, &dd5, &px5, &rv5)

	return r5, d5, dr5, dd5, px5, rv5
}

// Fk5hz converts a J2000.0 FK5 position with no proper motion to Hipparcos
// (ICRS), at the TT epoch given by the two-part Julian date. The frames differ
// by a small rotation *and* a spin, so the epoch matters.
func Fk5hz(r5, d5, date1, date2 float64) (rh, dh float64) {
	gofa.Fk5hz(r5, d5, date1, date2, &rh, &dh)

	return rh, dh
}

// Hfk5z is the inverse of [Fk5hz]: a Hipparcos (ICRS) position with no proper
// motion to J2000.0 FK5, returning the proper motion the FK5 position acquires
// from the frame spin.
func Hfk5z(rh, dh, date1, date2 float64) (r5, d5, dr5, dd5 float64) {
	gofa.Hfk5z(rh, dh, date1, date2, &r5, &d5, &dr5, &dd5)

	return r5, d5, dr5, dd5
}

// Pvtob returns the observer's position and velocity with respect to the
// celestial intermediate reference system, in metres and metres per second.
//
// It is the routine iauApio uses to derive the diurnal aberration magnitude.
// iauApco13 sets that magnitude to zero instead, because on its path the
// observer's rotation velocity is already inside ASTROM.V and Atciq applies
// it — so a caller reducing a position vector by rotation alone, with no
// Atciq step, has to obtain it here.
//
// Arguments are geodetic longitude, latitude (radians) and height (metres),
// polar motion xp, yp (radians), the TIO locator sp (radians) and the Earth
// rotation angle theta (radians).
func Pvtob(elong, phi, hm, xp, yp, sp, theta float64) [2][3]float64 {
	var pv [2][3]float64

	gofa.Pvtob(elong, phi, hm, xp, yp, sp, theta, &pv)

	return pv
}

// Ld applies light deflection by one gravitating body, returning the deflected
// direction from the observer to the source.
//
// bm is the body's mass in solar masses; p is the observer-to-source unit
// vector, q the body-to-source unit vector, e the body-to-observer unit
// vector, em their separation in AU, and dlim the deflection limiter phi^2/2.
//
// The returned vector is not exactly unit — SOFA states the departure is
// always negligible — so a caller needing a length should restore it.
func Ld(bm float64, p, q, e [3]float64, em, dlim float64) [3]float64 {
	var p1 [3]float64

	gofa.Ld(bm, p, q, e, em, dlim, &p1)

	return p1
}

// TTToTCG converts Terrestrial Time to Geocentric Coordinate Time.
//
// TCG is the coordinate time of the geocentric reference system: TT is TCG
// rescaled so that it ticks at the rate of a clock on the rotating geoid, which
// is what makes TT the scale a terrestrial observation is timed in and TCG the
// one a geocentric equation of motion is integrated in. The two differ by a
// secular rate of L_G = 6.969290134e-10, about 22 ms per year, zero at
// 1977-01-01 by definition.
func TTToTCG(tt1, tt2 float64) (tcg1, tcg2 float64, status int) {
	status = gofa.Tttcg(tt1, tt2, &tcg1, &tcg2)

	return tcg1, tcg2, status
}

// TCGToTT converts Geocentric Coordinate Time to Terrestrial Time.
func TCGToTT(tcg1, tcg2 float64) (tt1, tt2 float64, status int) {
	status = gofa.Tcgtt(tcg1, tcg2, &tt1, &tt2)

	return tt1, tt2, status
}

// TDBToTCB converts Barycentric Dynamical Time to Barycentric Coordinate Time.
//
// TCB is to TDB what TCG is to TT: the unscaled coordinate time of the
// barycentric system. The rate difference is L_B = 1.550519768e-8, about
// half a second per year, which is why an ephemeris argument is TDB and not
// TCB — TDB was defined to stay close to TT.
func TDBToTCB(tdb1, tdb2 float64) (tcb1, tcb2 float64, status int) {
	status = gofa.Tdbtcb(tdb1, tdb2, &tcb1, &tcb2)

	return tcb1, tcb2, status
}

// TCBToTDB converts Barycentric Coordinate Time to Barycentric Dynamical Time.
func TCBToTDB(tcb1, tcb2 float64) (tdb1, tdb2 float64, status int) {
	status = gofa.Tcbtdb(tcb1, tcb2, &tdb1, &tdb2)

	return tdb1, tdb2, status
}
