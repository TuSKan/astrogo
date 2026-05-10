package coord

import (
	"log"
	"math"
	"sync"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/iers"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// warnEOPOnce guards the one-time warning emitted when IERS EOP data is unavailable.
var warnEOPOnce sync.Once

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
	atm    atmosphere.Atmosphere
	astrom gofaext.ASTROM
	eop    iers.EOP // cached for AltAzToICRS reuse

	// Precomputed geocentric reduction fields (for GeocentricToObserved).
	// These avoid rebuilding the C2t06a matrix + observer vector per call.
	mat    [3][3]float64 // ICRS → TIRS rotation matrix
	obsVec vector.Vec3   // observer position in ICRS frame (AU)

	// Cached site trigonometry (computed once).
	sinLat, cosLat float64
	sinLon, cosLon float64
}

// NewContext prepares the astrometry parameters for a specific observer time and site.
// The input time is defensively converted to UTC internally, since SOFA's Apco13
// expects UTC. Callers may pass any time scale; the conversion is a no-op for UTC.
func NewContext(t time.Time, site *Geodetic, atm atmosphere.Atmosphere) *Context {
	// SOFA Apco13 requires UTC two-part JD. Enforce UTC regardless of input scale
	// to prevent silent corruption of the ASTROM cache.
	t = t.UTC()
	jd1, jd2 := t.JDParts()
	mjd := (jd1 - 2400000.5) + jd2

	eop, err := iers.GetModel().EOP(mjd)
	if err != nil {
		warnEOPOnce.Do(func() {
			log.Printf("astrogo/coord: IERS EOP data unavailable (MJD %.1f): using zero DUT1/polar motion. Topocentric accuracy degraded to ~1 arcsec.", mjd)
		})
	}

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

	// Precompute C2t06a matrix and observer ICRS vector once.
	ut1, ut2 := jd1, jd2+eop.DUT1/86400.0
	tt1, tt2 := t.TT().JDParts()
	mat := gofaext.C2t06a(tt1, tt2, ut1, ut2, eop.XP, eop.YP)

	sinLat, cosLat := math.Sincos(site.Lat().Radians())
	sinLon, cosLon := math.Sincos(site.Lon().Radians())

	const (
		au  = 149597870.7
		rEq = 6378.137
		f   = 1.0 / 298.257223563
	)

	cEarth := 1.0 / math.Sqrt(cosLat*cosLat+(1.0-f)*(1.0-f)*sinLat*sinLat)
	sEarth := (1.0 - f) * (1.0 - f) * cEarth
	heightKm := site.Height() / 1000.0

	xTIRS := (rEq*cEarth + heightKm) * cosLat * cosLon / au
	yTIRS := (rEq*cEarth + heightKm) * cosLat * sinLon / au
	zTIRS := (rEq*sEarth + heightKm) * sinLat / au

	// Observer ICRS = transpose(mat) * TIRS
	obsVec := vector.Vec3{
		X: mat[0][0]*xTIRS + mat[1][0]*yTIRS + mat[2][0]*zTIRS,
		Y: mat[0][1]*xTIRS + mat[1][1]*yTIRS + mat[2][1]*zTIRS,
		Z: mat[0][2]*xTIRS + mat[1][2]*yTIRS + mat[2][2]*zTIRS,
	}

	return &Context{
		t:      t,
		site:   site,
		atm:    atm,
		astrom: astrom,
		eop:    eop,
		mat:    mat,
		obsVec: obsVec,
		sinLat: sinLat, cosLat: cosLat,
		sinLon: sinLon, cosLon: cosLon,
	}
}

// Clone returns an independent copy of the Context, safe for concurrent use.
// Each copy has its own ASTROM struct, avoiding data races from SOFA's
// internal refraction coefficient caching in iauAtioq.
func (ctx *Context) Clone() *Context {
	c := *ctx // shallow copy — all fields are value types or immutable pointers
	return &c
}

// Time returns the encapsulated observation time.
func (ctx *Context) Time() time.Time { return ctx.t }

// Site returns the encapsulated observation geodetic location.
func (ctx *Context) Site() *Geodetic { return ctx.site }

// Atmosphere returns the encapsulated atmosphere configuration.
func (ctx *Context) Atmosphere() atmosphere.Atmosphere { return ctx.atm }

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
// Atmospheric refraction is applied using the Context's refraction model.
// When no explicit model is set (Model == nil) but atmospheric pressure is
// nonzero, the Saemundsson (1986) rigorous formula is used as a fallback,
// matching the behaviour of SOFA's internal refraction in the stellar path.
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

	azimuth := math.Atan2(E, N)
	if azimuth < 0 {
		azimuth += 2 * math.Pi
	}

	altitude := math.Asin(U / topoVec.Norm())

	alt := angle.Rad(altitude)

	switch {
	case ctx.atm.Model != nil:
		alt += ctx.atm.Model.RefractFromTrue(alt, ctx.atm)
	case ctx.atm.Pressure > 0:
		// Use SOFA's own refraction coefficients (Refa, Refb) computed by
		// Apco13 and stored in the ASTROM context. This is exactly the model
		// that Atioq uses for the stellar path, ensuring consistency between
		// the direct geocentric pipeline and the SOFA astrometric pipeline.
		// Formula: ΔR = Refa·tan(z) + Refb·tan³(z)  (z = π/2 − alt)
		//
		// We apply refraction down to alt ≈ −1° (z ≤ 91°), since atmospheric
		// refraction is physically nonzero slightly below the geometric
		// horizon. Below −1° the tan(z) series diverges and refraction is
		// negligible for practical purposes.
		z := math.Pi/2 - altitude

		const zMax = 91.0 * math.Pi / 180.0 // alt ≈ −1°
		if z > 0 && z < zMax {
			tz := math.Tan(z)

			dR := ctx.astrom.Refa*tz + ctx.astrom.Refb*tz*tz*tz
			if dR > 0 {
				alt = angle.Rad(altitude + dR)
			}
		}
	}

	return NewAltAz(alt, angle.Rad(azimuth))
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
