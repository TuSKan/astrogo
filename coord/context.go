package coord

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/internal/refraction"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// Context encapsulates the observation environment (time and location) and precomputes
// the computationally expensive SOFA intermediate astrometry parameters (ASTROM),
// the C2t06a ICRS↔TIRS rotation matrix, and the geocentric observer ICRS vector.
//
// All heavy matrix work is done once at construction. Both stellar paths
// (via Atciq/Atioq) and planetary paths (via the cached C2t06a matrix)
// benefit from the precomputation.
//
// # One Context, moved through time
//
// A Context holds two kinds of state. Precession-nutation, the Earth's
// position and velocity, and polar motion come from series, at the cost of a
// full SOFA evaluation, and move by a fraction of an arcsecond an hour. Earth
// rotation, and everything that turns with it — the observer's position and
// velocity, the celestial-to-terrestrial matrix — move 15″ a second.
// [Context.SetTime] moves a Context to another instant by recomputing the
// second kind at that instant and interpolating the first between two
// evaluations a day apart, so the series are evaluated about once a day of
// movement. It never allocates.
//
// A Context is a value: c := *ctx is an independent copy, for a caller that
// needs two instants at once.
type Context struct {
	t time.Time

	// nodes are the instants the slow state has been evaluated at, in time
	// order, and what was evaluated there: one after a build, and two, a
	// bracket apart, once SetTime has moved further than holdFor from it.
	// SetTime interpolates between the two; see [Context.SetTime].
	nodes  [2]node
	nnodes int

	// refa and refb are the refraction constants SOFA's astrometry carries,
	// which depend on the atmosphere alone.
	refa, refb float64

	site   *Geodetic
	atm    atmosphere.Refraction
	astrom gofaext.ASTROM
	eop    time.EOP // cached for AltAzToICRS reuse

	// Precomputed geocentric reduction fields (for GeocentricToObserved).
	// These avoid rebuilding the C2t06a matrix + observer vector per call.
	mat    [3][3]float64 // ICRS → ITRS rotation matrix; see [Context.ICRSToITRS]
	obsVec vector.Vec3   // observer position in ICRS frame (AU)

	// rc2i (precession-nutation) and rpom (polar motion) are the slow
	// factors C2t06a composes internally (mat = C2tcio(rc2i, era, rpom)).
	// Cached so SetTime can recompute mat from a fresh era alone.
	rc2i [3][3]float64
	rpom [3][3]float64

	// Cached site trigonometry (computed once).
	sinLat, cosLat float64
	sinLon, cosLon float64

	// tirs is the observer's position in the terrestrial frame (AU), which
	// depends on the site alone; obsVec is it rotated into ICRS at t.
	tirs vector.Vec3

	// diurab is the magnitude of the diurnal aberration vector, in units of
	// c: the observer's eastward rotation speed divided by the speed of
	// light, 0.32 arcsec at the equator falling as cos(latitude).
	//
	// It is not read from ASTROM, which holds zero. Apco13 zeroes it
	// deliberately, because on the stellar path the observer's rotation
	// velocity is already inside ASTROM.V and Atciq applies it there; Atioq
	// must not apply it a second time. The vector path has no Atciq step, so
	// it is the one place the term has to be supplied — see
	// [Context.GeocentricToObserved].
	diurab float64

	// eo is the equation of the origins at this epoch, in radians:
	// ERA minus GST, which is the precession in right ascension since
	// J2000.0 plus the equation of the equinoxes. About -0.33 degrees in
	// 2026 and growing by 46 arcseconds a year. See [Context.CIRSToTETE].
	eo float64
}

// NewContext prepares the astrometry parameters for a specific observer time
// and site. Callers may pass any time scale.
//
// # One time scale per Context
//
// Everything a Context holds is built from one TT, t.TT(), and one UT1,
// t.UT1Using with the epoch's DUT1: the astrometry the stellar path reads, the
// celestial-to-intermediate matrix the vector path and SetTime read, and the
// Earth rotation both apply.
//
// It used to hand SOFA's Apco13 the UTC date and let it derive its own TT and
// UT1 from SOFA's TAI−UTC table, while deriving astrogo's for everything else.
// From 1972 on the two agree. Before, they did not: astrogo's ΔT against
// SOFA's TAI−UTC put the TTs up to 34 s apart, and on the eleven days SOFA
// stretches for a step in TAI−UTC (1959-12-31, ten in the 1960s, 1971-12-31)
// the UT1s differed by up to 0.82 s, so a star and the Moon in one Context
// were rotated by different Earths, 11.8″ apart (#474).
func NewContext(t time.Time, site *Geodetic, atm atmosphere.Refraction) *Context {
	ctx := &Context{site: site, atm: atm}
	ctx.sinLat, ctx.cosLat = math.Sincos(site.Lat().Radians())
	ctx.sinLon, ctx.cosLon = math.Sincos(site.Lon().Radians())
	ctx.tirs = tirsVec(ctx.sinLat, ctx.cosLat, ctx.sinLon, ctx.cosLon, site.Height().Meters())

	// A custom model refracts outside SOFA, so SOFA is given no atmosphere;
	// see [Context.refract].
	p := atm.Pressure
	if atm.Model != nil {
		p = 0.0
	}

	ctx.refa, ctx.refb = gofaext.Refco(p, atm.Temperature, atm.Humidity, atm.Wavelength)
	ctx.build(t)

	return ctx
}

