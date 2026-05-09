package coord

import (
	"math"
	"sync"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Reduction represents the calculated outcomes of an apparent-place reduction sequence.
// It exposes the intermediate geometries from the transformation pipeline.
type Reduction struct {
	Dispersion  map[float64]AltAz
	Geocentric  vector.Vec3
	Topocentric vector.Vec3
	Geometric   AltAz
	Observed    AltAz
}

// Reducer defines the explicit apparent-place reduction pipeline, converting
// geocentric positions to topocentric observed geometries.
//
// The first call to Reduce lazily builds and caches a [Context], so subsequent
// calls for the same (site, time, atmosphere) avoid rebuilding the SOFA C2t06a
// matrix and IERS EOP lookup (~91 µs each). This makes Reducer suitable for
// batch reduction of many targets at the same epoch.
type Reducer struct {
	atmos atmosphere.Atmosphere
	time  time.Time
	site  *Geodetic
	ctx   *Context
	once  sync.Once
}

// NewReducer creates a new apparent-place reduction pipeline.
func NewReducer(site *Geodetic, t time.Time, atmos atmosphere.Atmosphere) *Reducer {
	return &Reducer{
		site:  site,
		time:  t,
		atmos: atmos,
	}
}

// context returns the lazily-initialized Context, building it on first access.
func (r *Reducer) context() *Context {
	r.once.Do(func() {
		r.ctx = NewContext(r.time, r.site, r.atmos)
	})
	return r.ctx
}

// Reduce translates a geocentric inertial vector into topocentric coordinates,
// explicitly modeling Earth orientation, topological observer displacement,
// and applying the environmental refraction model.
//
// The expensive SOFA matrix computation is performed lazily on the first call
// and cached for all subsequent calls via the underlying Context.
func (r *Reducer) Reduce(v vector.Vec3) *Reduction {
	ctx := r.context()

	// Topocentric vector in ICRS frame (for the Reduction struct).
	topoVec := v.Sub(ctx.obsVec)

	// Geometric (unrefracted) AltAz via the cached ICRS→ITRS→ENU pipeline.
	// We compute geometric separately by temporarily ignoring refraction.
	tx := ctx.mat[0][0]*topoVec.X + ctx.mat[0][1]*topoVec.Y + ctx.mat[0][2]*topoVec.Z
	ty := ctx.mat[1][0]*topoVec.X + ctx.mat[1][1]*topoVec.Y + ctx.mat[1][2]*topoVec.Z
	tz := ctx.mat[2][0]*topoVec.X + ctx.mat[2][1]*topoVec.Y + ctx.mat[2][2]*topoVec.Z

	E := -ctx.sinLon*tx + ctx.cosLon*ty
	N := -ctx.sinLat*ctx.cosLon*tx - ctx.sinLat*ctx.sinLon*ty + ctx.cosLat*tz
	U := ctx.cosLat*ctx.cosLon*tx + ctx.cosLat*ctx.sinLon*ty + ctx.sinLat*tz

	azimuth := math.Atan2(E, N)
	if azimuth < 0 {
		azimuth += 2 * math.Pi
	}
	altitude := math.Asin(U / topoVec.Norm())
	geomAltAz := NewAltAz(angle.Rad(altitude), angle.Rad(azimuth))

	// Observed (refracted) AltAz via Context's full pipeline.
	obsAltAz := ctx.GeocentricToObserved(v)

	return &Reduction{
		Geocentric:  v,
		Topocentric: topoVec,
		Geometric:   geomAltAz,
		Observed:    obsAltAz,
	}
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
