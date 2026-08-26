package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for the cloudy-sky artificial component.
var (
	// ErrPartialCloud is returned for a cloud layer that covers part of the
	// sky. See [CloudySkyglow] for why this is refused rather than
	// approximated.
	ErrPartialCloud = errors.New("skybrightness: fractional cloud cover is not modelled")

	// ErrCloudBase is returned when a cloud layer's base altitude is not
	// above the observer.
	ErrCloudBase = errors.New("skybrightness: cloud base must be above the observer")

	// ErrIntegrationSteps is returned for a non-positive step count.
	ErrIntegrationSteps = errors.New("skybrightness: height integration needs at least one step")
)

// DefaultCloudySteps is how finely the height integral is sampled.
//
// The integrand is smooth in h but not flat: it is killed at the ground by the
// emission function going to zero at the source's horizon, rises through the
// layer where the line of sight passes closest to the source, and is damped
// above by extinction. Sixty-four intervals of Simpson's rule put the
// quadrature error well below the uncertainty in any input; the count is
// exposed because a very close source concentrates the integrand into the
// lowest few hundred metres.
const DefaultCloudySteps = 64

// ClearSkyTopM is the altitude the height integral runs to when there is no
// cloud.
//
// Kocifaj (2007) integrates to the cloud base, and a cloudless sky has none,
// so the upper limit becomes the top of the scattering atmosphere. At 100 km
// the molecular term has fallen by exp(-12.5) against the ground on an 8 km
// scale height, which contributes below one part in a hundred thousand — far
// under any other error here, and cheap because the integrand is already
// negligible over most of that range.
const ClearSkyTopM = 100_000

// CloudySkyglow is artificial skyglow under a cloud deck, after
// Kocifaj (2007).
//
// # What it computes
//
// Kocifaj (2007) Eq. 27, the night-sky radiance from ground sources with an
// overlying reflecting cloud layer. Radiance in one direction is the sum of
// two physically distinct paths:
//
//	L = L_cloud + L_air
//
//	L_cloud = 2*pi * rho * cos^4(z0_H) * S(z0_H) * T(H,z,phi) / H^2
//	L_air   = (1/cos z) * INT[0,H] S(z0_h) cos^2(z0_h) [T(h,z,phi)/h^2] Psi(h,Theta_h) dh
//
// The first is light that reached the cloud, reflected off its base and came
// back down. The second is light scattered by the air between the ground and
// the cloud. `S` is the source term, `T` the two-leg transmission (up from the
// source, down to the observer), `Psi` the angular volume scattering
// coefficient at height h, and `rho` the cloud's reflectance.
//
// # Why this is a separate component from [ArtificialSkyglow]
//
// Both compute the same physical contribution — artificial light scattered
// into the observer's line of sight — by different solutions.
// [ArtificialSkyglow] is Kocifaj, Bará & Falchi (2022): an analytic
// expression with a single atmospheric parameter and no height integral,
// which is fast and has no way to represent a cloud. This one resolves the
// vertical, which is what a cloud deck requires, and costs an integral per
// direction per source.
//
// They therefore share a [ComponentID], and [NewModel] refuses a model
// holding both. That is deliberate: two terms claiming the same contribution
// would double-count it, and which solution to use is a choice rather than a
// combination.
//
// # What it does not do
//
// Fractional cloud cover. Kocifaj (2007) solves the clear sky and the
// overcast sky, and this implements both; a deck covering part of the sky is
// the subject of Kocifaj, Falchi & Kundracik (2025), which adds an
// above-cloud scattering term and a line-of-sight cloud opacity that have no
// counterpart here. A layer with a fraction strictly between zero and one is
// refused with [ErrPartialCloud] rather than evaluated as overcast, because
// the difference is not small: over a city the 2025 paper reports overcast
// amplifying zenith radiance more than fifteenfold, so returning the overcast
// answer for a tenth of a sky's cover would be wrong by an order of magnitude
// in the direction that flatters the model.
type CloudySkyglow struct {
	emitters []GroundEmitter
	steps    int
}

// CloudyOption configures a [CloudySkyglow].
type CloudyOption func(*CloudySkyglow)

// WithCloudySteps sets the number of height-integration intervals.
func WithCloudySteps(n int) CloudyOption {
	return func(c *CloudySkyglow) { c.steps = n }
}