// kmPerAU is the number of kilometers in one Astronomical Unit.
//
// var, not const: constants.IAU.AstronomicalUnit is a struct, and Go does not
// permit selecting a struct field inside a constant expression.
var kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3

// tirsVec returns the observer's geocentric position in the TIRS frame
// (AU) from site trigonometry and height — pure site geometry, independent
// of time, computed once by NewContext.
func tirsVec(sinLat, cosLat, sinLon, cosLon, heightM float64) vector.Vec3 {
	// rEq and f are WGS84 reference-ellipsoid parameters, which belong to the
	// geodesy here rather than to constants. au is a unit conversion with a
	// canonical home, so it comes from there — see kmPerAU.
	const (
		rEq = 6378.137
		f   = 1.0 / 298.257223563
	)

	au := kmPerAU

	cEarth := 1.0 / math.Sqrt(cosLat*cosLat+(1.0-f)*(1.0-f)*sinLat*sinLat)
	sEarth := (1.0 - f) * (1.0 - f) * cEarth
	heightKm := heightM / 1000.0

	return vector.Vec3{
		X: (rEq*cEarth + heightKm) * cosLat * cosLon / au,
		Y: (rEq*cEarth + heightKm) * cosLat * sinLon / au,
		Z: (rEq*sEarth + heightKm) * sinLat / au,
	}
}

// icrsFromTIRS rotates a TIRS-frame vector into the ICRS frame:
// v_ICRS = transpose(mat) * v_TIRS.
func icrsFromTIRS(mat [3][3]float64, tirs vector.Vec3) vector.Vec3 {
	return vector.Vec3{
		X: mat[0][0]*tirs.X + mat[1][0]*tirs.Y + mat[2][0]*tirs.Z,
		Y: mat[0][1]*tirs.X + mat[1][1]*tirs.Y + mat[2][1]*tirs.Z,
		Z: mat[0][2]*tirs.X + mat[1][2]*tirs.Y + mat[2][2]*tirs.Z,
	}
}

// bracket and holdFor are how SetTime spends series evaluations, and both
// follow from how fast the slow state moves. Its fastest-curving term is
// nutation's 13.66-day term, 0.23″ in longitude; after it comes the annual
// aberration, 20.5″ over a year. Interpolated linearly across a bracket L, a
// term of amplitude A and angular frequency ω errs by at most A·ω²·L²/8: for a
// day, 0.006″ from that nutation term and 0.0008″ from aberration, ≲0.007″.
// Held fixed instead, the state moves by at most the sum of A·ω, with
// precession ≲0.026″ an hour, so a quarter of an hour holds it inside the
// same 0.007″.
var (
	bracket = unit.Days(1)
	holdFor = unit.Minutes(15)
)

