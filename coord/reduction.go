package coord

import (
	"log"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/iers"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Reduction represents the calculated outcomes of an apparent-place reduction sequence.
// It exposes the intermediate geometries from the transformation pipeline.
type Reduction struct {
	// Geocentric is the input inertial state vector (e.g., True or Apparent state).
	Geocentric vector.Vec3
	// Topocentric is the Cartesian state vector relative to the observer (Geocentric - Observer)
	// measured in the inertial ICRS frame.
	Topocentric vector.Vec3
	// Geometric is the un-refracted local horizon coordinate (Altitude / Azimuth).
	Geometric AltAz
	// Observed is the refracted local horizon coordinate for the primary wavelength.
	Observed AltAz
	// Dispersion maps specific wavelengths to their refracted local horizon coordinates.
	Dispersion map[float64]AltAz
}

// Reducer defines the explicit apparent-place reduction pipeline, converting
// geocentric positions to topocentric observed geometries.
type Reducer struct {
	site  *Geodetic
	time  time.Time
	atmos atmosphere.Atmosphere
}

// NewReducer creates a new apparent-place reduction pipeline.
func NewReducer(site *Geodetic, t time.Time, atmos atmosphere.Atmosphere) *Reducer {
	return &Reducer{
		site:  site,
		time:  t,
		atmos: atmos,
	}
}

// Reduce translates a geocentric inertial vector into topocentric coordinates,
// explicitly modeling Earth orientation, topological observer displacement,
// and applying the environmental refraction model.
func (r *Reducer) Reduce(v vector.Vec3) *Reduction {
	res := &Reduction{
		Geocentric: v,
	}

	// Ensure UTC for EOP lookup and UT1 derivation, regardless of input scale.
	utcTime := r.time.UTC()
	jd1, jd2 := utcTime.JDParts()

	// Fetch EOP once for both UT1 derivation and polar motion,
	// avoiding the redundant IERS lookup that UT1()/TT() would each make.
	mjd := (jd1 - 2400000.5) + jd2
	eop, err := iers.GetModel().EOP(mjd)
	if err != nil {
		warnEOPOnce.Do(func() {
			log.Printf("astrogo/coord: IERS EOP data unavailable (MJD %.1f): using zero DUT1/polar motion. Topocentric accuracy degraded to ~1 arcsec.", mjd)
		})
	}

	// Derive UT1 from UTC + DUT1, and TT via the scale-aware conversion.
	ut1, ut2 := jd1, jd2+eop.DUT1/86400.0
	tt1, tt2 := r.time.TT().JDParts()

	// 1. Get SOFA ICRS-to-TIRS matrix
	mat := gofaext.C2t06a(tt1, tt2, ut1, ut2, eop.XP, eop.YP)

	// 2. WGS84 simplistic radius displacement in strictly Terrestrial Frame (TIRS)
	const au = 149597870.7
	const rEq = 6378.137
	const f = 1.0 / 298.257223563

	sinLat := math.Sin(r.site.Lat().Radians())
	cosLat := math.Cos(r.site.Lat().Radians())

	c_earth := 1.0 / math.Sqrt(cosLat*cosLat+(1.0-f)*(1.0-f)*sinLat*sinLat)
	s_earth := (1.0 - f) * (1.0 - f) * c_earth

	xTIRS := (rEq*c_earth + r.site.Height()/1000.0) * cosLat * math.Cos(r.site.Lon().Radians()) / au
	yTIRS := (rEq*c_earth + r.site.Height()/1000.0) * cosLat * math.Sin(r.site.Lon().Radians()) / au
	zTIRS := (rEq*s_earth + r.site.Height()/1000.0) * sinLat / au

	// 3. Multiply TIRS Vector by TRANSPOSE of ICRS->TIRS Matrix to produce Observer ICRS Vector
	obsX := mat[0][0]*xTIRS + mat[1][0]*yTIRS + mat[2][0]*zTIRS
	obsY := mat[0][1]*xTIRS + mat[1][1]*yTIRS + mat[2][1]*zTIRS
	obsZ := mat[0][2]*xTIRS + mat[1][2]*yTIRS + mat[2][2]*zTIRS

	obsVec := vector.Vec3{X: obsX, Y: obsY, Z: obsZ}

	// 4. Absolute Cartesian Shift (Topocentric vector in ICRS frame)
	topoVec := v.Sub(obsVec)
	res.Topocentric = topoVec

	// 5. Transform Topocentric ICRS vector directly back into the body-fixed ITRS (Terrestrial) frame.
	tx := mat[0][0]*topoVec.X + mat[0][1]*topoVec.Y + mat[0][2]*topoVec.Z
	ty := mat[1][0]*topoVec.X + mat[1][1]*topoVec.Y + mat[1][2]*topoVec.Z
	tz := mat[2][0]*topoVec.X + mat[2][1]*topoVec.Y + mat[2][2]*topoVec.Z

	// 6. Convert ITRS to Local Horizon ENU (East, North, Up)
	lonRad := r.site.Lon().Radians()
	sinLon, cosLon := math.Sincos(lonRad)

	E := -sinLon*tx + cosLon*ty
	N := -sinLat*cosLon*tx - sinLat*sinLon*ty + cosLat*tz
	U := cosLat*cosLon*tx + cosLat*sinLon*ty + sinLat*tz

	// 7. Calculate Geometric Altitude and Azimuth
	azimuth := math.Atan2(E, N)
	if azimuth < 0 {
		azimuth += 2 * math.Pi
	}
	altitude := math.Asin(U / topoVec.Norm())

	geomAltAz := NewAltAz(angle.Rad(altitude), angle.Rad(azimuth))
	res.Geometric = geomAltAz

	// 8. Apply Atmospheric Refraction for primary wavelength
	obsAltAz := NewAltAz(geomAltAz.Alt(), geomAltAz.Az())
	switch {
	case r.atmos.Model != nil:
		shift := r.atmos.Model.RefractFromTrue(geomAltAz.Alt(), r.atmos)
		obsAltAz.SetAlt(geomAltAz.Alt().Add(shift))
	case r.atmos.Pressure > 0:
		// Use SOFA's refraction model (Refco + tan(z) series) for consistency
		// with the stellar path (Atioq). Refco computes the same coefficients
		// that Apco13 stores in ASTROM.Refa/Refb.
		refa, refb := gofaext.Refco(r.atmos.Pressure, r.atmos.Temperature,
			r.atmos.Humidity, r.atmos.Wavelength)
		z := math.Pi/2 - geomAltAz.Alt().Radians()
		const zMax = 91.0 * math.Pi / 180.0 // alt ≈ −1°
		if z > 0 && z < zMax {
			tz := math.Tan(z)
			dR := refa*tz + refb*tz*tz*tz
			if dR > 0 {
				obsAltAz.SetAlt(angle.Rad(geomAltAz.Alt().Radians() + dR))
			}
		}
	}
	res.Observed = obsAltAz

	return res
}

// Disperse computes wavelength-dependent refraction for a set of target wavelengths,
// returning the reduction evaluated differentially for each wavelength.
func (r *Reducer) Disperse(v vector.Vec3, wavelengths []float64) *Reduction {
	res := r.Reduce(v)
	res.Dispersion = make(map[float64]AltAz)

	if r.atmos.Model == nil {
		// No refraction model; dispersion is identical across all wavelengths
		for _, wl := range wavelengths {
			res.Dispersion[wl] = res.Observed
		}
		return res
	}

	for _, wl := range wavelengths {
		// Clone and substitute the specific environment wavelength dynamically
		wlAtmos := r.atmos
		wlAtmos.Wavelength = wl

		shift := wlAtmos.Model.RefractFromTrue(res.Geometric.Alt(), wlAtmos)
		dispersedAltAz := NewAltAz(res.Geometric.Alt().Add(shift), res.Geometric.Az())
		res.Dispersion[wl] = dispersedAltAz
	}

	return res
}
