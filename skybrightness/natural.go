package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	astrotime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// ErrNoDustMap is returned when a diffuse-galactic-light component is built
// without one.
var ErrNoDustMap = errors.New("skybrightness: diffuse galactic light needs a 100 micron dust map")

// sceneFrame caches the direction-independent part of a scene: the SOFA
// transform context, which costs about 91 microseconds to build, and the
// Sun's position.
//
// Every natural-sky component needs the same thing — a viewing direction in
// alt-az has to reach ecliptic or galactic coordinates, and that goes through
// coord.Context. Building one per direction would dominate a full-sky
// evaluation, so each component holds one of these behind a read-write lock
// and rebuilds it only when the scene changes.
type sceneFrame struct {
	observer *coord.Geodetic
	at       time.Time

	ctx *coord.Context

	// sunEcliptic is the Sun's apparent ecliptic position; sunDistanceAU is
	// the observer's heliocentric distance.
	sunEcliptic   coord.Ecliptic
	sunDistanceAU float64
}

// matches reports whether this frame was built for the given scene.
func (f *sceneFrame) matches(scene *Scene) bool {
	return f != nil && f.at.Equal(scene.Time) && f.observer == scene.Observer
}

// newSceneFrame resolves a scene's transform context and Sun position.
//
// needSun is false for components that do not use it, so a diffuse-galactic
// or airglow evaluation does not require an ephemeris in the scene at all.
func newSceneFrame(scene *Scene, needSun bool) (*sceneFrame, error) {
	if err := scene.Validate(); err != nil {
		return nil, err
	}

	at := astrotime.FromGo(scene.Time)

	frame := &sceneFrame{
		observer: scene.Observer,
		at:       scene.Time,
		ctx:      coord.NewContext(at, scene.Observer, scene.Atmosphere.Refraction()),
	}

	if !needSun {
		return frame, nil
	}

	if scene.Ephemeris == nil {
		return nil, ErrNoEphemeris
	}

	sun, err := scene.Ephemeris.State(eph.Sun, at)
	if err != nil {
		return nil, fmt.Errorf("skybrightness: sun state: %w", err)
	}

	icrs, err := eph.ToICRS(sun.Pos)
	if err != nil {
		return nil, fmt.Errorf("skybrightness: sun direction: %w", err)
	}

	frame.sunEcliptic = coord.ICRSToEcliptic(icrs, at)
	frame.sunDistanceAU = sun.Pos.Norm()

	return frame, nil
}

// frameCache is the per-scene frame each natural-sky component holds.
type frameCache struct {
	mu      sync.RWMutex
	cached  *sceneFrame
	needSun bool
}

// get returns the frame for a scene, rebuilding only when the scene changes.
func (c *frameCache) get(scene *Scene) (*sceneFrame, error) {
	c.mu.RLock()
	cached := c.cached
	c.mu.RUnlock()

	if cached.matches(scene) {
		return cached, nil
	}

	fresh, err := newSceneFrame(scene, c.needSun)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cached = fresh
	c.mu.Unlock()

	return fresh, nil
}

// ── Diffuse galactic light ──────────────────────────────────────────────────

// DustMap samples the 100 micron diffuse emission of interstellar dust, in
// MJy sr^-1, by galactic coordinates.
//
// It is an interface because the map is a dataset: the Schlegel, Finkbeiner &
// Davis (1998) product is the usual one, and a caller with a newer reprocessing
// should be able to supply it without this package changing.
type DustMap interface {
	// IntensityAt returns the 100 micron intensity in MJy sr^-1 toward the
	// given galactic longitude and latitude.
	IntensityAt(l, b angle.Angle) (float64, error)
}

// DiffuseGalacticLight is the [Component] for starlight scattered by
// interstellar dust, evaluated through [DiffuseGalacticRadiance].
//
// It contributes typically 20 to 30 per cent of the Milky Way's integrated
// light (Leinert et al. 1998), so it is not a correction to integrated
// starlight but a term of comparable weight along dusty sightlines.
//
// A value is safe for concurrent use.
type DiffuseGalacticLight struct {
	dust  DustMap
	frame frameCache
}

