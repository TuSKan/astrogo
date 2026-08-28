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
	// ErrCloudLayers is returned when a scene carries more than one cloud
	// deck, which neither published model solves.
	ErrCloudLayers = errors.New("skybrightness: only one cloud layer is modelled")

	// ErrCloudFraction is returned for a cover outside [0,1].
	ErrCloudFraction = errors.New("skybrightness: cloud fraction must be in [0,1]")

	// ErrCloudBase is returned when a cloud layer's base altitude is not
	// above the observer.
	ErrCloudBase = errors.New("skybrightness: cloud base must be above the observer")

	// ErrIntegrationSteps is returned for a non-positive step count.
	ErrIntegrationSteps = errors.New("skybrightness: height integration needs at least one step")
)

// DefaultCloudySteps is how many Simpson intervals each octave of the height
// integral gets.
//
// Per octave, not per integral: the range is subdivided dyadically, so this
// sets the resolution at every scale rather than one absolute spacing. Each
// octave holds a smooth piece of the integrand, which is why so few intervals
// suffice — measured against a city 60 km away, two per octave already give
// 0.2 per cent and four give 0.015, so eight is comfortable margin rather
// than a guess. A whole clear column is twenty octaves, so the default costs
// about 160 evaluations per direction.
const DefaultCloudySteps = 8

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

