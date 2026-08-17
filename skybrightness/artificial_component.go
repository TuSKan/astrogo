package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// ErrNoEmitters is returned when an ArtificialSkyglow is built with no
// sources. An empty artificial component would report a dark sky over a city
// with no way to tell that apart from a genuinely dark site.
var ErrNoEmitters = errors.New("skybrightness: artificial skyglow needs at least one emitter")

// ArtificialSkyglow is the artificial-light [Component]: ground sources
// propagated to the observer's sky by the semi-analytic model of Kocifaj,
// Bará & Falchi (2022).
//
// Each source contributes independently and the contributions sum in linear
// radiance space, which is what makes a whole-sky map from many cities
// tractable and is also the only correct way to add them.
//
// # The two things the paper does not specify
//
// Eq. 2 needs a source radiance L_S and an airmass toward the source M_S.
// Kocifaj & Bará (2019) Eq. 9 — the model this one generalises — defines L_S
// precisely: **the line-of-sight radiance, measured at the detector, of the
// city located on the horizon at that azimuth.** It is what a photometer
// pointed at the horizon in the source's direction would read, not a
// property of the source's surface, and the paper says it may be "inferred
// from satellite radiance data". The remaining choices are made here
// explicitly rather than buried, because they move the answer:
//
//  1. **M_S is the horizon airmass**, in the model's own airmass formula —
//     Gushchin (1988) via [atmosphere.GushchinAirmass], which Kocifaj & Bará
//     (2019) Eq. 3 adopts and which gives 35.7 at the horizon rather than
//     Pickering's 38. The paper's own stated limit is that Eq. 2 reduces to
//     L_S·P·(1−g)²/(1+g) when looking at the horizon, and that holds exactly
//     when M(z) reaches M_S. A ground source beyond a few kilometres sits at
//     the observer's horizon, so this is what makes the model self-consistent
//     with its own limit.
//
//  2. **Light leaves the source horizontally**, so the emission function is
//     evaluated at zero elevation above the source's horizon. That is the
//     same geometry, and it is the part of a luminaire's output that reaches
//     a distant sky at all — which is why [UpwardEmission] carries
//     HorizontalFraction as a separate term from its cosine lobe. Override
//     with [WithEscapeElevation] if a source's geometry says otherwise.
//
// Distance does not appear in Eq. 2 directly. It enters through the
// atmospheric parameter t of [OpticalParameterT], and through the
// transmission e^{−M_S·t} that this component applies to each emitter's
// output before handing it to [AllSkyRadiance] — which is that function's
// documented contract, and the single easiest thing to get wrong about the
// model.
//
// A value is safe for concurrent use, caching per-scene geometry behind a
// read-write lock and pooling its per-direction scratch space.
type ArtificialSkyglow struct {
	emitters []GroundEmitter
	escape   angle.Angle

	mu     sync.RWMutex
	cached *artificialGeometry

	scratch sync.Pool
}

// ArtificialOption configures an [ArtificialSkyglow].
type ArtificialOption func(*ArtificialSkyglow)

// WithEscapeElevation sets the elevation above a source's own horizon at
// which its emission function is evaluated, overriding the default of zero.
//
// Raise it for a source whose light reaches the observer's sky along a
// steeper path — a nearby installation rather than a distant city — bearing
// in mind that the rest of the model still treats the source as sitting at
// the horizon.
func WithEscapeElevation(e angle.Angle) ArtificialOption {
	return func(a *ArtificialSkyglow) { a.escape = e }
}

// artificialGeometry is everything that depends on the scene and the grid
// but not on the viewing direction.
type artificialGeometry struct {
	observer *coord.Geodetic
	at       time.Time
	grid     unit.SpectralGrid

	sources []artificialSource
	flags   Flag
}

// artificialSource is one emitter, resolved against a scene.
type artificialSource struct {
	// direction is where the source sits in the observer's sky: at the
	// horizon, in the source's azimuth.
	direction coord.AltAz

	airmass float64

	// Per-band, on the scene's grid.
	rayleigh  []unit.OpticalDepth
	aerosol   []unit.OpticalDepth
	asymmetry []float64
	optical   []float64 // the parameter t of Eq. 3
	arriving  []float64 // L_S after transmission over the separation
}

