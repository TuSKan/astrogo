package coord

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Context encapsulates the observation environment (time and location) and precomputes
// the computationally expensive SOFA intermediate astrometry parameters (ASTROM),
// the C2t06a ICRS↔TIRS rotation matrix, and the geocentric observer ICRS vector.
//
// All heavy matrix work is done once at construction. Both stellar paths
// (via Atciq/Atioq) and planetary paths (via the cached C2t06a matrix)
// benefit from the precomputation.
type Context struct {
	t      time.Time
	site   *Geodetic
	atm    atmosphere.Refraction
	astrom gofaext.ASTROM
	eop    time.EOP // cached for AltAzToICRS reuse

	// Precomputed geocentric reduction fields (for GeocentricToObserved).
	// These avoid rebuilding the C2t06a matrix + observer vector per call.
	mat    [3][3]float64 // ICRS → TIRS rotation matrix
	obsVec vector.Vec3   // observer position in ICRS frame (AU)

	// rc2i (precession-nutation) and rpom (polar motion) are the slow
	// factors C2t06a composes internally (mat = C2tcio(rc2i, era, rpom)).
	// Cached so AtTime can recompute mat from a fresh era alone.
	rc2i [3][3]float64
	rpom [3][3]float64

	// Cached site trigonometry (computed once).
	sinLat, cosLat float64
	sinLon, cosLon float64

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
}

// NewContext prepares the astrometry parameters for a specific observer time and site.
// The input time is defensively converted to UTC internally, since SOFA's Apco13
// expects UTC. Callers may pass any time scale; the conversion is a no-op for UTC.
func NewContext(t time.Time, site *Geodetic, atm atmosphere.Refraction) *Context {
	// SOFA Apco13 requires UTC two-part JD. Enforce UTC regardless of input scale
	// to prevent silent corruption of the ASTROM cache.
	t = t.UTC()
	jd1, jd2 := t.JDParts()
	eop := t.EOP()

	p := atm.Pressure
	if atm.Model != nil {
		p = 0.0 // Custom model overrides internal SOFA refraction
	}

	astrom, _ := gofaext.Apco13(
		jd1, jd2, eop.DUT1,
		site.Lon().Radians(), site.Lat().Radians(), site.Height(),
		eop.XP, eop.YP,
		p, atm.Temperature, atm.Humidity, atm.Wavelength,
	)

	// Precompute the C2t06a matrix and observer ICRS vector once, via its
	// decomposed factors (rc2i, rpom) rather than the monolithic call, so
	// AtTime can later recompute mat from a fresh Earth Rotation Angle
	// alone without rebuilding the slow precession-nutation/polar-motion
	// factors. Bit-identical to a direct C2t06a call.
	ut1, ut2 := jd1, jd2+eop.DUT1/86400.0
	tt1, tt2 := t.TT().JDParts()
	rc2i := gofaext.C2i06a(tt1, tt2)
	sp := gofaext.Sp00(tt1, tt2)
	rpom := gofaext.Pom00(eop.XP, eop.YP, sp)
	era0 := gofaext.Era00(ut1, ut2)
	mat := gofaext.C2tcio(rc2i, era0, rpom)

	sinLat, cosLat := math.Sincos(site.Lat().Radians())
	sinLon, cosLon := math.Sincos(site.Lon().Radians())

	tirs := tirsVec(sinLat, cosLat, sinLon, cosLon, site.Height())
	obsVec := icrsFromTIRS(mat, tirs)

	// Diurnal aberration, derived exactly as iauApio does: the horizontal
	// part of the observer's velocity in the celestial intermediate system,
	// over c. sp and era0 are already in hand, so this costs one Pvtob.
	pvob := gofaext.Pvtob(
		site.Lon().Radians(), site.Lat().Radians(), site.Height(),
		eop.XP, eop.YP, sp, era0,
	)
	diurab := math.Hypot(pvob[1][0], pvob[1][1]) / constants.SI2019.SpeedOfLight.Value

	return &Context{
		t:      t,
		site:   site,
		atm:    atm,
		astrom: astrom,
		eop:    eop,
		mat:    mat,
		obsVec: obsVec,
		rc2i:   rc2i,
		rpom:   rpom,
		sinLat: sinLat, cosLat: cosLat,
		sinLon: sinLon, cosLon: cosLon,
		diurab: diurab,
	}
}

// Clone returns an independent copy of the Context, safe for concurrent use.
// Each copy has its own ASTROM struct, avoiding data races from SOFA's
// internal refraction coefficient caching in iauAtioq.
func (ctx *Context) Clone() *Context {
	c := *ctx // shallow copy — all fields are value types or immutable pointers
	return &c
}

// kmPerAU is the number of kilometres in one Astronomical Unit.
//
// var, not const: constants.IAU.AstronomicalUnit is a struct, and Go does not
// permit selecting a struct field inside a constant expression.
var kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3

// tirsVec returns the observer's geocentric position in the TIRS frame
// (AU) from site trigonometry and height — pure site geometry, independent
// of time, shared by NewContext and AtTime.
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