// NewCloudySkyglow builds the component over a ground-emitter inventory.
//
// The inventory is the caller's, for the reason [ArtificialSkyglow] states:
// satellite radiance alone cannot determine a source spectrum or an upward
// emission function, so an invented inventory would be reporting somebody
// else's city.
func NewCloudySkyglow(emitters []GroundEmitter, opts ...CloudyOption) (*CloudySkyglow, error) {
	if len(emitters) == 0 {
		return nil, ErrNoEmitters
	}

	c := &CloudySkyglow{
		emitters: append([]GroundEmitter(nil), emitters...),
		steps:    DefaultCloudySteps,
	}

	for _, o := range opts {
		o(c)
	}

	if c.steps < 1 {
		return nil, fmt.Errorf("%w: got %d", ErrIntegrationSteps, c.steps)
	}

	// Simpson's rule pairs its intervals, so an odd count would silently
	// drop one.
	if c.steps%2 == 1 {
		c.steps++
	}

	for _, e := range c.emitters {
		if e == nil || e.Location() == nil {
			return nil, ErrNoEmitterLocation
		}
	}

	return c, nil
}

// ID implements [Component]. Shared with [ArtificialSkyglow] on purpose; see
// the type's own documentation.
func (c *CloudySkyglow) ID() ComponentID { return Artificial }

// Provenance implements [Component].
func (c *CloudySkyglow) Provenance() Provenance {
	return Provenance{
		Model:   "Kocifaj (2007) height-resolved artificial sky radiance with a reflecting cloud deck",
		Version: "Eq. 27",
		PrimaryReference: "Kocifaj, M. (2007), Appl. Opt. 46, 3013: Light-pollution model " +
			"for cloudy and cloudless night skies with ground-based light sources",
		SecondaryReferences: []string{
			"Kocifaj, M., Falchi, F. & Kundracik, F. (2025), PNAS 122(44) e2508001122, " +
				"which extends this to a fractional 3D cloud field",
			"Garstang, R.H. (1986), PASP 98, 364 (upward emission function)",
			"Gushchin, G.P. (1988), by way of Kocifaj & Bara (2019) (airmass)",
			"Henyey, L.G. & Greenstein, J.L. (1941), ApJ 93, 70",
		},
		Equations: "Eq. 10 and the ray geometry as skyglowGeometry.atHeight; Eq. 18 with " +
			"Eq. 36's profiles as atmosphere.VolumeScatteringFunction over " +
			"atmosphere.ExponentialExtinction; Eq. 9/11 as the two-leg transmission; " +
			"Eq. 27's two terms as addCloudTerm and addAirTerm",
		ValidityDomain: "Clear sky or a single overcast deck. Ground sources at a horizontal " +
			"distance large enough that the source subtends little solid angle at the " +
			"cloud, which the paper states as sqrt(4*A_0/pi) < H/10.",
		KnownApproximations: []string{
			"Fractional cloud cover is refused rather than approximated; it needs the " +
				"above-cloud term and line-of-sight opacity of Kocifaj et al. (2025).",
			"One cloud deck. A second layer would return the first's downward light " +
				"upward again, which Eq. 27 has no term for.",
			"First scattering order, as Eq. 27 is derived. Kocifaj (2018) reports " +
				"higher orders as marginal below 30 km.",
			"Molecular and aerosol extinction both fall exponentially, the profiles " +
				"Eq. 36 adopts; real aerosol is not exponential, but a single decay " +
				"rate is what an operational aerosol product supplies.",
			"The cloud reflectance is a scalar rather than bidirectional, which the " +
				"paper itself states is usually satisfactory.",
			"Source spectra and upward emission functions are whatever the emitters " +
				"supply, and each emitter reports whether those were measured or assumed.",
		},
		ExpectedAccuracy: "Not established against measurement here. The paper reports " +
			"cloudless radiance falling about two orders of magnitude with angular " +
			"distance from the source, which this reproduces.",
	}
}