// artificialScratch is one direction's working space.
type artificialScratch struct {
	radiance []float64
}

// NewArtificialSkyglow builds the artificial-skyglow component over a set of
// ground sources.
func NewArtificialSkyglow(emitters []GroundEmitter, opts ...ArtificialOption) (*ArtificialSkyglow, error) {
	if len(emitters) == 0 {
		return nil, ErrNoEmitters
	}

	for i, e := range emitters {
		if e == nil || e.Location() == nil {
			return nil, fmt.Errorf("%w: emitter %d has no location", ErrNoEmitterLocation, i)
		}
	}

	a := &ArtificialSkyglow{emitters: append([]GroundEmitter(nil), emitters...)}
	for _, opt := range opts {
		opt(a)
	}

	a.scratch.New = func() any { return &artificialScratch{} }

	return a, nil
}

// ID implements [Component].
func (a *ArtificialSkyglow) ID() ComponentID { return Artificial }

// AddRadiance implements [Component].
func (a *ArtificialSkyglow) AddRadiance(
	_ context.Context,
	dst SpectralRadiance,
	grid unit.SpectralGrid,
	dir coord.AltAz,
	scene *Scene,
) (Flag, error) {
	if err := scene.Validate(); err != nil {
		return 0, err
	}

	if len(dst) != grid.Len() {
		return 0, fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	geom, err := a.geometry(scene, grid)
	if err != nil {
		return 0, err
	}

	if dir.Alt() <= 0 {
		return geom.flags, nil
	}

	// Gushchin's airmass, not Pickering's: the two-index model is calibrated
	// against it, and its horizon value of 35.7 rather than 38 is baked into
	// the kernel's own limit.
	viewAirmass, err := atmosphere.GushchinAirmass(dir.Alt())
	if err != nil {
		return 0, fmt.Errorf("skybrightness: artificial: view airmass: %w", err)
	}

	view := dir.ToUnitVector()

	scratch, _ := a.scratch.Get().(*artificialScratch)
	defer a.scratch.Put(scratch)

	if cap(scratch.radiance) < grid.Len() {
		scratch.radiance = make([]float64, grid.Len())
	}

	total := scratch.radiance[:grid.Len()]
	clear(total)

	for i := range geom.sources {
		src := &geom.sources[i]

		scatter := angle.Acos(clamp(src.direction.ToUnitVector().Dot(view), -1, 1))

		if err := src.addTo(total, scatter, src.airmass, viewAirmass); err != nil {
			return 0, err
		}
	}

	for i := range dst {
		dst[i] += total[i]
	}

	return geom.flags, nil
}

// addTo accumulates one source's contribution across the band set.
//
// The molecular phase function depends only on the scattering angle, not on
// wavelength, so it is evaluated once here rather than once per band inside
// atmosphere.CombinedPhaseFunction. The weighting that follows is that
// function's own Eq. 12 combination, kept identical to it — this is a hoist,
// not a second implementation.
func (s *artificialSource) addTo(dst []float64, scatter angle.Angle, airmassSource, airmassView float64) error {
	theta := scatter.Radians()
	molecular := atmosphere.RayleighPhaseFunction(theta, atmosphere.RayleighDepolarisation)

	for i := range dst {
		if s.arriving[i] <= 0 {
			continue
		}

		aerosolPhase, err := atmosphere.HenyeyGreensteinPhaseFunction(theta, s.asymmetry[i])
		if err != nil {
			return fmt.Errorf("skybrightness: artificial: %w", err)
		}

		total := float64(s.rayleigh[i] + s.aerosol[i])
		if total <= 0 {
			continue
		}

		phase := (float64(s.rayleigh[i])/total)*molecular + (float64(s.aerosol[i])/total)*aerosolPhase

		l, err := AllSkyRadiance(s.arriving[i], phase, s.asymmetry[i],
			airmassSource, airmassView, s.optical[i])
		if err != nil {
			return fmt.Errorf("skybrightness: artificial: %w", err)
		}

		dst[i] += l
	}

	return nil
}

// Provenance implements [Component].
func (a *ArtificialSkyglow) Provenance() Provenance {
	return Provenance{
		Model:            "Kocifaj, Bara & Falchi (2022) semi-analytic artificial all-sky radiance",
		Version:          "Eq. 1-5",
		PrimaryReference: "Kocifaj, M., Bara, S. & Falchi, F. (2022), MNRAS Letters 513, L25; arXiv:2203.09322",
		SecondaryReferences: []string{
			"Kocifaj, M. & Bara, S. (2019), the model this one generalises",
			"Bucholtz, A. (1995), Appl. Opt. 34, 2765 (Rayleigh)",
			"Henyey, L.G. & Greenstein, J.L. (1941), ApJ 93, 70",
			"Pickering, K.A. (2002), DIO 12, 20 (airmass)",
		},
		Equations: "Eq. 1 as atmosphere.CombinedPhaseFunction; Eq. 2 as AllSkyRadiance; " +
			"Eq. 3 as OpticalParameterT; Eq. 4/5 as AsymmetryParameter",
		ValidityDomain: "Clear sky. The asymmetry parameterisation was solved at 450 and " +
			"550 nm and represents a band roughly 20-30 nm wide, so this component is " +
			"less spectrally resolved than the grid it writes onto. Ground sources " +
			"beyond a few kilometres, where the horizon-source geometry holds.",
		KnownApproximations: []string{
			"M_S is taken as the horizon airmass in Gushchin's formula, and the " +
				"emission function is evaluated at zero elevation; the paper " +
				"specifies the latter nowhere.",
			"Sources are azimuthally symmetric, as Eq. 2 itself assumes.",
			"The asymmetry parameter is evaluated per band from Eq. 4/5, outside the " +
				"two wavelengths it was fitted at; values that leave (-1, 1) are " +
				"clamped and flagged rather than failing the evaluation.",
			"No cloud, no ground reflection, no obstruction by terrain.",
			"Source spectra and upward emission functions are whatever the emitters " +
				"supply, and each emitter reports whether those were measured or assumed.",
		},
		ExpectedAccuracy: "The paper validates against a multiple-scattering code to fifth " +
			"order; the dominant uncertainty in practice is the source inventory, not " +
			"the propagation.",
	}
}

// geometry returns the direction-independent part, recomputing it only when
// the scene or the grid changes.
func (a *ArtificialSkyglow) geometry(scene *Scene, grid unit.SpectralGrid) (*artificialGeometry, error) {
	a.mu.RLock()
	cached := a.cached
	a.mu.RUnlock()

	if cached.matches(scene, grid) {
		return cached, nil
	}

	fresh, err := a.computeGeometry(scene, grid)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	a.cached = fresh
	a.mu.Unlock()

	return fresh, nil
}

// matches reports whether this geometry was computed for the given scene.
func (g *artificialGeometry) matches(scene *Scene, grid unit.SpectralGrid) bool {
	return g != nil && g.at.Equal(scene.Time) && g.observer == scene.Observer && g.grid.Equal(grid)
}

// computeGeometry resolves every source against the scene.
func (a *ArtificialSkyglow) computeGeometry(scene *Scene, grid unit.SpectralGrid) (*artificialGeometry, error) {
	_, temperature := scene.Atmosphere.Surface()

	molecularScaleHeight, err := atmosphere.MolecularScaleHeight(temperature)
	if err != nil {
		return nil, fmt.Errorf("skybrightness: artificial: %w", err)
	}

	// The source sits at the observer's horizon, so the airmass toward it is
	// the airmass at zero elevation. See the type's doc comment.
	airmassSource, err := atmosphere.GushchinAirmass(0)
	if err != nil {
		return nil, fmt.Errorf("skybrightness: artificial: horizon airmass: %w", err)
	}

	geom := &artificialGeometry{
		observer: scene.Observer,
		at:       scene.Time,
		grid:     grid,
		sources:  make([]artificialSource, 0, len(a.emitters)),
	}

	for _, emitter := range a.emitters {
		src, flags, err := a.resolveSource(emitter, scene, grid, airmassSource, molecularScaleHeight)
		if err != nil {
			return nil, err
		}

		geom.flags |= flags
		geom.sources = append(geom.sources, src)
	}

	return geom, nil
}

// resolveSource turns one emitter into its per-band propagation state.
func (a *ArtificialSkyglow) resolveSource(
	emitter GroundEmitter,
	scene *Scene,
	grid unit.SpectralGrid,
	airmassSource, molecularScaleHeight float64,
) (artificialSource, Flag, error) {
	var zero artificialSource

	at := emitter.Location()

	separation, err := coord.GroundDistance(scene.Observer, at)
	if err != nil {
		return zero, 0, fmt.Errorf("skybrightness: artificial: %w", err)
	}

	toSource, err := coord.InitialBearing(scene.Observer, at)
	if err != nil {
		return zero, 0, fmt.Errorf("skybrightness: artificial: %w", err)
	}

	toObserver, err := coord.InitialBearing(at, scene.Observer)
	if err != nil {
		return zero, 0, fmt.Errorf("skybrightness: artificial: %w", err)
	}

	n := grid.Len()
	src := artificialSource{
		direction: coord.NewAltAz(0, toSource),
		airmass:   airmassSource,
		rayleigh:  make([]unit.OpticalDepth, n),
		aerosol:   make([]unit.OpticalDepth, n),
		asymmetry: make([]float64, n),
		optical:   make([]float64, n),
		arriving:  make([]float64, n),
	}

	if err := emitter.SourceRadiance(src.arriving, grid, toObserver, a.escape); err != nil {
		return zero, 0, fmt.Errorf("skybrightness: artificial: %w", err)
	}

	flags := emitter.Quality()

	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()

	aerosolScaleHeight := float64(aerosol.BoundaryLayerHeight)
	if aerosolScaleHeight <= 0 {
		return zero, 0, fmt.Errorf("%w: the atmosphere has no aerosol boundary-layer height",
			ErrScaleHeight)
	}

	for i := range n {
		lambda := grid.At(i)

		rayleigh, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return zero, 0, fmt.Errorf("skybrightness: artificial: %w", err)
		}

		aer := unit.OpticalDepth(aerosol.TauAt(lambda))

		t, err := OpticalParameterT(aer, unit.OpticalDepth(aerosolScaleHeight),
			rayleigh, unit.OpticalDepth(molecularScaleHeight), separation, airmassSource)
		if err != nil {
			return zero, 0, fmt.Errorf("skybrightness: artificial: %w", err)
		}

		g, err := AsymmetryParameter(aerosol.Asymmetry, aer)
		if err != nil {
			// The published fit is not bounded to (-1, 1). Rather than fail
			// the whole sky for a hazy atmosphere, clamp into the range a
			// Henyey-Greenstein phase function is defined on and say so.
			if !errors.Is(err, ErrAsymmetryOutOfRange) {
				return zero, 0, fmt.Errorf("skybrightness: artificial: %w", err)
			}

			g = clamp(g, -maxAsymmetry, maxAsymmetry)
			flags |= ExtrapolatedModel
		}

		src.rayleigh[i] = rayleigh
		src.aerosol[i] = aer
		src.asymmetry[i] = g
		src.optical[i] = t

		// L_S must reach AllSkyRadiance already attenuated over the
		// separation; M_S*t is exactly that optical depth.
		src.arriving[i] *= math.Exp(-airmassSource * t)
	}

	return src, flags, nil
}

// maxAsymmetry bounds the clamp applied when Eq. 4 leaves the physical
// range. It is short of 1 by enough that the Henyey-Greenstein denominator
// stays well conditioned at forward scattering.
const maxAsymmetry = 0.95