// SetTime moves ctx to instant t. It never allocates.
//
// Everything that turns with the Earth is recomputed at t: the Earth
// rotation angle, the observer's position and velocity — and with that
// velocity the diurnal aberration of the stellar path — the
// celestial-to-terrestrial matrix and the observer vector. That is SOFA's
// Apco, about a microsecond, with no series in it.
//
// The rest comes from series: the Earth's position and velocity, the CIP and
// the CIO locator, the equation of the origins, polar motion. SetTime
// evaluates them at two instants a day apart, a bracket, and interpolates
// across it, which is good to ≲0.007″ (see bracket). It moves the bracket a
// day at a time as t leaves it, one evaluation a day, and starts a new one at
// t when t is further than that. Within a quarter of an hour of where ctx was
// built it holds the evaluation there rather than making a second; see
// holdFor. At an instant where the series were evaluated, ctx is what
// [NewContext] builds there.
//
// It used to hold the series' state fixed for an hour and evaluate it again
// an hour out: 24 evaluations in a day of steps, at 0.1 ms each, and a drift
// of up to 0.07″, three quarters of it the observer's velocity held at the
// epoch's Earth rotation angle while the Earth turned (#790).
//
// It replaces AtTime, which returned a new Context for every instant, 704
// bytes on the heap to change four values, and left the rebuild to each
// caller (#675).
//
// SetTime writes ctx, so it must not run while anything else reads or moves
// the same Context; see the package's Concurrency section.
//
// # DUT1 is t's own, not interpolated
//
// Across a leap second DUT1 = UT1 − UTC jumps by exactly one second, because
// UTC steps back and UT1 does not, so reusing an earlier instant's put every
// instant after the leap a second early in Earth rotation — 13″ for a star at
// −30° (#489). Looking DUT1 up at t, as NewContext does, costs one
// interpolation.
func (ctx *Context) SetTime(t time.Time) {
	t = t.UTC()
	lo, hi := &ctx.nodes[0], &ctx.nodes[1]
	fromLo, fromHi := utcDays(t, lo.at), utcDays(t, hi.at)

	switch {
	case ctx.nnodes == 2 && fromLo >= 0 && fromHi <= 0:
		// Inside the bracket.
	case ctx.nnodes == 1 && math.Abs(fromLo) <= holdFor.Days():
		ctx.place(t, lo)

		return
	case ctx.nnodes == 1 && math.Abs(fromLo) <= bracket.Days():
		if fromLo > 0 {
			*hi = nodeAt(lo.at.Add(bracket))
		} else {
			*hi = *lo
			*lo = nodeAt(hi.at.Add(-bracket))
		}

		ctx.nnodes = 2
	case ctx.nnodes == 2 && fromHi > 0 && fromHi <= bracket.Days():
		*lo = *hi
		*hi = nodeAt(lo.at.Add(bracket))
	case ctx.nnodes == 2 && fromLo < 0 && -fromLo <= bracket.Days():
		*hi = *lo
		*lo = nodeAt(hi.at.Add(-bracket))
	default:
		ctx.build(t)

		return
	}

	n := between(lo, hi, utcDays(t, lo.at)/utcDays(hi.at, lo.at))
	ctx.place(t, &n)
}

// utcDays is a − b in days of UTC labels, for two UTC instants: their
// elapsed time, except across a leap second, which it leaves out. SetTime
// uses it where a second does not matter — to choose nodes and weigh them,
// and to carry TT for proper motion — because Time.Sub, which does count
// the second, looks both instants up in the leap-second table, and that was
// most of a move.
func utcDays(a, b time.Time) float64 {
	a1, a2 := a.JDParts()
	b1, b2 := b.JDParts()

	return (a1 - b1) + (a2 - b2)
}

// node is the series' state at one instant: what [gofaext.CelestialAt]
// evaluates there, that instant's polar motion and its matrix, and its TT.
type node struct {
	at       time.Time
	tt1, tt2 float64
	cel      gofaext.Celestial
	xp, yp   float64
	rpom     [3][3]float64
}

func nodeAt(t time.Time) node {
	tt1, tt2 := t.TT().JDParts()
	eop := t.EOP()

	n := node{at: t, tt1: tt1, tt2: tt2, cel: gofaext.CelestialAt(tt1, tt2), xp: eop.XP, yp: eop.YP}
	n.rpom = gofaext.Pom00(n.xp, n.yp, n.cel.Sp)

	return n
}

// between is the series' state a fraction f of the way from a to b, linearly.
// It is a's at f = 0 and b's at f = 1, exactly. Its instant and TT stay a's,
// which place moves on from.
func between(a, b *node, f float64) node {
	l := func(p, q float64) float64 { return p*(1-f) + q*f }

	n := node{at: a.at, tt1: a.tt1, tt2: a.tt2, xp: l(a.xp, b.xp), yp: l(a.yp, b.yp)}
	c, ca, cb := &n.cel, &a.cel, &b.cel

	for i := range 2 {
		for j := range 3 {
			c.Ebpv[i][j] = l(ca.Ebpv[i][j], cb.Ebpv[i][j])
		}
	}

	for j := range 3 {
		c.Ehp[j] = l(ca.Ehp[j], cb.Ehp[j])

		for k := range 3 {
			n.rpom[j][k] = l(a.rpom[j][k], b.rpom[j][k])
		}
	}

	c.X, c.Y, c.S = l(ca.X, cb.X), l(ca.Y, cb.Y), l(ca.S, cb.S)
	c.EO, c.Sp = l(ca.EO, cb.EO), l(ca.Sp, cb.Sp)

	return n
}