// NewDiffuseGalacticLight builds the component over a 100 micron dust map.
func NewDiffuseGalacticLight(dust DustMap) (*DiffuseGalacticLight, error) {
	if dust == nil {
		return nil, ErrNoDustMap
	}

	return &DiffuseGalacticLight{dust: dust}, nil
}

// ID implements [Component].
func (d *DiffuseGalacticLight) ID() ComponentID { return DiffuseGalactic }

// AddRadiance implements [Component].
func (d *DiffuseGalacticLight) AddRadiance(
	_ context.Context,
	dst SpectralRadiance,
	grid unit.SpectralGrid,
	dir coord.AltAz,
	scene *Scene,
) (Flag, error) {
	if dir.Alt() <= 0 {
		return 0, nil
	}

	frame, err := d.frame.get(scene)
	if err != nil {
		return 0, err
	}

	icrs, err := frame.ctx.AltAzToICRS(dir)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: diffuse galactic: direction: %w", err)
	}

	galactic := coord.ICRSToGalactic(icrs)

	intensity, err := d.dust.IntensityAt(galactic.L(), galactic.B())
	if err != nil {
		// A direction the map does not cover is missing data, not a dark
		// sightline, so it contributes nothing and says so.
		return UnknownCloud, nil //nolint:nilerr // absence of map coverage is not a failure
	}

	return DiffuseGalacticRadiance(dst, grid, intensity)
}

// Provenance implements [Component].
func (d *DiffuseGalacticLight) Provenance() Provenance {
	return Provenance{
		Model:            "optical/100 micron correlation",
		PrimaryReference: "Kawara, K. et al. (2017), PASJ 69, 31",
		SecondaryReferences: []string{
			"Masana, E. et al. (2021), MNRAS 501, 5443, Eq. 13-14",
			"Schlegel, D.J., Finkbeiner, D.P. & Davis, M. (1998), ApJ 500, 525",
		},
		Equations:      "Kawara Eq. 7 as DiffuseGalacticRadiance",
		ValidityDomain: "100 micron intensity below 50 MJy/sr; 225 to 648 nm, held at the endpoints beyond",
		KnownApproximations: []string{
			"The quadratic coefficient's published power of ten is read as 1e-5 " +
				"against the printed 1e5; see docs/skybrightness.md section 17.",
			"Kawara's coefficients stop at 648 nm, so the red half of the optical " +
				"grid holds the endpoint values.",
			"The DGL-to-starlight ratio cap of 0.35 that Masana et al. apply needs " +
				"integrated starlight in the same direction and is not applied here.",
		},
		ExpectedAccuracy: "The correlation's own scatter is large; Kawara et al. quote " +
			"uncertainties of 10 to 50 per cent on the slope depending on band.",
	}
}

// ── Zodiacal light ──────────────────────────────────────────────────────────

// ZodiacalLight is the [Component] for sunlight scattered by interplanetary
// dust, evaluated through [ZodiacalRadiance].
//
// The geometry comes from the scene: the viewing direction is carried to
// ecliptic coordinates and differenced against the Sun's own ecliptic
// longitude, which is what Leinert et al.'s table is indexed by.
//
// A value is safe for concurrent use.
type ZodiacalLight struct {
	frame frameCache
}

// NewZodiacalLight builds the component. It needs an ephemeris in the scene
// for the Sun's position.
func NewZodiacalLight() *ZodiacalLight {
	return &ZodiacalLight{frame: frameCache{needSun: true}}
}

// ID implements [Component].
func (z *ZodiacalLight) ID() ComponentID { return Zodiacal }