// CloudySkyglow is artificial skyglow under cloud, after Kocifaj (2007) and
// Kocifaj, Falchi & Kundracik (2025).
//
// # What it computes
//
// Radiance in one direction is three physically distinct paths, which the
// 2025 paper writes as L = L_1 + L_2 + L_infinity:
//
//	L_1   = (1/cos z) * INT[0,H] S(z0_h) cos^2(z0_h) [T/h^2] Psi dh
//	L_2   = (1-CF)[1-o(z,A)] * (1/cos z) * INT[H,top] (the same integrand)
//	L_inf = CF * 2*pi * alpha * cos^4(z0_H) * S(z0_H) * T(H,z,phi) / H^2
//
// L_1 is light scattered into the line of sight by the air below the cloud,
// L_2 the same above it, and L_infinity light that reached the cloud base and
// reflected back down. `S` is the source term, `T` the two-leg transmission
// (up from the source, down to the observer), `Psi` the angular volume
// scattering coefficient at height h, `alpha` the cloud reflectance, `CF` the
// cloud fraction and `o(z,A)` the opacity along the line of sight.
//
// # Why L_2 shares an integrand with L_1
//
// Because the transmissions compose. Eq. 2 carries T_h->H(z) and T_0->H(z)
// where Eq. 1 carries T_0->h(z), and the first two multiply to the third: a
// photon scattered above the deck reaches the observer through the same total
// column as one scattered below it. So the two terms differ only in their
// limits and in Eq. 2's weight, and at CF = 0 they sum to the clear-sky
// integral over the whole atmosphere exactly. That is not a coincidence to
// rely on quietly — it is a property worth testing, and
// TestCloudFractionZeroRecoversTheClearSky does.
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
// # Where this departs from the 2025 paper
//
// Two places, both because the paper states a quantity without giving a way
// to compute it.
//
// It derives the line-of-sight opacity from a stochastic 3D cloud field —
// randomised cuboids filled with cloud elements — and reads it off a ray cast
// through one realisation. The text does not specify that generator closely
// enough to reproduce, so [cloudDeck.opacity] takes the other route to the
// same quantity, Beer-Lambert through a deck of stated optical depth. The
// consequence is real and worth stating: a ray through a realised field is
// binary per realisation, and this is the ensemble mean, so a broken sky
// comes out as what its patchiness averages to rather than as patchiness.
//
// And its printed Eq. 3 carries no cloud-fraction weight, which cannot be
// right on its own — a sky one tenth covered would return the reflection of a
// whole deck. CF is the vertically projected area of cloud over the area of
// the zone, so it is the share of the reflecting surface that is present, and
// L_infinity is scaled by it here.
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
		Model: "Kocifaj (2007) height-resolved artificial sky radiance, extended to a " +
			"fractional cloud deck after Kocifaj, Falchi & Kundracik (2025)",
		Version: "Eq. 27; 2025 Eq. 1-3",
		PrimaryReference: "Kocifaj, M. (2007), Appl. Opt. 46, 3013: Light-pollution model " +
			"for cloudy and cloudless night skies with ground-based light sources",
		SecondaryReferences: []string{
			"Kocifaj, M., Falchi, F. & Kundracik, F. (2025), PNAS 122(44) e2508001122, " +
				"the fractional cloud field and the above-cloud term",
			"Garstang, R.H. (1986), PASP 98, 364 (upward emission function)",
			"Gushchin, G.P. (1988), by way of Kocifaj & Bara (2019) (airmass)",
			"Henyey, L.G. & Greenstein, J.L. (1941), ApJ 93, 70",
		},
		Equations: "Eq. 10 and the ray geometry as skyglowGeometry.atHeight; Eq. 18 with " +
			"Eq. 36's profiles as atmosphere.VolumeScatteringFunction over " +
			"atmosphere.ExponentialExtinction; Eq. 9/11 as the two-leg transmission; " +
			"Eq. 27's terms and the 2025 Eq. 2 as addCloudTerm and addAirTerm over two " +
			"height ranges",
		ValidityDomain: "Clear sky through overcast, one deck. Ground sources at a horizontal " +
			"distance large enough that the source subtends little solid angle at the " +
			"cloud, which the paper states as sqrt(4*A_0/pi) < H/10.",
		KnownApproximations: []string{
			"The line-of-sight opacity is Beer-Lambert through a deck of stated optical " +
				"depth, where Kocifaj et al. (2025) ray-cast a stochastic 3D cloud " +
				"field. This is the ensemble mean of that, so a broken sky is what its " +
				"patchiness averages to rather than patchiness itself.",
			"L_infinity is scaled by the cloud fraction. The 2025 paper's printed Eq. 3 " +
				"carries no such weight, and without one a tenth-covered sky returns " +
				"the reflection of a whole deck.",
			"One cloud deck. A second layer would return the first's downward light " +
				"upward again, which neither paper has a term for.",
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

	deck, err := c.deck(scene)
	if err != nil {
		return 0, err
	}

	var flags Flag

	buf := make([]float64, grid.Len())

	for _, e := range c.emitters {
		if err := ctx.Err(); err != nil {
			return 0, fmt.Errorf("skybrightness: cloudy skyglow: %w", err)
		}

		f, err := c.addEmitter(dst, buf, grid, dir, scene, e, deck)
		if err != nil {
			return 0, err
		}

		flags |= f
	}

	return flags, nil
}

// deck resolves the cloud layer into an integration ceiling and a
// reflectance, and reports zero reflectance for a clear sky.
type cloudDeck struct {
	// baseM is the cloud base, and the altitude that splits the two
	// scattering integrals. ClearSkyTopM when there is no cloud, which
	// leaves the whole atmosphere below the split and nothing above it.
	baseM float64

	// fraction is Kocifaj et al. (2025)'s CF: the vertically projected area
	// of cloud over the area of the zone.
	fraction float64

	// albedo is the cloud's reflectance, Eq. 3's alpha.
	albedo float64

	// opticalDepth is the vertical optical depth through the deck, from
	// which the line-of-sight opacity follows.
	opticalDepth float64
}