// AddRadiance implements [Component].
func (c *CloudySkyglow) AddRadiance(
	ctx context.Context,
	dst SpectralRadiance,
	grid unit.SpectralGrid,
	dir coord.AltAz,
	scene *Scene,
) (Flag, error) {
	if len(dst) != grid.Len() {
		return 0, fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	// Below the horizon there is no sky to look at.
	if dir.Alt() <= 0 {
		return 0, nil
	}

	top, reflectance, err := c.deck(scene)
	if err != nil {
		return 0, err
	}

	var flags Flag

	buf := make([]float64, grid.Len())

	for _, e := range c.emitters {
		if err := ctx.Err(); err != nil {
			return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
		}

		f, err := c.addEmitter(dst, buf, grid, dir, scene, e, top, reflectance)
		if err != nil {
			return 0, err
		}

		flags |= f
	}

	return flags, nil
}

// deck resolves the cloud layer into an integration ceiling and a
// reflectance, and reports zero reflectance for a clear sky.
func (c *CloudySkyglow) deck(scene *Scene) (topM float64, reflectance float64, err error) {
	clouds := scene.Atmosphere.Clouds()
	if len(clouds) == 0 {
		return ClearSkyTopM, 0, nil
	}

	// One deck. A second layer would reflect the first's downward light back
	// up again, which Eq. 27 has no term for.
	if len(clouds) > 1 {
		return 0, 0, fmt.Errorf("%w: %d layers, and Eq. 27 solves one",
			ErrPartialCloud, len(clouds))
	}

	layer := clouds[0]

	if f := float64(layer.Fraction); f > 0 && f < 1 {
		return 0, 0, fmt.Errorf("%w: cover is %.2f", ErrPartialCloud, f)
	}

	if layer.Fraction == 0 {
		return ClearSkyTopM, 0, nil
	}

	base := float64(layer.BaseAlt)
	if base <= 0 || math.IsInf(base, 0) || math.IsNaN(base) {
		return 0, 0, fmt.Errorf("%w: got %g m", ErrCloudBase, base)
	}

	return base, float64(layer.Albedo), nil
}

// addEmitter accumulates one source's contribution.
func (c *CloudySkyglow) addEmitter(
	dst SpectralRadiance,
	buf []float64,
	grid unit.SpectralGrid,
	dir coord.AltAz,
	scene *Scene,
	emitter GroundEmitter,
	topM, reflectance float64,
) (Flag, error) {
	at := emitter.Location()

	horizontal, err := coord.GroundDistance(scene.Observer, at)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	toSource, err := coord.InitialBearing(scene.Observer, at)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	toObserver, err := coord.InitialBearing(at, scene.Observer)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
	}

	geom := skyglowGeometry{
		horizontalM: horizontal,
		// The azimuth of the source measured at the observer, against the
		// azimuth being looked at: only their difference enters Eq. 10.
		deltaAzimuth: dir.Az().Radians() - toSource.Radians(),
		tanZenith:    math.Tan(math.Pi/2 - dir.Alt().Radians()),
		cosZenith:    dir.Alt().Sin(),
	}

	optics, err := newSkyglowOptics(grid, scene)
	if err != nil {
		return 0, err
	}

	flags := emitter.Quality()

	// The cloud term, evaluated once: it is a single reflection at one
	// altitude rather than an integral.
	if reflectance > 0 {
		if err := c.addCloudTerm(dst, buf, grid, geom, optics,
			emitter, toObserver, topM, reflectance); err != nil {
			return 0, err
		}
	}

	if err := c.addAirTerm(dst, buf, grid, geom, optics, emitter, toObserver, topM); err != nil {
		return 0, err
	}

	return flags, nil
}

// skyglowGeometry holds the direction-and-source geometry that does not
// change with height.
type skyglowGeometry struct {
	horizontalM  float64
	deltaAzimuth float64
	tanZenith    float64
	cosZenith    float64
}

// atHeight returns the geometry of the scattering point at altitude h: the
// cosine of the zenith angle at the source, the scattering angle there, and
// the slant distance from source to that point.
//
// Kocifaj (2007) Eq. 10 for the first, and the angle between the two ray
// directions for the second. Both are computed from the same vectors rather
// than from the paper's trigonometric identities, because the vector form has
// no branch to get wrong near the zenith or the horizon and reduces to the
// identities anyway.
//
// The observer sits at the origin, looks along the direction of interest, and
// the scattering point is where that line of sight crosses altitude h.
func (g skyglowGeometry) atHeight(h float64) (cosZ0, theta float64) {
	// Line of sight footprint at height h, in a frame whose x axis points at
	// the source's azimuth.
	losX := h * g.tanZenith * math.Cos(g.deltaAzimuth)
	losY := h * g.tanZenith * math.Sin(g.deltaAzimuth)

	// Source to scattering point. The source sits at (L, 0) in this frame,
	// so its offset is subtracted: adding it puts the source diametrically
	// opposite where it is, which makes the sky brightest in the direction
	// facing away from the city.
	dx := losX - g.horizontalM
	dy := losY
	dz := h

	slant := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if slant == 0 {
		return 0, 0
	}

	cosZ0 = dz / slant

	// The scattering angle is between the incoming propagation direction
	// (source to point) and the outgoing one (point to observer, at the
	// origin). Straight up and straight back is 180 degrees, which is the
	// backscatter a source directly below the zenith produces.
	losLen := math.Sqrt(losX*losX + losY*losY + h*h)
	if losLen == 0 {
		return cosZ0, 0
	}

	cosTheta := (dx*(-losX) + dy*(-losY) + dz*(-h)) / (slant * losLen)

	return cosZ0, math.Acos(math.Min(1, math.Max(-1, cosTheta)))
}
