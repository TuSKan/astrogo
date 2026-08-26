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

// stepKernel is everything about one point on the line of sight that does not
// depend on wavelength.
//
// # Why this exists
//
// The height integral runs a spectral loop inside a height loop, and most of
// what the inner loop was computing is constant across it. The airmass along
// each leg depends only on a zenith angle; both phase functions depend only on
// the scattering angle and the asymmetry; and the exponential profiles factor
// into a wavelength-independent shape times a per-wavelength column depth,
// because only the column varies with wavelength and the decay does not.
//
// Recomputing those per wavelength meant two square roots, a 1.5 power and
// four exponentials repeated 671 times per height step for values that never
// changed. Hoisting them leaves the inner loop with one exponential and a
// handful of multiplies. This is the same optimisation the Eq. 11 scattering
// kernel already carries, and it is worth repeating for the same reason: the
// cost of this component is that inner loop and nothing else.
type stepKernel struct {
	// airmassSum is the total slant weighting of the two legs, up from the
	// source and down to the observer. They share one vertical depth, so the
	// two transmissions combine into a single exponential.
	airmassSum float64

	// depthFractionM and depthFractionA are (1 - exp(-h/H)) for the two
	// profiles: the share of each column lying below this altitude.
	depthFractionM, depthFractionA float64

	// extinctionShapeM and extinctionShapeA are exp(-h/H)/H, the profile
	// shape a column depth is multiplied by to give a local extinction.
	extinctionShapeM, extinctionShapeA float64

	// The two phase functions at this scattering angle, already normalised
	// per steradian.
	rayleighPhase, aerosolPhase float64
}

