package skybrightness

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// skyglowOptics holds the per-wavelength atmospheric state Eq. 27 reads.
//
// Built once per direction rather than per integration step, because none of
// it depends on height: the profiles are analytic in h and the column values
// are what parameterise them. The height loop then does arithmetic only.
type skyglowOptics struct {
	// Column optical depths at each wavelength, molecular and aerosol.
	molecular, aerosol []unit.OpticalDepth

	// Scale heights of the two profiles, in metres.
	molecularScaleM, aerosolScaleM float64

	// aerosolAlbedo is the aerosol single-scattering albedo, which converts
	// its extinction into scattering. Molecular scattering is conservative,
	// so its albedo is one and does not appear.
	aerosolAlbedo float64

	asymmetry float64
}

// newSkyglowOptics reads the scene's atmosphere onto the grid.
func newSkyglowOptics(grid unit.SpectralGrid, scene *Scene) (*skyglowOptics, error) {
	n := grid.Len()

	pressure, _ := scene.Atmosphere.Surface()
	aer := scene.Atmosphere.Aerosol()

	aerosolScale := float64(aer.BoundaryLayerHeight)
	if aerosolScale <= 0 {
		return nil, fmt.Errorf("%w: the atmosphere has no aerosol boundary-layer height",
			ErrScaleHeight)
	}

	_, temperature := scene.Atmosphere.Surface()

	molecularScale, err := atmosphere.MolecularScaleHeight(temperature)
	if err != nil {
		return nil, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	o := &skyglowOptics{
		molecular:       make([]unit.OpticalDepth, n),
		aerosol:         make([]unit.OpticalDepth, n),
		molecularScaleM: molecularScale,
		aerosolScaleM:   aerosolScale,
		aerosolAlbedo:   float64(aer.SingleScatteringAlbedo),
		asymmetry:       float64(aer.Asymmetry),
	}

	for i := range n {
		lambda := grid.At(i)

		rayleigh, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return nil, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
		}

		o.molecular[i] = rayleigh
		o.aerosol[i] = unit.OpticalDepth(aer.TauAt(lambda))
	}

	return o, nil
}

// transmission returns the two-leg transmission of Kocifaj (2007) Eq. 11 at
// one wavelength: up from the source through the slant path its zenith angle
// implies, and down to the observer along theirs.
//
//	T(h,z,phi) = t(h,z) * t(h,z0_h)
//
// with each leg Eq. 9's exp of an airmass-weighted vertical depth. The
// airmass is Gushchin's, which is what this model lineage adopts and what
// keeps the horizon finite where a plane-parallel secant would not.
func (o *skyglowOptics) transmission(i int, heightM, cosObserver, cosSource float64) (float64, error) {
	molecular, err := atmosphere.ExponentialDepth(
		unit.AltitudeM(heightM), o.molecular[i], o.molecularScaleM)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	aerosol, err := atmosphere.ExponentialDepth(
		unit.AltitudeM(heightM), o.aerosol[i], o.aerosolScaleM)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	down, err := legTransmission(molecular+aerosol, cosObserver)
	if err != nil {
		return 0, err
	}

	up, err := legTransmission(molecular+aerosol, cosSource)
	if err != nil {
		return 0, err
	}

	return down * up, nil
}