// Time returns the encapsulated observation time.
func (ctx *Context) Time() time.Time { return ctx.t }

// Site returns the encapsulated observation geodetic location.
func (ctx *Context) Site() *Geodetic { return ctx.site }

// Refraction returns the encapsulated refraction configuration. Renamed
// from Atmosphere alongside atmosphere.Atmosphere/Refraction's own swap
// (see atmosphere/doc.go) — this has always returned the refraction-input
// struct, never the package's richer atmospheric-state type.
func (ctx *Context) Refraction() atmosphere.Refraction { return ctx.atm }

// ObsVec returns the observer's geocentric position in the ICRS frame (AU).
// This can be subtracted from a body's geocentric vector to obtain the
// topocentric position, correcting for diurnal parallax (~1° for the Moon,
// ~23″ for Mars at opposition).
func (ctx *Context) ObsVec() vector.Vec3 { return ctx.obsVec }

// ObsVel returns the observer's geocentric velocity in the ICRS frame
// (AU/day): the site carried around by the Earth's rotation, about
// 0.46 km/s·cos(latitude).
//
// It is SOFA's iauPvtob velocity in the celestial intermediate system, ω × r
// with ω the Earth's rotation rate, carried to the ICRS axes by the same
// precession-nutation matrix as [Context.ObsVec]. Its sum with the Earth's
// barycentric velocity is the velocity Apco13 aberrates with, so an observer
// built from ObsVec and ObsVel — [github.com/TuSKan/astrogo/ephemeris.Site] —
// applies annual and diurnal aberration together, in SOFA's order.
func (ctx *Context) ObsVel() vector.Vec3 {
	// The site in the celestial intermediate system, where the Earth turns
	// about z.
	m := ctx.rc2i
	o := ctx.obsVec
	x := m[0][0]*o.X + m[0][1]*o.Y + m[0][2]*o.Z
	y := m[1][0]*o.X + m[1][1]*o.Y + m[1][2]*o.Z

	// iauPvtob's OM, 1.00273781191135448 turns per UT1 day, in rad/day.
	const omega = 2 * math.Pi * 1.00273781191135448

	vx, vy := -omega*y, omega*x

	// Back to the ICRS axes: the transpose, the matrix being a rotation.
	return vector.V3(
		m[0][0]*vx+m[1][0]*vy,
		m[0][1]*vx+m[1][1]*vy,
		m[0][2]*vx+m[1][2]*vy,
	)
}

// AstrometricToCIRS computes the Celestial Intermediate Reference System (CIRS) apparent
// position of an object from its Astrometric (catalog ICRS) coordinates.
func (ctx *Context) AstrometricToCIRS(c Astrometric) CIRS {
	ri, di := ctx.atciq(c)

	return NewCIRS(angle.Rad(ri).Wrap360(), angle.Rad(di))
}

// CIRSToObserved converts geocentric CIRS coordinates to local Observed AltAz
// taking into account Earth rotation, polar motion, and atmospheric refraction.
func (ctx *Context) CIRSToObserved(c CIRS) AltAz {
	alt, az, _ := ctx.observed(c.RA().Radians(), c.Dec().Radians())

	return NewAltAz(alt, az)
}

// AstrometricToObserved collapses the entire apparent pipeline from an Astrometric catalog
// point explicitly to a refracted local AltAz position.
func (ctx *Context) AstrometricToObserved(c Astrometric) AltAz {
	alt, az, _ := ctx.observed(ctx.atciq(c))

	return NewAltAz(alt, az)
}