// AtTime derives a new Context at instant t from this one, cheaply updating
// only Earth-rotation-dependent state (the ASTROM Earth Rotation Angle, the
// celestial-to-terrestrial matrix, and the observer vector) while reusing
// this Context's precession-nutation, Earth ephemeris, polar motion, cached
// EOP, and site/atmosphere state. Cost is O(1) (a handful of trig calls and
// matrix multiplies) versus NewContext's ~91 µs full SOFA rebuild.
//
// Accuracy: holding precession-nutation and aberration fixed costs ≲0.1″ per
// hour of |t − ctx.Time()| — dominated by nutation's ~13.66-day term
// (≈0.025″/h) and the annual-aberration direction's drift (≈0.015″/h);
// precession (≈0.006″/h) and reusing this Context's DUT1/polar-motion
// (<0.001″/h combined) are smaller still. At the horizon's steepest crossing
// rate, 0.1″ of positional error is under 0.01 s of rise/set-time bias.
// Callers sweeping longer spans should rebuild a fresh NewContext
// periodically rather than calling AtTime indefinitely far from ctx.Time().
func (ctx *Context) AtTime(t time.Time) *Context {
	t = t.UTC()
	jd1, jd2 := t.JDParts()
	ut1, ut2 := jd1, jd2+ctx.eop.DUT1/86400.0
	era := gofaext.Era00(ut1, ut2)

	c := ctx.Clone()
	c.t = t
	gofaext.Aper(era, &c.astrom)
	c.mat = gofaext.C2tcio(c.rc2i, era, c.rpom)
	c.obsVec = icrsFromTIRS(c.mat, tirsVec(c.sinLat, c.cosLat, c.sinLon, c.cosLon, c.site.Height()))

	return c
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

// AstrometricToApparent computes the Celestial Intermediate Reference System (CIRS) apparent
// position of an object from its Astrometric (catalog ICRS) coordinates.
func (ctx *Context) AstrometricToApparent(c Astrometric) Apparent {
	ri, di := gofaext.Atciq(
		c.RA().Radians(), c.Dec().Radians(),
		c.PmRA().Radians(), c.PmDec().Radians(), c.Parallax().Radians(), c.RV(),
		&ctx.astrom,
	)

	return NewApparent(angle.Rad(ri).Wrap360(), angle.Rad(di))
}

// ApparentToObserved converts geocentric CIRS Apparent coordinates to local Observed AltAz
// taking into account Earth rotation, polar motion, and atmospheric refraction.
func (ctx *Context) ApparentToObserved(c Apparent) AltAz {
	az, zd, _, _, _ := gofaext.Atioq(
		c.RA().Radians(), c.Dec().Radians(),
		&ctx.astrom,
	)

	alt := angle.Rad(math.Pi/2 - zd)
	if ctx.atm.Model != nil {
		alt += ctx.atm.Model.RefractFromTrue(alt, ctx.atm)
	}

	return NewAltAz(alt, angle.Rad(az).Wrap360())
}

// AstrometricToObserved collapses the entire apparent pipeline from an Astrometric catalog
// point explicitly to a refracted local AltAz position.
func (ctx *Context) AstrometricToObserved(c Astrometric) AltAz {
	az, zd, _, _, _ := gofaext.Atcoq(
		c.RA().Radians(), c.Dec().Radians(),
		c.PmRA().Radians(), c.PmDec().Radians(), c.Parallax().Radians(), c.RV(),
		&ctx.astrom,
	)

	alt := angle.Rad(math.Pi/2 - zd)
	if ctx.atm.Model != nil {
		alt += ctx.atm.Model.RefractFromTrue(alt, ctx.atm)
	}

	return NewAltAz(alt, angle.Rad(az).Wrap360())
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
// Atmospheric refraction is applied using the Context's refraction model.
// When no explicit model is set (Model == nil) but atmospheric pressure is
// nonzero, refractLikeAtioq applies SOFA's own two-term series from the Refa
// and Refb constants Apco13 already cached — the same resolution
// [atmosphere.Refraction.EffectiveModel] makes, reached by a faster route.
//
// atmosphere.RefractionSOFA is that model in portable form, recomputing the
// constants per call rather than reusing the cache. The two are pinned against
// each other by TestNilModelMatchesExplicitSOFAModel, which measures agreement
// to 0.012 arcsec above 3 degrees and 0.7 milliarcsecond above 5 degrees.
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

	alt := angle.Rad(altitude)

	switch {
	case ctx.atm.Model != nil:
		alt += ctx.atm.Model.RefractFromTrue(alt, ctx.atm)
	case ctx.atm.Pressure > 0:
		alt = refractLikeAtioq(E, N, U, 1, ctx.astrom.Refa, ctx.astrom.Refb)
	}

	return NewAltAz(alt, angle.Rad(azimuth))
}

// Refraction clamps, copied from SOFA's iauAtioq rather than chosen here.
//
// selMin bounds sin(altitude) at 0.05 — about 2.87° — so the series is never
// evaluated where it diverges. celMin bounds cos(altitude) away from zero at
// the zenith, where the horizontal component vanishes.
const (
	selMin = 0.05
	celMin = 1e-6
)

// refractLikeAtioq applies refraction to a topocentric ENU direction exactly as
// SOFA's Atioq does, and returns the refracted altitude.
//
// # Why this reproduces Atioq instead of approximating it
//
// This branch exists so the vector pipeline agrees with the stellar one, which
// goes through Atioq. It previously wrote out what looked like the same model —
//
//	dR := Refa*tan(z) + Refb*tan³(z)
//
// — guarded only by z < 91° and dR > 0, and it was wrong three ways at once.
//
// It had no clamp. tan(z) diverges at z = 90°, which is the horizon, not the
// 91° the guard allowed: at alt −0.076° the cubic term flips sign to about
// +61 rad and the dR > 0 test waves it straight through. Measured, that
// returned an altitude of +7028°. The guard meant to reject bad values was
// admitting only the catastrophic ones, because it rejected the honest
// negatives.
//
// Between about 0° and 2° the cubic term cancels the linear one instead, dR
// went non-positive, and refraction was dropped entirely — 0.000° where the
// stellar path applied 0.16°.
//
// And the model itself was the uncorrected series. Atioq applies a
// Newton-Raphson correction, dividing by 1 + (A + 3B·tan²z)/sin²(alt), which
// the raw form omits even where it converges.
//
// Reproducing the routine rather than re-deriving it is what makes the two
// pipelines agree by construction. TestRefractionMatchesTheStellarPipeline pins that.
//
// # A consequence worth stating
//
// Because sin(altitude) is clamped, a target below the horizon is refracted as
// though it were at 2.87°, so this reports about 0.17° of refraction for
// something that has already set. That is arguable physics and it is precisely
// what the stellar path does; matching it is the point. A caller wanting
// geometric altitude should use a Context with no atmosphere.
func refractLikeAtioq(e, n, u, norm, refa, refb float64) angle.Angle {
	if norm == 0 {
		return 0
	}

	// Atioq works on a unit vector, so the clamps are in units of sine and
	// cosine of altitude.
	ue, un, uu := e/norm, n/norm, u/norm

	r := math.Hypot(ue, un)
	if r <= celMin {
		r = celMin
	}

	z := uu
	if z <= selMin {
		z = selMin
	}

	// A·tan(z) + B·tan³(z) with Atioq's Newton-Raphson correction.
	tz := r / z
	w := refb * tz * tz
	del := (refa + w) * tz / (1.0 + (refa+3.0*w)/(z*z))

	// Rotate the direction by del, as Atioq does. The clamped r and z drive
	// the rotation; the unclamped components are what it rotates.
	cosdel := 1.0 - del*del/2.0
	f := cosdel - del*z/r
	eo, no := ue*f, un*f
	uo := cosdel*uu + del*r

	return angle.Rad(math.Pi/2 - math.Atan2(math.Hypot(eo, no), uo))
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
func (ctx *Context) ICRSToHourAngle(c ICRS) (angle.Angle, error) {
	_, _, ha, _, _ := gofaext.Atcoq(
		c.RA().Radians(), c.Dec().Radians(),
		c.PmRA().Radians(), c.PmDec().Radians(), c.Parallax().Radians(), c.RV(),
		&ctx.astrom,
	)

	return angle.Rad(ha).Wrap180(), nil
}

// AltAzToICRS converts local observed AltAz back into geometric ICRS.
// NOTE: Atoc13 is used because the reverse pipeline requires the observation type ('A').
// The EOP data cached at Context construction is reused to avoid a redundant IERS lookup.
func (ctx *Context) AltAzToICRS(c AltAz) (ICRS, error) {
	jd1, jd2 := ctx.t.JDParts()

	p := ctx.atm.Pressure
	geomAlt := c.Alt()

	if ctx.atm.Model != nil {
		p = 0.0
		R := ctx.atm.Model.RefractFromApparent(c.Alt(), ctx.atm)
		geomAlt = angle.Rad(c.Alt().Radians() - R.Radians())
	}

	ra, dec := gofaext.Atoc13(
		"A",
		c.Az().Radians(), math.Pi/2-geomAlt.Radians(),
		jd1, jd2, ctx.eop.DUT1,
		ctx.site.Lon().Radians(), ctx.site.Lat().Radians(), ctx.site.Height(),
		ctx.eop.XP, ctx.eop.YP,
		p, ctx.atm.Temperature, ctx.atm.Humidity, ctx.atm.Wavelength,
	)

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
// The result is not renormalised, again as Atioq leaves it — the magnitude
// differs from one by about 1.5e-6, and the callers here take an atan2.
func (ctx *Context) aberrateDiurnal(e, n, u, norm float64) (float64, float64, float64) {
	if norm == 0 {
		return e, n, u
	}

	e, n, u = e/norm, n/norm, u/norm

	f := 1.0 - ctx.diurab*e

	return f * (e + ctx.diurab), f * n, f * u
}