// AddRadiance implements [Component].
func (z *ZodiacalLight) AddRadiance(
	_ context.Context,
	dst SpectralRadiance,
	grid unit.SpectralGrid,
	dir coord.AltAz,
	scene *Scene,
) (Flag, error) {
	if dir.Alt() <= 0 {
		return 0, nil
	}

	frame, err := z.frame.get(scene)
	if err != nil {
		return 0, err
	}

	icrs, err := frame.ctx.AltAzToICRS(dir)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: zodiacal: direction: %w", err)
	}

	ecliptic := coord.ICRSToEcliptic(icrs, astrotime.FromGo(scene.Time))

	geom := ZodiacalGeometry{
		DifferentialLongitude: ecliptic.Lon() - frame.sunEcliptic.Lon(),
		EclipticLatitude:      ecliptic.Lat(),
		SunDistanceAU:         frame.sunDistanceAU,
		// The Earth sits opposite the Sun as seen from the Sun, which is what
		// the seasonal term's phase is measured against.
		EarthLongitude: frame.sunEcliptic.Lon() + angle.Deg(180),
	}

	flags, err := ZodiacalRadiance(dst, grid, geom)
	if errors.Is(err, ErrZodiacalGeometry) {
		// Inside the solar vicinity the table does not reach. That is a real
		// gap in the model, not a failure of the evaluation, and a caller
		// looking that close to the Sun has bigger problems than zodiacal
		// light.
		return ExtrapolatedModel, nil
	}

	return flags, err
}

// Provenance implements [Component].
func (z *ZodiacalLight) Provenance() Provenance {
	return Provenance{
		Model:            "Leinert et al. (1998) tabulated zodiacal light",
		PrimaryReference: "Leinert, Ch. et al. (1998), A&AS 127, 1",
		SecondaryReferences: []string{
			"Levasseur-Regourd, A.C. & Dumont, R. (1980), A&A 84, 277",
			"Masana, E. et al. (2021), MNRAS 501, 5443, Eq. 15-18",
		},
		Equations: "Leinert Table 17 and Eq. 22, with the heliocentric and seasonal " +
			"factors of Masana et al. Eq. 18",
		ValidityDomain: "Beyond about 15 degrees solar elongation, which is where the " +
			"table begins; 220 nm to 2.5 microns for the colour correction",
		KnownApproximations: []string{
			"The colour correction is interpolated linearly in elongation between " +
				"the 30 and 90 degree relations Leinert et al. give.",
			"Small-scale structure such as cometary trails is not represented; the " +
				"table is explicit that it cannot be.",
			"The seasonal term applies a single sinusoid above 60 degrees of " +
				"ecliptic latitude rather than a cloud model.",
		},
		ExpectedAccuracy: "Leinert et al. describe the table as good to roughly 10 per " +
			"cent away from the solar vicinity.",
	}
}

// ── Airglow ─────────────────────────────────────────────────────────────────

// Airglow is the [Component] for chemiluminescent emission from the upper
// atmosphere, evaluated through [AirglowRadiance].
//
// The zenith spectrum is fixed at construction because it is a property of the
// night rather than of the direction: a caller with a measurement for their own
// night supplies it, and a caller without one supplies a reference. It is the
// most variable term in a dark sky and the least predictable, so this component
// never invents it.
//
// A value is safe for concurrent use and holds no per-scene state, since the
// van Rhijn factor depends only on the zenith angle.
type Airglow struct {
	zenith       SpectralRadiance
	grid         unit.SpectralGrid
	layerHeightM float64
	measured     bool
}

// NewAirglow builds the component from a zenith spectrum on a given grid.
//
// layerHeightM defaults to [github.com/TuSKan/astrogo/atmosphere.AirglowLayerHeightM]
// when zero. Set measured when the spectrum comes from an observation of the
// night being modelled rather than from a reference, which changes the quality
// flag the component reports.
func NewAirglow(zenith SpectralRadiance, grid unit.SpectralGrid, layerHeightM float64, measured bool) (*Airglow, error) {
	if len(zenith) != grid.Len() {
		return nil, fmt.Errorf("%w: %d values, grid has %d", ErrAirglowSpectrum, len(zenith), grid.Len())
	}

	return &Airglow{
		zenith:       append(SpectralRadiance(nil), zenith...),
		grid:         grid,
		layerHeightM: layerHeightM,
		measured:     measured,
	}, nil
}

// ID implements [Component].
func (a *Airglow) ID() ComponentID { return AirglowContinuum }