// legTransmission applies one slant leg at the given cosine of zenith angle.
func legTransmission(vertical unit.OpticalDepth, cosZenith float64) (float64, error) {
	if cosZenith <= 0 {
		return 0, nil // at or below the horizon nothing gets through
	}

	airmass, err := atmosphere.GushchinAirmass(angle.Rad(math.Asin(math.Min(1, cosZenith))))
	if err != nil {
		return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	return float64(atmosphere.Transmission(vertical * unit.OpticalDepth(airmass))), nil
}

// scattering returns Psi, the angular volume scattering coefficient at a
// height and scattering angle, at one wavelength — Kocifaj (2007) Eq. 18 with
// the exponential profiles of its Eq. 36.
func (o *skyglowOptics) scattering(i int, heightM, theta float64) (float64, error) {
	molecularExt, err := atmosphere.ExponentialExtinction(
		unit.AltitudeM(heightM), o.molecular[i], o.molecularScaleM)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	aerosolExt, err := atmosphere.ExponentialExtinction(
		unit.AltitudeM(heightM), o.aerosol[i], o.aerosolScaleM)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	// Scattering rather than extinction: the aerosol absorbs a share of what
	// it removes, and only what it scatters can reach the observer. Molecular
	// scattering is conservative.
	psi, err := atmosphere.VolumeScatteringFunction(theta,
		molecularExt, aerosolExt*o.aerosolAlbedo,
		o.asymmetry, atmosphere.RayleighDepolarisation)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	return psi, nil
}

// addCloudTerm accumulates the first term of Kocifaj (2007) Eq. 27: light
// that reached the cloud base, reflected, and came back down.
//
//	2*pi * rho * cos^4(z0_H) * S(z0_H) * T(H,z,phi) / H^2
//
// One evaluation rather than an integral, because a reflection happens at one
// altitude. This is the term that makes an overcast sky over a city brighter
// than a clear one — the cloud returns light that would otherwise have left.
func (c *CloudySkyglow) addCloudTerm(
	dst SpectralRadiance,
	buf []float64,
	grid unit.SpectralGrid,
	geom skyglowGeometry,
	optics *skyglowOptics,
	emitter GroundEmitter,
	toObserver angle.Angle,
	baseM, reflectance float64,
) error {
	cosZ0, _ := geom.atHeight(baseM)
	if cosZ0 <= 0 {
		return nil
	}

	elevation := angle.Rad(math.Asin(math.Min(1, cosZ0)))

	if err := emitter.SourceRadiance(buf, grid, toObserver, elevation); err != nil {
		return fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	cos4 := cosZ0 * cosZ0 * cosZ0 * cosZ0
	scale := 2 * math.Pi * reflectance * cos4 / (baseM * baseM)

	for i := range dst {
		t, err := optics.transmission(i, baseM, geom.cosZenith, cosZ0)
		if err != nil {
			return err
		}

		dst[i] += buf[i] * scale * t
	}

	return nil
}

// addAirTerm accumulates the second term of Kocifaj (2007) Eq. 27: light
// scattered into the line of sight by the air below the cloud.
//
//	(1/cos z) * INT[0,H] S(z0_h) cos^2(z0_h) [T(h,z,phi)/h^2] Psi(h,Theta_h) dh
//
// # Why the integrand is finite at the ground
//
// It carries 1/h^2, which diverges, and cos^2(z0_h), which does not: at low
// altitude the source subtends a nearly horizontal ray, so cos(z0_h) falls
// like h/L and the two cancel into 1/L^2. The emission function then takes
// the integrand to zero at h = 0, since no light leaves a source along its
// own horizon. So the lower limit is a genuine zero rather than a removable
// singularity, and the quadrature needs no special case — but a source
// directly underfoot, L = 0, is the geometry where that cancellation fails,
// and is why an emitter at the observer's own position is not meaningful
// here.
func (c *CloudySkyglow) addAirTerm(
	dst SpectralRadiance,
	buf []float64,
	grid unit.SpectralGrid,
	geom skyglowGeometry,
	optics *skyglowOptics,
	emitter GroundEmitter,
	toObserver angle.Angle,
	topM float64,
) error {
	if geom.cosZenith <= 0 {
		return nil
	}

	step := topM / float64(c.steps)

	// Simpson's rule: the endpoints once, then alternating fours and twos.
	for k := range c.steps + 1 {
		h := float64(k) * step
		if h <= 0 {
			continue // the integrand vanishes at the ground
		}

		weight := simpsonWeight(k, c.steps)

		if err := c.accumulateStep(dst, buf, grid, geom, optics,
			emitter, toObserver, h, weight*step/3/geom.cosZenith); err != nil {
			return err
		}
	}

	return nil
}

// accumulateStep adds one quadrature node of the height integral.
func (c *CloudySkyglow) accumulateStep(
	dst SpectralRadiance,
	buf []float64,
	grid unit.SpectralGrid,
	geom skyglowGeometry,
	optics *skyglowOptics,
	emitter GroundEmitter,
	toObserver angle.Angle,
	h, weight float64,
) error {
	cosZ0, theta := geom.atHeight(h)
	if cosZ0 <= 0 {
		return nil
	}

	elevation := angle.Rad(math.Asin(math.Min(1, cosZ0)))

	if err := emitter.SourceRadiance(buf, grid, toObserver, elevation); err != nil {
		return fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	geometric := cosZ0 * cosZ0 / (h * h)

	for i := range dst {
		t, err := optics.transmission(i, h, geom.cosZenith, cosZ0)
		if err != nil {
			return err
		}

		psi, err := optics.scattering(i, h, theta)
		if err != nil {
			return err
		}

		dst[i] += buf[i] * geometric * t * psi * weight
	}

	return nil
}

// simpsonWeight returns the composite Simpson coefficient at node k of n
// intervals: 1 at the ends, 4 at odd nodes and 2 at even interior ones.
func simpsonWeight(k, n int) float64 {
	switch {
	case k == 0 || k == n:
		return 1
	case k%2 == 1:
		return 4
	default:
		return 2
	}
}