// GeocentricToObserved converts a geocentric ICRS position vector to local observed AltAz
// using the precomputed C2t06a matrix and observer vector cached in the Context.
// This avoids the per-call overhead of re-fetching IERS data, recomputing TT,
// and rebuilding the full rotation matrix that a fresh Reducer would incur.
//
// Three things happen to the input: the observer vector is subtracted, which is
// diurnal parallax; the result is rotated into the local horizon; and diurnal
// aberration and refraction are applied to the direction. Annual aberration and
// light deflection are not — they belong to the geocentric place, so a caller
// passes an *apparent* geocentric vector, which is what
// [github.com/TuSKan/astrogo/ephemeris] produces. Passing a vector that already
// carries the observer's own rotation velocity would double-count it; nothing
// in astrogo produces one.
//
// Atmospheric refraction is applied as on every other path; see
// [Context.refract].
//
// The result carries the topocentric distance, |v − observer|, with v read in
// AU as the observer vector is. It used to carry none: Dist() was zero, while
// ICRSToAltAz passes its input's distance through, and a satellite's
// TargetDetails read that zero as its range (#495).
func (ctx *Context) GeocentricToObserved(v vector.Vec3) AltAz {
	// Topocentric vector in ICRS frame.
	topoVec := v.Sub(ctx.obsVec)

	// Rotate ICRS → ITRS.
	tx := ctx.mat[0][0]*topoVec.X + ctx.mat[0][1]*topoVec.Y + ctx.mat[0][2]*topoVec.Z
	ty := ctx.mat[1][0]*topoVec.X + ctx.mat[1][1]*topoVec.Y + ctx.mat[1][2]*topoVec.Z
	tz := ctx.mat[2][0]*topoVec.X + ctx.mat[2][1]*topoVec.Y + ctx.mat[2][2]*topoVec.Z

	// ITRS → local horizon ENU.
	E := -ctx.sinLon*tx + ctx.cosLon*ty
	N := -ctx.sinLat*ctx.cosLon*tx - ctx.sinLat*ctx.sinLon*ty + ctx.cosLat*tz
	U := ctx.cosLat*ctx.cosLon*tx + ctx.cosLat*ctx.sinLon*ty + ctx.sinLat*tz

	E, N, U = ctx.aberrateDiurnal(E, N, U, topoVec.Norm())

	azimuth := math.Atan2(E, N)
	if azimuth < 0 {
		azimuth += 2 * math.Pi
	}

	// Atan2 rather than Asin(U): after the aberration the triple is no longer
	// exactly a unit vector, and Atioq itself takes the altitude this way.
	altitude := math.Atan2(U, math.Hypot(E, N))

	aa := NewAltAz(ctx.refract(angle.Rad(altitude)), angle.Rad(azimuth))
	aa.SetDist(unit.AU(topoVec.Norm()))

	return aa
}

// TopocentricToObserved converts the apparent place seen from this Context's
// site, a vector from the observer on the ICRS axes with light deflection and
// both annual and diurnal aberration already applied, to the local observed
// AltAz: rotated into the horizon and refracted, nothing more.
//
// It is [Context.GeocentricToObserved] without the two steps that one takes on
// a geocentric vector: no observer vector is subtracted, the input being from
// the site already, and no diurnal aberration is added, the observer's
// velocity having been in the aberration. That is SOFA's order, iauAtciq then
// iauAtioq for a star, applied to a body at a finite distance:
// [github.com/TuSKan/astrogo/ephemeris.Instant.Apparent] with
// [github.com/TuSKan/astrogo/ephemeris.Site] produces the input. Parallax taken
// after aberration, as the geocentric route does, misplaces the Moon by up to
// 0.035″ (docs/ephemeris.md §5.3).
//
// The result carries the input's length as its distance.
func (ctx *Context) TopocentricToObserved(v vector.Vec3) AltAz {
	tx := ctx.mat[0][0]*v.X + ctx.mat[0][1]*v.Y + ctx.mat[0][2]*v.Z
	ty := ctx.mat[1][0]*v.X + ctx.mat[1][1]*v.Y + ctx.mat[1][2]*v.Z
	tz := ctx.mat[2][0]*v.X + ctx.mat[2][1]*v.Y + ctx.mat[2][2]*v.Z

	E := -ctx.sinLon*tx + ctx.cosLon*ty
	N := -ctx.sinLat*ctx.cosLon*tx - ctx.sinLat*ctx.sinLon*ty + ctx.cosLat*tz
	U := ctx.cosLat*ctx.cosLon*tx + ctx.cosLat*ctx.sinLon*ty + ctx.sinLat*tz

	azimuth := math.Atan2(E, N)
	if azimuth < 0 {
		azimuth += 2 * math.Pi
	}

	altitude := math.Atan2(U, math.Hypot(E, N))

	aa := NewAltAz(ctx.refract(angle.Rad(altitude)), angle.Rad(azimuth))
	aa.SetDist(unit.AU(v.Norm()))

	return aa
}

// ICRSToAltAz converts ICRS coordinates to local observed AltAz utilizing the
// precomputed epoch pipeline matrices. If the ICRS carries stellar kinematics
// (proper motion, parallax, radial velocity), they are forwarded to SOFA for
// rigorous space-motion propagation.
func (ctx *Context) ICRSToAltAz(c ICRS) (AltAz, error) {
	altaz := ctx.AstrometricToObserved(c.Astrometric())
	altaz.SetDist(c.Dist())

	return altaz, nil
}