// AddRadiance implements [Component].
func (a *Airglow) AddRadiance(
	_ context.Context,
	dst SpectralRadiance,
	grid unit.SpectralGrid,
	dir coord.AltAz,
	_ *Scene,
) (Flag, error) {
	if !a.grid.Equal(grid) {
		return 0, fmt.Errorf("%w: component holds %v, asked for %v", ErrAirglowSpectrum, a.grid, grid)
	}

	flags, err := AirglowRadiance(dst, grid, a.zenith, angle.Deg(90)-dir.Alt(), a.layerHeightM)
	if err != nil {
		return 0, err
	}

	if a.measured {
		flags &^= ClimatologicalAirglow
		flags |= MeasuredAirglow
	}

	return flags, nil
}

// Provenance implements [Component].
func (a *Airglow) Provenance() Provenance {
	source := "a caller-supplied reference spectrum"
	if a.measured {
		source = "a measured zenith spectrum"
	}

	return Provenance{
		Model:            "van Rhijn geometry over " + source,
		PrimaryReference: "Leinert, Ch. et al. (1998), A&AS 127, 1, Eq. 13",
		SecondaryReferences: []string{
			"van Rhijn, P.J. (1921)",
			"Roach, F.E. & Meinel, A.B. (1955)",
			"Masana, E. et al. (2021), MNRAS 501, 5443, Eq. 19-20",
		},
		Equations:      "Leinert Eq. 13 as atmosphere.VanRhijn",
		ValidityDomain: "Above the horizon; the geometry alone is reliable within about 40 degrees of the zenith",
		KnownApproximations: []string{
			"A single thin emitting layer, where the real emissions arise between " +
				"about 90 km and 300 km depending on species.",
			"No extinction or scattering along the longer slant path, which work " +
				"against the geometric enhancement and matter beyond 40 degrees.",
			"The zenith spectrum is not predicted; airglow varies by up to 100 per " +
				"cent night to night and with the solar cycle.",
		},
		ExpectedAccuracy: "Dominated by the zenith spectrum's own provenance rather than " +
			"by the geometry.",
	}
}

// ── Integrated starlight ────────────────────────────────────────────────────

// ErrNoStarMap is returned when a starlight component is built without one.
var ErrNoStarMap = errors.New("skybrightness: integrated starlight needs a sky map")

// StarMap samples band-integrated extra-atmospheric radiance by direction.
//
// The values are outside the atmosphere: attenuating them is the component's
// job, not the map's, because the same map serves every site and airmass.
type StarMap interface {
	// RadianceAt returns W m^-2 sr^-1 in the map's own frame.
	RadianceAt(lon, lat angle.Angle) (float64, error)

	// Galactic reports whether the map is indexed in galactic coordinates.
	// An ICRS map returns false. Reading one as the other rotates the Milky
	// Way across the sky and still returns plausible numbers everywhere,
	// which is why the map carries the answer rather than the caller.
	Galactic() bool
}

// IntegratedStarlight is the [Component] for the summed light of resolved and
// unresolved stars, attenuated on its way to the observer.
//
// # Direct attenuation only
//
// Masana et al. (2021) Eq. 8 splits the observed radiance into a directly
// attenuated term and a scattered term, and this implements the first:
//
//	L_obs = L_0 * T(lambda, z)
//
// The scattered term returns to the line of sight some of what extinction
// removed, and for a source that fills the sky the two partly cancel. Omitting
// it therefore **overstates** the dimming toward the horizon — Masana et al.
// put the difference between their full and simplified scattering at under
// 0.1 mag arcsec^-2. Every result carries [ExtrapolatedModel] past 60 degrees
// from the zenith to say where that matters.
//
// A value is safe for concurrent use.
type IntegratedStarlight struct {
	sky   StarMap
	shape []float64

	frame frameCache
}

