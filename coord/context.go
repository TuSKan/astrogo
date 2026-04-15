package coord

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/iers"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Context encapsulates the observation environment (time and location) and precomputes
// the computationally expensive SOFA intermediate astrometry parameters (ASTROM).
// Utilizing a single Context fundamentally accelerates large-scale catalog coordinate operations
// resolving the matrix recomputation bottleneck.
type Context struct {
	t      time.Time
	site   *Geodetic
	atm    Atmosphere
	astrom gofaext.ASTROM
	eo     float64
}

// NewContext prepares the astrometry parameters for a specific observer time and site.
func NewContext(t time.Time, site *Geodetic, atm Atmosphere) *Context {
	jd1, jd2 := t.JDParts()
	mjd := (jd1 - 2400000.5) + jd2
	eop, _ := iers.GetModel().EOP(mjd)

	p := atm.Pressure
	if atm.Model != nil {
		p = 0.0 // Custom model overrides internal SOFA refraction
	}

	astrom, eo := gofaext.Apco13(
		jd1, jd2, eop.DUT1,
		site.Lon().Radians(), site.Lat().Radians(), site.Height(),
		eop.XP, eop.YP,
		p, atm.Temperature, atm.Humidity, atm.Wavelength,
	)

	return &Context{
		t:      t,
		site:   site,
		atm:    atm,
		astrom: astrom,
		eo:     eo,
	}
}

// Time returns the encapsulated observation time.
func (ctx *Context) Time() time.Time { return ctx.t }

// Site returns the encapsulated observation geodetic location.
func (ctx *Context) Site() *Geodetic { return ctx.site }

// Atmosphere returns the encapsulated atmosphere configuration.
func (ctx *Context) Atmosphere() Atmosphere { return ctx.atm }

// AstrometricToApparent computes the Celestial Intermediate Reference System (CIRS) apparent
// position of an object from its Astrometric (catalog ICRS) coordinates.
func (ctx *Context) AstrometricToApparent(c *Astrometric) *Apparent {
	ri, di := gofaext.Atciq(
		c.RA().Radians(), c.Dec().Radians(),
		c.PmRA().Radians(), c.PmDec().Radians(), c.Parallax().Radians(), c.RV(),
		&ctx.astrom,
	)
	return NewApparent(angle.Rad(ri).Wrap360(), angle.Rad(di))
}

// ApparentToObserved converts geocentric CIRS Apparent coordinates to local Observed AltAz
// taking into account Earth rotation, polar motion, and atmospheric refraction.
func (ctx *Context) ApparentToObserved(c *Apparent) *AltAz {
	az, zd, _, _, _ := gofaext.Atioq(
		c.RA().Radians(), c.Dec().Radians(),
		&ctx.astrom,
	)

	alt := angle.Rad(math.Pi/2 - zd)
	if ctx.atm.Model != nil {
		alt += ctx.atm.Model.RefractFromTrue(alt, ctx.atm, ctx.site)
	}

	return NewAltAz(alt, angle.Rad(az).Wrap360())
}

// AstrometricToObserved collapses the entire apparent pipeline from an Astrometric catalog
// point explicitly to a refracted local AltAz position.
func (ctx *Context) AstrometricToObserved(c *Astrometric) *AltAz {
	az, zd, _, _, _ := gofaext.Atcoq(
		c.RA().Radians(), c.Dec().Radians(),
		c.PmRA().Radians(), c.PmDec().Radians(), c.Parallax().Radians(), c.RV(),
		&ctx.astrom,
	)

	alt := angle.Rad(math.Pi/2 - zd)
	if ctx.atm.Model != nil {
		alt += ctx.atm.Model.RefractFromTrue(alt, ctx.atm, ctx.site)
	}

	return NewAltAz(alt, angle.Rad(az).Wrap360())
}

// GeocentricToObserved traces planets algebraically dynamically solving observer topological shifts natively.
func (ctx *Context) GeocentricToObserved(v vector.Vec3) *AltAz {
	pipeline := NewReducer(ctx.site, ctx.t, ctx.atm)
	res := pipeline.Reduce(v)
	return res.Observed
}

// ICRSToAltAz converts purely geometric ICRS coordinates to local observed AltAz
// utilizing the precomputed epoch pipeline matrices.
func (ctx *Context) ICRSToAltAz(c *ICRS) (*AltAz, error) {
	astro := NewAstrometric(c.RA(), c.Dec())
	altaz := ctx.AstrometricToObserved(astro)
	altaz.SetDist(c.Dist())
	return altaz, nil
}

// ICRSToHourAngle converts purely geometric ICRS coordinates to local observed Hour Angle.
func (ctx *Context) ICRSToHourAngle(c *ICRS) (angle.Angle, error) {
	_, _, ha, _, _ := gofaext.Atcoq(
		c.RA().Radians(), c.Dec().Radians(),
		0, 0, 0, 0,
		&ctx.astrom,
	)
	return angle.Rad(ha).Wrap180(), nil
}

// AltAzToICRS converts local observed AltAz back into geometric ICRS.
// NOTE: AltAzToICRS does not use Atioq because the reverse pipeline Atoc13 is dependent on the type ('A', 'H', 'R').
// To avoid rewriting the entire ASTROM inverse function logic right now, we retain the original logic for reverse mappings.
func (ctx *Context) AltAzToICRS(c *AltAz) (*ICRS, error) {
	jd1, jd2 := ctx.t.JDParts()
	mjd := (jd1 - 2400000.5) + jd2
	eop, _ := iers.GetModel().EOP(mjd)

	p := ctx.atm.Pressure
	geomAlt := c.Alt()

	if ctx.atm.Model != nil {
		p = 0.0
		R := ctx.atm.Model.RefractFromApparent(c.Alt(), ctx.atm, ctx.site)
		geomAlt = angle.Rad(c.Alt().Radians() - R.Radians())
	}

	ra, dec := gofaext.Atoc13(
		"A",
		c.Az().Radians(), math.Pi/2-geomAlt.Radians(),
		jd1, jd2, eop.DUT1,
		ctx.site.Lon().Radians(), ctx.site.Lat().Radians(), ctx.site.Height(),
		eop.XP, eop.YP,
		p, ctx.atm.Temperature, ctx.atm.Humidity, ctx.atm.Wavelength,
	)

	return NewICRS(angle.Rad(ra).Wrap360(), angle.Rad(dec)), nil
}