// ICRSToHourAngle converts ICRS coordinates to local observed Hour Angle.
// If the ICRS carries kinematics, they are forwarded to SOFA for rigorous
// space-motion propagation.
//
// Observed means refracted: the hour angle of the direction [Context.ICRSToAltAz]
// returns, rotated from the horizon to the equator as iauAtioq rotates it,
// with the latitude corrected for polar motion. Until #588 a Context with an
// explicit refraction model returned the unrefracted hour angle.
func (ctx *Context) ICRSToHourAngle(c ICRS) (angle.Angle, error) {
	_, _, ha := ctx.observed(ctx.atciq(c.Astrometric()))

	return angle.Rad(ha).Wrap180(), nil
}

// AltAzToICRS converts local observed AltAz back into geometric ICRS.
//
// It inverts with the astrometry this Context already holds, through SOFA's
// quick inverse (Atoiq, then Aticq), which is what Atoc13 does after building
// that astrometry itself. It used to call Atoc13, so every inversion evaluated
// the precession-nutation series again on a Context built to cache it, and did
// so at SOFA's own TT and UT1 rather than this Context's: on the days #474
// names, the inverse of ICRSToAltAz was not ICRSToAltAz's inverse.
func (ctx *Context) AltAzToICRS(c AltAz) (ICRS, error) {
	var ri, di float64

	switch {
	case ctx.sofaRefracts() && c.Alt().Radians() >= refraction.SOFAAbove:
		// SOFA takes out its own refraction, as on the forward path.
		ri, di = gofaext.Atoiq("A", c.Az().Radians(), math.Pi/2-c.Alt().Radians(), &ctx.astrom)
	default:
		geo := ctx.geometric()
		geomAlt := ctx.unrefract(c.Alt())
		ri, di = gofaext.Atoiq("A", c.Az().Radians(), math.Pi/2-geomAlt.Radians(), &geo)
	}

	ra, dec := gofaext.Aticq(ri, di, &ctx.astrom)

	return NewICRS(angle.Rad(ra).Wrap360(), angle.Rad(dec)), nil
}

// BarycentricVelocity returns the observer's velocity relative to the
// solar system barycenter, in km/s, ICRS-aligned. This is a unit
// conversion of ctx's already-cached astrometry parameters
// (ASTROM.V, in units of c, built once at Context construction by
// Apco13) — not a new computation. It includes both Earth's own
// barycentric orbital velocity and the observing site's diurnal
// rotation velocity, since Apco13 builds V topocentrically.
//
// See coord/radialvelocity.go for the radial-velocity corrections this
// exists to support.
func (ctx *Context) BarycentricVelocity() vector.Vec3 {
	kmPerSec := constants.SI2019.SpeedOfLight.Value / 1000.0

	return vector.V3(ctx.astrom.V[0], ctx.astrom.V[1], ctx.astrom.V[2]).MulScalar(kmPerSec)
}

// place sets everything in ctx that turns with the Earth, at t, from the
// series' state n: the astrometry, with the observer's position and velocity
// at t's Earth rotation angle, and the celestial-to-terrestrial matrix and
// observer vector.
func (ctx *Context) place(t time.Time, n *node) {
	// UT1 through time rather than by adding DUT1 to the UTC Julian Date: on
	// a day that ends in a leap second that date's fraction is of 86401
	// seconds, and the sum would misplace Earth's rotation by up to a second.
	ut1, ut2 := t.UT1Using(t.EOP().DUT1).JDParts()
	era := gofaext.Era00(ut1, ut2)
	site := ctx.site

	// TT moves uniformly, so it is the node's and the time since: no
	// leap-second lookup at every move, which was a quarter of one. Apco reads
	// it for one thing, the interval stars' proper motion is applied over,
	// which a second across a leap second does not move.
	tt1, tt2 := n.tt1, n.tt2+utcDays(t, n.at)

	// SOFA's Apco13 given this Context's TT and UT1 rather than deriving its
	// own (#474), and the series' state rather than evaluating it here.
	ctx.astrom = gofaext.ApcoFrom(&n.cel, tt1, tt2, era,
		site.Lon().Radians(), site.Lat().Radians(), site.Height().Meters(),
		n.xp, n.yp, ctx.refa, ctx.refb)

	// The celestial-to-intermediate matrix, which Apco has already built as
	// astrom.Bpn: evaluating it again with C2i06a cost a third of every
	// NewContext (#473). C2tcio(rc2i, era, rpom) is bit-identical to a direct
	// C2t06a call at the same TT and UT1.
	ctx.t = t
	ctx.eo = n.cel.EO
	ctx.rc2i = ctx.astrom.Bpn
	ctx.rpom = n.rpom
	ctx.mat = gofaext.C2tcio(ctx.rc2i, era, ctx.rpom)
	ctx.obsVec = icrsFromTIRS(ctx.mat, ctx.tirs)
}