// NewIntegratedStarlight builds the component over a sky map and a spectral
// shape.
//
// A published starlight map is band-integrated, so spreading it across
// wavelengths needs an assumed spectrum: integrated starlight is the summed
// light of stars of every type, and no single blackbody is right. shape gives
// the relative spectral radiance per grid point, normalised however the
// caller likes — only its shape matters, since it is rescaled to reproduce
// the map's band value.
func NewIntegratedStarlight(sky StarMap, shape []float64) (*IntegratedStarlight, error) {
	if sky == nil {
		return nil, ErrNoStarMap
	}

	if len(shape) == 0 {
		return nil, fmt.Errorf("%w: no spectral shape", ErrNoStarMap)
	}

	var total float64

	for _, v := range shape {
		if v < 0 {
			return nil, fmt.Errorf("%w: negative spectral shape", ErrNoStarMap)
		}

		total += v
	}

	if total <= 0 {
		return nil, fmt.Errorf("%w: spectral shape sums to zero", ErrNoStarMap)
	}

	return &IntegratedStarlight{sky: sky, shape: append([]float64(nil), shape...)}, nil
}

// ID implements [Component].
func (s *IntegratedStarlight) ID() ComponentID { return Starlight }

// AddRadiance implements [Component].
func (s *IntegratedStarlight) AddRadiance(
	_ context.Context,
	dst SpectralRadiance,
	grid unit.SpectralGrid,
	dir coord.AltAz,
	scene *Scene,
) (Flag, error) {
	if dir.Alt() <= 0 {
		return 0, nil
	}

	if len(s.shape) != grid.Len() {
		return 0, fmt.Errorf("%w: shape has %d values, grid has %d",
			ErrNoStarMap, len(s.shape), grid.Len())
	}

	frame, err := s.frame.get(scene)
	if err != nil {
		return 0, err
	}

	icrs, err := frame.ctx.AltAzToICRS(dir)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: starlight: direction: %w", err)
	}

	lon, lat := icrs.RA(), icrs.Dec()
	if s.sky.Galactic() {
		g := coord.ICRSToGalactic(icrs)
		lon, lat = g.L(), g.B()
	}

	band, err := s.sky.RadianceAt(lon, lat)
	if err != nil || band <= 0 {
		return UnknownCloud, nil //nolint:nilerr // absence of map coverage is not a failure
	}

	airmass, err := atmosphere.Airmass(dir.Alt())
	if err != nil {
		return 0, fmt.Errorf("skybrightness: starlight: airmass: %w", err)
	}

	flags := Flag(0)
	if dir.Alt().Degrees() < 30 {
		flags |= ExtrapolatedModel
	}

	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()

	// The shape is rescaled so its band integral reproduces the map's value,
	// then each wavelength is attenuated by its own optical depth.
	var norm float64

	for i := range grid.Len() {
		norm += s.shape[i]
	}

	for i := range dst {
		lambda := grid.At(i)

		rayleigh, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return 0, fmt.Errorf("skybrightness: starlight: %w", err)
		}

		slant := (rayleigh + unit.OpticalDepth(aerosol.TauAt(lambda))) * unit.OpticalDepth(airmass)

		dst[i] += band * s.shape[i] / norm * float64(atmosphere.Transmission(slant))
	}

	return flags, nil
}

// Provenance implements [Component].
func (s *IntegratedStarlight) Provenance() Provenance {
	return Provenance{
		Model:            "tabulated extra-atmospheric starlight with direct attenuation",
		PrimaryReference: "Masana, E. et al. (2021), MNRAS 501, 5443, Eq. 8",
		SecondaryReferences: []string{
			"Gaia Collaboration (2021), A&A 649, A1 (EDR3)",
			"Leinert, Ch. et al. (1998), A&AS 127, 1, section 10",
		},
		Equations:      "L_obs = L_0 * exp(-tau * M(z))",
		ValidityDomain: "Above the horizon; the direct term alone is reliable within about 60 degrees of the zenith",
		KnownApproximations: []string{
			"Only the directly attenuated term of Masana et al. Eq. 8 is applied. " +
				"The scattered term returns some of the extinguished light to the " +
				"line of sight, so this overstates the dimming toward the horizon.",
			"The band value is spread across wavelengths by a caller-supplied " +
				"spectral shape, since integrated starlight has no single one.",
			"Whatever the supplied map omits — bright stars, faint completion, " +
				"colour imputation — is omitted here too.",
		},
		ExpectedAccuracy: "Dominated by the map's own provenance.",
	}
}