// opacity is Kocifaj et al. (2025)'s o(z,A): how opaque the deck is along one
// line of sight.
//
// # Where this comes from
//
// The paper does not print a formula for it. It derives the quantity from a
// stochastic 3D cloud field — randomised cuboids filled with cloud elements —
// and reads the opacity off a ray cast through that realisation. That
// generator is not specified closely enough in the text to reproduce, so this
// takes the other route to the same quantity: Beer-Lambert through the deck
// along the slant path,
//
//	o(z) = 1 - exp(-tau_c * M(z))
//
// which is what "opacity along the line of sight" means for a deck of stated
// optical depth. The difference is that a ray through a realised field is
// binary per realisation and this is the ensemble mean, so this cannot
// produce the patchiness of a broken sky in one direction — it produces what
// that patchiness averages to. That is recorded as a known approximation
// rather than presented as their model.
func (d cloudDeck) opacity(cosZenith float64) (float64, error) {
	if d.opticalDepth <= 0 || cosZenith <= 0 {
		return 0, nil
	}

	airmass, err := legAirmass(cosZenith)
	if err != nil {
		return 0, err
	}

	return 1 - math.Exp(-d.opticalDepth*airmass), nil
}

// deck resolves the scene's cloud layer.
func (c *CloudySkyglow) deck(scene *Scene) (cloudDeck, error) {
	clouds := scene.Atmosphere.Clouds()
	if len(clouds) == 0 {
		return cloudDeck{baseM: ClearSkyTopM}, nil
	}

	// One deck. A second layer would reflect the first's downward light back
	// up again, which neither paper has a term for.
	if len(clouds) > 1 {
		return cloudDeck{}, fmt.Errorf("%w: %d layers, and the model solves one",
			ErrCloudLayers, len(clouds))
	}

	layer := clouds[0]

	fraction := float64(layer.Fraction)
	if fraction <= 0 {
		return cloudDeck{baseM: ClearSkyTopM}, nil
	}

	if fraction > 1 || math.IsNaN(fraction) {
		return cloudDeck{}, fmt.Errorf("%w: cover is %g", ErrCloudFraction, fraction)
	}

	baseM := float64(layer.BaseAlt)
	if baseM <= 0 || math.IsInf(baseM, 0) || math.IsNaN(baseM) {
		return cloudDeck{}, fmt.Errorf("%w: got %g m", ErrCloudBase, baseM)
	}

	return cloudDeck{
		baseM:        baseM,
		fraction:     fraction,
		albedo:       float64(layer.Albedo),
		opticalDepth: float64(layer.OpticalDepth),
	}, nil
}

// addEmitter accumulates one source's contribution.
func (c *CloudySkyglow) addEmitter(
	dst SpectralRadiance,
	buf []float64,
	grid unit.SpectralGrid,
	dir coord.AltAz,
	scene *Scene,
	emitter GroundEmitter,
	deck cloudDeck,
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

	// L_infinity: one reflection at the cloud base, scaled by how much of the
	// sky is covered. Eq. 3 as printed carries no CF, and it needs one here:
	// without it a sky with a tenth of a cloud would return the reflection of
	// a whole deck. CF is the vertically projected area of cloud over the
	// area of the zone, so it is the share of the reflecting surface that is
	// actually there, and the albedo says how well that share reflects.
	if deck.albedo > 0 && deck.fraction > 0 {
		if err := c.addCloudTerm(dst, buf, grid, geom, optics, emitter, toObserver,
			deck.baseM, deck.albedo*deck.fraction); err != nil {
			return 0, err
		}
	}

	// L_1: scattering below the deck, always present.
	if err := c.addAirTerm(dst, buf, grid, geom, optics, emitter, toObserver,
		0, deck.baseM, 1); err != nil {
		return 0, err
	}

	// L_2: scattering above the deck, reaching the observer only through the
	// part of the sky the cloud does not block. Kocifaj et al. (2025) Eq. 2.
	if deck.baseM < ClearSkyTopM {
		opacity, err := deck.opacity(geom.cosZenith)
		if err != nil {
			return 0, err
		}

		if err := c.addAirTerm(dst, buf, grid, geom, optics, emitter, toObserver,
			deck.baseM, ClearSkyTopM, (1-deck.fraction)*(1-opacity)); err != nil {
			return 0, err
		}
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