// newStepKernel evaluates the wavelength-independent terms once.
//
// Returned by value rather than by pointer: it is built once per height step
// and read once per wavelength, so a pointer would put one heap allocation on
// every step of every direction for a struct that never outlives the loop.
func (o *skyglowOptics) newStepKernel(
	heightM, cosObserver, cosSource, theta float64,
) (kernel stepKernel, ok bool, err error) {
	if cosObserver <= 0 || cosSource <= 0 {
		return kernel, false, nil
	}

	observerAir, err := legAirmass(cosObserver)
	if err != nil {
		return kernel, false, err
	}

	sourceAir, err := legAirmass(cosSource)
	if err != nil {
		return kernel, false, err
	}

	aerosolPhase, err := atmosphere.HenyeyGreensteinPhaseFunction(theta, o.asymmetry)
	if err != nil {
		return kernel, false, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	decayM := math.Exp(-heightM / o.molecularScaleM)
	decayA := math.Exp(-heightM / o.aerosolScaleM)

	return stepKernel{
		airmassSum:       observerAir + sourceAir,
		depthFractionM:   1 - decayM,
		depthFractionA:   1 - decayA,
		extinctionShapeM: decayM / o.molecularScaleM,
		extinctionShapeA: decayA / o.aerosolScaleM,
		rayleighPhase:    atmosphere.RayleighPhaseFunction(theta, atmosphere.RayleighDepolarisation),
		aerosolPhase:     aerosolPhase,
	}, true, nil
}

// legAirmass returns the slant weighting at a cosine of zenith angle, using
// the airmass this model lineage adopts.
func legAirmass(cosZenith float64) (float64, error) {
	m, err := atmosphere.GushchinAirmass(angle.Rad(math.Asin(math.Min(1, cosZenith))))
	if err != nil {
		return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	return m, nil
}

// transmission is Kocifaj (2007) Eq. 11 at one wavelength: the two legs of
// Eq. 9 combined, since both weight the same vertical depth.
//
//	T = exp(-tau(0,h) * [M(z) + M(z0_h)])
func (k stepKernel) transmission(o *skyglowOptics, i int) float64 {
	vertical := float64(o.molecular[i])*k.depthFractionM + float64(o.aerosol[i])*k.depthFractionA

	return math.Exp(-vertical * k.airmassSum)
}

// scattering is Kocifaj (2007) Eq. 18 with the Eq. 36 profiles, at one
// wavelength: the angular volume scattering coefficient at this point.
//
// The aerosol term carries the single-scattering albedo because only what it
// scatters can reach the observer; molecular scattering is conservative.
func (k stepKernel) scattering(o *skyglowOptics, i int) float64 {
	molecular := float64(o.molecular[i]) * k.extinctionShapeM
	aerosol := float64(o.aerosol[i]) * k.extinctionShapeA * o.aerosolAlbedo

	return molecular*k.rayleighPhase + aerosol*k.aerosolPhase
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

	kernel, ok, err := optics.newStepKernel(baseM, geom.cosZenith, cosZ0, 0)
	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	for i := range dst {
		dst[i] += buf[i] * scale * kernel.transmission(optics, i)
	}

	return nil
}

// addAirTerm accumulates a height range of the scattering integral, scaled.
//
//	scale * (1/cos z) * INT[a,b] S(z0_h) cos^2(z0_h) [T(h,z,phi)/h^2] Psi(h,Theta_h) dh
//
// One function serves both scattering terms because they share an integrand.
// Kocifaj (2007) Eq. 27 integrates below the cloud, which is Kocifaj et al.
// (2025) Eq. 1; their Eq. 2 integrates above it and carries the extra
// transmissions T_h->H(z) and T_0->H(z), which compose to the T_0->h(z) this
// integrand already has — a photon scattered above the deck reaches the
// observer through the same total column as one scattered below it. So the
// two terms differ only in their limits and in Eq. 2's (1-CF)[1-o] weight,
// and at CF = 0 they sum to exactly the clear-sky integral over the whole
// atmosphere. TestCloudFractionZeroRecoversTheClearSky pins that.
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
	bottomM, topM, scale float64,
) error {
	if geom.cosZenith <= 0 || scale <= 0 || topM <= bottomM {
		return nil
	}

	// # Why the range is subdivided rather than sampled evenly across
	//
	// The integrand is spiked at its lower limit. Transmission reaches the
	// scattering point as exp(-M(z0_h) * tau(0,h)): as h falls the vertical
	// depth tau(0,h) goes to zero faster than the source's airmass grows, so
	// the factor climbs to one at the ground. For a city 60 km away it falls
	// by about 650 between 100 m and 1 km — a feature a few hundred metres
	// wide sitting at the bottom of a range that may run to 100 km.
	//
	// Sampling that evenly does not work. Sixty-four uniform nodes over the
	// whole column put the clear sky 4.6 times too low and still climbing at
	// sixteen thousand, which looks like a dim sky rather than like a bug. A
	// change of variable does not work either: an exponential substitution
	// concentrates nodes at the bottom, but its Jacobian grows like
	// exp(h/Href) while the integrand's tail falls on the molecular scale
	// height, so anything short enough to resolve the spike diverges at the
	// top.
	//
	// Dyadic subdivision does work, and is the ordinary answer for an
	// endpoint singularity: ranges that double in width from the bottom, each
	// integrated on its own. Every range then holds a comparable share of the
	// integral, and the node spacing tracks the structure automatically at
	// every scale rather than at one chosen scale.
	lo := bottomM
	if lo <= 0 {
		lo = dyadicFloorM
	}

	// Below the floor the emission function has taken the integrand to zero
	// at the source's own horizon, so nothing is lost by starting there.
	for width := lo; lo < topM; width *= 2 {
		hi := min(lo+width, topM)

		if err := c.simpsonRange(dst, buf, grid, geom, optics, emitter, toObserver,
			lo, hi, scale); err != nil {
			return err
		}

		lo = hi
	}

	return nil
}

// dyadicFloorM is where subdivision starts when a range begins at the ground.
//
// Ten centimetres, and the value was measured rather than chosen. The
// integrand is finite at the ground rather than singular — the 1/h^2 is
// cancelled by cos^2(z0_h), which falls like h/L — so the omitted sliver
// contributes about f(0) times the floor, and shrinking the floor converges.
// Measured against a city 60 km away it converges cleanly: a 10 m floor is
// 6.1 per cent low, 1 m is 0.63, 10 cm is 0.06 and 1 cm is 0.006. Ten
// centimetres is where that stops mattering against every other error here,
// and each further decade costs another octave of subdivision.
//
// The converged value agrees to 0.26 per cent with sixteen thousand uniform
// nodes over the same range — two quadratures with nothing in common
// arriving at the same number, which is the check that the scheme is right
// and not merely self-consistent.
const dyadicFloorM = 0.1

// simpsonRange integrates one contiguous height range by composite Simpson.
func (c *CloudySkyglow) simpsonRange(
	dst SpectralRadiance,
	buf []float64,
	grid unit.SpectralGrid,
	geom skyglowGeometry,
	optics *skyglowOptics,
	emitter GroundEmitter,
	toObserver angle.Angle,
	bottomM, topM, scale float64,
) error {
	step := (topM - bottomM) / float64(c.steps)

	// The endpoints once, then alternating fours and twos.
	for k := range c.steps + 1 {
		h := bottomM + float64(k)*step
		if h <= 0 {
			continue // the integrand vanishes at the ground
		}

		weight := simpsonWeight(k, c.steps)

		if err := c.accumulateStep(dst, buf, grid, geom, optics, emitter, toObserver,
			h, scale*weight*step/3/geom.cosZenith); err != nil {
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

	kernel, ok, err := optics.newStepKernel(h, geom.cosZenith, cosZ0, theta)
	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	geometric := cosZ0 * cosZ0 / (h * h) * weight

	// The inner loop, which is where this component's whole cost lives: one
	// exponential and a few multiplies per wavelength.
	for i := range dst {
		dst[i] += buf[i] * geometric * kernel.transmission(optics, i) * kernel.scattering(optics, i)
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