// build computes, in place, everything in ctx for instant t, evaluating the
// series there: what NewContext does once the site is set, and what SetTime
// does again when t is beyond the bracket next to the one it has.
func (ctx *Context) build(t time.Time) {
	t = t.UTC()
	site := ctx.site

	// The series, evaluated at t, and the only node until SetTime needs a
	// second. Its equation of the origins, the angle between the true equinox
	// and the CIO, is what separates the CIRS place this Context computes from
	// the equinox-based apparent place an almanac quotes; see
	// [Context.CIRSToTETE].
	ctx.nodes[0], ctx.nnodes = nodeAt(t), 1
	ctx.eop = t.EOP()
	ctx.place(t, &ctx.nodes[0])

	// Diurnal aberration, derived exactly as iauApio does: the horizontal
	// part of the observer's velocity in the celestial intermediate system,
	// over c. Its size depends on the site alone, the Earth's rotation turning
	// it without changing it, so it is taken once, at rotation angle zero.
	n := &ctx.nodes[0]
	pvob := gofaext.Pvtob(
		site.Lon().Radians(), site.Lat().Radians(), site.Height().Meters(),
		n.xp, n.yp, n.cel.Sp, 0,
	)
	ctx.diurab = math.Hypot(pvob[1][0], pvob[1][1]) / constants.SI2019.SpeedOfLight.Value
}

// refract carries a true altitude to the observed one under this Context's
// refraction: an explicit model's, or for none, [atmosphere.RefractionSOFA]
// from the constants in the astrometry, which is SOFA's series exactly as
// iauAtioq applies it wherever the observed altitude is above 10°, handed over
// to Bennett-NA below. A zero pressure is no atmosphere.
//
// The vector path always comes here. The stellar paths let SOFA's own
// routines refract where they agree with this, above 10° of observed
// altitude, which keeps the common case as fast as SOFA; below it they redo
// the place with SOFA's refraction switched off and come here, so near the
// horizon the two paths agree by construction.
//
// # History
//
// The stellar path used to leave refraction to iauAtioq, and the vector path
// reproduced Atioq's arithmetic to match it. Before that reproduction it had
// no clamp at all, and at −0.076° returned an altitude of +7028° (#100). And
// SOFA's clamp held every altitude below 2.87° at the refraction there, so a
// set target was still raised 0.17° and the horizon got 10′ where the
// almanacs give 34′ (#588).
func (ctx *Context) refract(alt angle.Angle) angle.Angle {
	switch {
	case ctx.atm.Model != nil:
		return alt + ctx.atm.Model.RefractFromTrue(alt, ctx.atm)
	case ctx.atm.Pressure > 0:
		return alt + angle.Rad(refraction.FromTrue(alt.Radians(), ctx.astrom.Refa, ctx.astrom.Refb,
			ctx.atm.Pressure, ctx.atm.Temperature, ctx.atm.Wavelength))
	default:
		return alt
	}
}

// atciq is SOFA's Atciq for a catalog place: its CIRS right ascension and
// declination, in radians, unwrapped, as Atcoq hands them on to Atioq.
func (ctx *Context) atciq(c Astrometric) (ri, di float64) {
	return gofaext.Atciq(
		c.RA().Radians(), c.Dec().Radians(),
		// SOFA wants dRA/dt; this package stores the catalogue's on-sky
		// rate. See [dRAdt].
		dRAdt(c.PmRA(), c.Dec()), c.PmDec().Radians(), c.Parallax().Radians(), c.RV().KmPerSec(),
		&ctx.astrom,
	)
}

// observed is SOFA's Atioq for a CIRS place, in radians: the observed
// altitude, azimuth and hour angle under this Context's refraction.
//
// Where SOFA refracts for this Context and the observed altitude is above
// 10°, Atioq's own answer is it, so the common case costs one Atioq. Below,
// the place is taken again with SOFA's refraction switched off and refracted
// by [Context.refract], at the cost of a second Atioq; Atciq, the expensive
// half of Atcoq, is not repeated.
func (ctx *Context) observed(ri, di float64) (alt, az angle.Angle, ha float64) {
	a, zd, h, _, _ := gofaext.Atioq(ri, di, &ctx.astrom)

	if ctx.sofaRefracts() {
		if obs := math.Pi/2 - zd; obs >= refraction.SOFAAbove {
			return angle.Rad(obs), angle.Rad(a).Wrap360(), h
		}

		geo := ctx.geometric()
		a, zd, _, _, _ = gofaext.Atioq(ri, di, &geo)
	}

	alt = ctx.refract(angle.Rad(math.Pi/2 - zd))

	// The hour angle of the refracted direction, rotated from the horizon
	// to the equator as Atioq rotates it, in its frame (x south, y east,
	// z up), with the latitude corrected for polar motion.
	sinAlt, cosAlt := math.Sincos(alt.Radians())
	sinAz, cosAz := math.Sincos(a)
	x, y, z := -cosAz*cosAlt, sinAz*cosAlt, sinAlt

	return alt, angle.Rad(a).Wrap360(), -math.Atan2(y, ctx.astrom.Sphi*x+ctx.astrom.Cphi*z)
}

// sofaRefracts reports whether SOFA's own routines refract for this Context:
// the default model, with an atmosphere.
func (ctx *Context) sofaRefracts() bool {
	return ctx.atm.Model == nil && ctx.atm.Pressure > 0
}

// geometric is this Context's astrometry with SOFA's refraction switched off.
func (ctx *Context) geometric() gofaext.ASTROM {
	geo := ctx.astrom
	geo.Refa, geo.Refb = 0, 0

	return geo
}

// unrefract is [Context.refract]'s inverse: the true altitude of an observed
// one.
func (ctx *Context) unrefract(alt angle.Angle) angle.Angle {
	switch {
	case ctx.atm.Model != nil:
		return alt - ctx.atm.Model.RefractFromApparent(alt, ctx.atm)
	case ctx.atm.Pressure > 0:
		return alt - angle.Rad(refraction.FromApparent(alt.Radians(), ctx.astrom.Refa, ctx.astrom.Refb,
			ctx.atm.Pressure, ctx.atm.Temperature, ctx.atm.Wavelength))
	default:
		return alt
	}
}

// aberrateDiurnal applies diurnal aberration to a topocentric ENU direction,
// returning the shifted components as a near-unit vector.
//
// # Why the vector path needs this and the stellar path does not
//
// Diurnal aberration is the observer's own rotation velocity — 465 m/s
// eastward at the equator, 0.32 arcseconds of displacement, falling as
// cos(latitude). It has to enter somewhere, and SOFA lets it enter at either
// of two places depending on which route a caller takes.
//
// On the stellar route, Apco13 puts the observer's *full* barycentric
// velocity into ASTROM.V — orbital motion and rotation together — so Atciq's
// aberration step already carries the diurnal part. Apco13 therefore sets
// ASTROM.Diurab to zero, and Atioq adds nothing. Apio13, which serves a
// CIRS-to-observed call with no Atciq before it, does the opposite: it sets
// Diurab and lets Atioq apply it.
//
// [Context.GeocentricToObserved] is the second case. It receives a geocentric
// place and reduces it by rotation and translation alone, with no Atciq step
// anywhere, so nothing on that path had applied the term. Until #261 it was
// simply absent, and the two routes disagreed by up to 0.32 arcseconds for
// the same target — measured at 0.3150" at the equator, 0.1966" at Greenwich
// and 0.0647" at 78N, tracking 0.32"·cos(latitude) to within 2%.
//
// # The arithmetic
//
// Taken from iauAtioq rather than re-derived, in the same form. Atioq works
// in a Cartesian -HA/Dec frame whose y axis is local east, and reaches the
// horizon frame by a rotation about that same axis, so the shift is identical
// expressed in ENU: y there is E here.
//
//	f = 1 - diurab*E
//	E' = f*(E + diurab),  N' = f*N,  U' = f*U
//
// The result is not renormalized, again as Atioq leaves it — the magnitude
// differs from one by about 1.5e-6, and the callers here take an atan2.
func (ctx *Context) aberrateDiurnal(e, n, u, norm float64) (float64, float64, float64) {
	if norm == 0 {
		return e, n, u
	}

	e, n, u = e/norm, n/norm, u/norm

	f := 1.0 - ctx.diurab*e

	return f * (e + ctx.diurab), f * n, f * u
}
