package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
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
	at       time.GoTime

	// refraction is part of the cache key, because ctx was built with it.
	refraction atmosphere.Refraction

	ctx *coord.Context

	// sunEcliptic is the Sun's apparent ecliptic position; sunDistanceAU is
	// the observer's heliocentric distance.
	sunEcliptic   coord.Ecliptic
	sunDistanceAU float64
}

// matches reports whether this frame was built for the given scene.
func (f *sceneFrame) matches(scene *Scene) bool {
	if f == nil || !f.at.Equal(scene.Time) || f.observer != scene.Observer {
		return false
	}

	// The refraction belongs in the key because the cached transform context
	// was built with it: Context.AltAzToICRS feeds the pressure, temperature,
	// humidity and wavelength straight into Atoc13. Two scenes at the same
	// epoch and site with different surface conditions used to share one
	// context, and the second silently reused the first one's refraction —
	// worth tens of arcminutes near the horizon, which is more than an order-8
	// pixel, so the sky was read from the wrong place.
	//
	// The observer is compared by pointer because coord.NewGeodetic returns
	// one, which is also why the stale frame only appears when a caller reuses
	// a site across scenes. That is the ordinary way to model one observatory
	// under changing weather.
	return sameRefraction(f.refraction, scene.Atmosphere.Refraction())
}

// sameRefraction reports whether two refraction settings would build the same
// transform context.
//
// The model is compared by type rather than by value. Every model this package
// ships is a stateless empty struct, so type identity is the right equivalence,
// and comparing the interface directly would panic for a caller whose own model
// happens to be a non-comparable type.
func sameRefraction(a, b atmosphere.Refraction) bool {
	if a.Pressure != b.Pressure || a.Temperature != b.Temperature ||
		a.Humidity != b.Humidity || a.Wavelength != b.Wavelength {
		return false
	}

	if a.Model == nil || b.Model == nil {
		return a.Model == nil && b.Model == nil
	}

	return reflect.TypeOf(a.Model) == reflect.TypeOf(b.Model)
}

// newSceneFrame resolves a scene's transform context and Sun position.
//
// needSun is false for components that do not use it, so a diffuse-galactic
// or airglow evaluation does not require an ephemeris in the scene at all.
func newSceneFrame(scene *Scene, needSun bool) (*sceneFrame, error) {
	if err := scene.Validate(); err != nil {
		return nil, err
	}

	at := time.FromGo(scene.Time)

	frame := &sceneFrame{
		observer:   scene.Observer,
		at:         scene.Time,
		refraction: scene.Atmosphere.Refraction(),
		ctx:        coord.NewContext(at, scene.Observer, scene.Atmosphere.Refraction()),
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

	// sky, band and scratch support the Toller cap. They are nil when the
	// caller supplied no star map, in which case the cap is not applied and
	// every result says so.
	sky     StarMap
	band    magnitude.Passband
	scratch sync.Pool
}

// NewDiffuseGalacticLight builds the component over a 100 micron dust map.
//
// sky and band are optional and enable the Toller cap: dust scatters
// starlight, so the diffuse galactic light cannot exceed
// [MaxDGLToStarlightRatio] of the integrated starlight along the same
// sightline. Pass a nil map to skip it, and every result then carries
// [ExtrapolatedModel] to say the correlation is running unbounded.
//
// The band is the passband the star map's values are averaged over, because
// the cap compares two radiances and they have to be compared over the same
// support. It is the same requirement, for the same reason, as
// [NewIntegratedStarlight]'s.
func NewDiffuseGalacticLight(
	dust DustMap,
	sky StarMap,
	band magnitude.Passband,
) (*DiffuseGalacticLight, error) {
	if dust == nil {
		return nil, ErrNoDustMap
	}

	d := &DiffuseGalacticLight{dust: dust}

	if sky != nil {
		if err := band.Validate(); err != nil {
			return nil, fmt.Errorf("%w: capping needs a usable passband: %w", ErrNoDustMap, err)
		}

		d.sky, d.band = sky, band
	}

	return d, nil
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

	if len(dst) != grid.Len() {
		return 0, fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
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
		return UnknownCloud, nil
	}

	// The spectrum is always built aside, because both the cap and the
	// atmosphere scale it before it reaches dst.
	scratch, ok := d.scratch.Get().(SpectralRadiance)
	if !ok || len(scratch) != grid.Len() {
		scratch = NewSpectralRadiance(grid)
	}

	clear(scratch)
	defer d.scratch.Put(scratch) //nolint:staticcheck // a slice header, deliberately pooled

	flags, err := DiffuseGalacticRadiance(scratch, grid, intensity)
	if err != nil {
		return 0, err
	}

	scale := 1.0

	if d.sky == nil {
		// Uncapped: the correlation is free to predict more scattered light
		// than there is starlight to scatter, which is what the flag says.
		flags |= ExtrapolatedModel
	} else {
		capScale, capFlags := d.capFactor(scratch, grid, icrs, galactic)
		scale, flags = capScale, flags|capFlags
	}

	// Diffuse galactic light reaches the top of the atmosphere and then has to
	// cross it, exactly as integrated starlight does. An earlier revision
	// added it to dst unattenuated, which made it too bright by 1/T - about
	// 17 per cent at the zenith at sea level and more toward the horizon - and
	// showed up as a DGL-to-starlight ratio of 0.41 against a cap of 0.35,
	// since the cap is applied above the atmosphere and only starlight was
	// then dimmed by it.
	airmass, err := atmosphere.Airmass(dir.Alt())
	if err != nil {
		return 0, fmt.Errorf("skybrightness: diffuse galactic: airmass: %w", err)
	}

	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()
	height := scene.Observer.Height()
	kappa := scene.Atmosphere.DiffuseKappa()

	for i := range dst {
		lambda := grid.At(i)

		rayleigh, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return 0, fmt.Errorf("skybrightness: diffuse galactic: %w", err)
		}

		// Masana et al. (2021) Eq. 29: the molecular and aerosol columns carry
		// their own scale heights, and kappa accounts for the light scattered
		// back into the line of sight from the rest of the sky, which a source
		// filling that sky supplies and a point source does not.
		slant, slantErr := atmosphere.ExtendedSourceOpticalDepth(
			rayleigh, unit.OpticalDepth(aerosol.TauAt(lambda)),
			airmass, airmass, height, kappa)
		if slantErr != nil {
			return 0, fmt.Errorf("skybrightness: %s: %w", "diffuse galactic", slantErr)
		}

		dst[i] += scratch[i] * scale * float64(atmosphere.Transmission(slant))
	}

	return flags, nil
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
			"Only the directly attenuated term is applied; light scattered out of " +
				"the beam is not returned to it, matching IntegratedStarlight.",
		},
		ExpectedAccuracy: "The correlation's own scatter is large; Kawara et al. quote " +
			"uncertainties of 10 to 50 per cent on the slope depending on band.",
	}
}

// capFactor returns the factor bringing the diffuse galactic light within
// [MaxDGLToStarlightRatio] of the starlight along the same sightline, and 1
// when it is already inside it.
//
// The two radiances are compared over the star map's own passband, because a
// ratio between quantities averaged over different supports is not a ratio of
// anything. A sightline the star map does not cover cannot be capped, and says
// so rather than being capped against zero — which would erase the DGL
// entirely on exactly the sightlines where a map is most likely to be missing.
func (d *DiffuseGalacticLight) capFactor(
	dgl SpectralRadiance,
	grid unit.SpectralGrid,
	icrs coord.ICRS,
	galactic coord.Galactic,
) (scale float64, flags Flag) {
	lon, lat := icrs.RA(), icrs.Dec()
	if d.sky.Galactic() {
		lon, lat = galactic.L(), galactic.B()
	}

	starlight, err := d.sky.RadianceAt(lon, lat)
	if err != nil || starlight <= 0 {
		return 1, UnknownCloud
	}

	mean, err := magnitude.MeanFluxDensity(dgl, grid, d.band, 0)
	if err != nil || mean <= 0 {
		return 1, 0
	}

	limit := MaxDGLToStarlightRatio * starlight
	if mean <= limit {
		return 1, 0
	}

	// Hitting the cap means the correlation was extrapolated past where it
	// describes anything, so the result is bounded rather than trusted.
	return limit / mean, ExtrapolatedModel
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

	// scratch holds the unattenuated spectrum while the atmosphere is
	// applied to it, the same reason DiffuseGalacticLight carries one.
	scratch sync.Pool
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

	if len(dst) != grid.Len() {
		return 0, fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	frame, err := z.frame.get(scene)
	if err != nil {
		return 0, err
	}

	icrs, err := frame.ctx.AltAzToICRS(dir)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: zodiacal: direction: %w", err)
	}

	ecliptic := coord.ICRSToEcliptic(icrs, time.FromGo(scene.Time))

	geom := ZodiacalGeometry{
		DifferentialLongitude: ecliptic.Lon() - frame.sunEcliptic.Lon(),
		EclipticLatitude:      ecliptic.Lat(),
		SunDistanceAU:         frame.sunDistanceAU,
		// The Earth sits opposite the Sun as seen from the Sun, which is what
		// the seasonal term's phase is measured against.
		EarthLongitude: frame.sunEcliptic.Lon() + angle.Deg(180),
	}

	scratch, ok := z.scratch.Get().(SpectralRadiance)
	if !ok || len(scratch) != grid.Len() {
		scratch = NewSpectralRadiance(grid)
	}

	clear(scratch)
	defer z.scratch.Put(scratch) //nolint:staticcheck // a slice header, deliberately pooled

	flags, err := ZodiacalRadiance(scratch, grid, geom)
	if errors.Is(err, ErrZodiacalGeometry) {
		// Inside the solar vicinity the table does not reach. That is a real
		// gap in the model, not a failure of the evaluation, and a caller
		// looking that close to the Sun has bigger problems than zodiacal
		// light.
		return ExtrapolatedModel, nil
	}

	if err != nil {
		return flags, err
	}

	// Zodiacal light is sunlight scattered by interplanetary dust, so it
	// reaches the top of the atmosphere and then has to cross it, exactly as
	// starlight and the extragalactic background do. An earlier revision added
	// it unattenuated, which made it too bright by 1/T and mattered more than
	// the same omission in diffuse galactic light because this term is three
	// times the size.
	airmass, err := atmosphere.Airmass(dir.Alt())
	if err != nil {
		return 0, fmt.Errorf("skybrightness: zodiacal: airmass: %w", err)
	}

	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()
	height := scene.Observer.Height()
	kappa := scene.Atmosphere.DiffuseKappa()

	for i := range dst {
		lambda := grid.At(i)

		rayleigh, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return 0, fmt.Errorf("skybrightness: zodiacal: %w", err)
		}

		// Masana et al. (2021) Eq. 29: the molecular and aerosol columns carry
		// their own scale heights, and kappa accounts for the light scattered
		// back into the line of sight from the rest of the sky, which a source
		// filling that sky supplies and a point source does not.
		slant, slantErr := atmosphere.ExtendedSourceOpticalDepth(
			rayleigh, unit.OpticalDepth(aerosol.TauAt(lambda)),
			airmass, airmass, height, kappa)
		if slantErr != nil {
			return 0, fmt.Errorf("skybrightness: %s: %w", "zodiacal", slantErr)
		}

		dst[i] += scratch[i] * float64(atmosphere.Transmission(slant))
	}

	return flags, nil
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
			"Only the directly attenuated term is applied; light scattered out of " +
				"the beam is not returned to it, matching IntegratedStarlight.",
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

	// scratch holds the unattenuated spectrum while the atmosphere is
	// applied to it, the same reason ZodiacalLight carries one.
	scratch sync.Pool
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
	scene *Scene,
) (Flag, error) {
	if !a.grid.Equal(grid) {
		return 0, fmt.Errorf("%w: component holds %v, asked for %v", ErrAirglowSpectrum, a.grid, grid)
	}

	// AirglowRadiance checks this for whatever slice it is handed, and it used
	// to be handed dst. Now that it fills a scratch buffer instead, dst is
	// only ever indexed here and has to be checked here.
	if len(dst) != grid.Len() {
		return 0, fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	scratch, ok := a.scratch.Get().(SpectralRadiance)
	if !ok || len(scratch) != grid.Len() {
		scratch = NewSpectralRadiance(grid)
	}

	clear(scratch)
	defer a.scratch.Put(scratch) //nolint:staticcheck // a slice header, deliberately pooled

	flags, err := AirglowRadiance(scratch, grid, a.zenith, angle.Deg(90)-dir.Alt(), a.layerHeightM)
	if err != nil {
		return 0, err
	}

	if a.measured {
		flags &^= ClimatologicalAirglow
		flags |= MeasuredAirglow
	}

	// Airglow is emitted at about 87 km, which is above essentially the whole
	// atmosphere, so what reaches the observer has crossed nearly the same
	// column that starlight crosses and is extinguished by nearly the same
	// amount. van Rhijn lengthens the path through the emitting layer and
	// brightens the limb; extinction lengthens the path through everything
	// below it and darkens the limb. Applying only the first is what made this
	// component too bright toward the horizon.
	//
	// The airmass is the ordinary atmospheric one for the line of sight, not
	// the van Rhijn factor: the two describe different paths through different
	// layers and are not interchangeable.
	//
	// Measured against GAMBONS before this was applied, our airglow ran 1.55
	// times theirs in the 0-15 degree altitude band while agreeing within the
	// stated validity near the zenith - a slope error of very nearly the
	// extinction that was missing.
	// Below the horizon AirglowRadiance has already returned zero, so there is
	// nothing to attenuate and no airmass to attenuate it over. Asked
	// explicitly rather than inferred from Airmass refusing, so a future
	// failure there is reported instead of read as a direction off the sky.
	if dir.Alt().Degrees() <= 0 {
		return flags, nil
	}

	airmass, err := atmosphere.Airmass(dir.Alt())
	if err != nil {
		return 0, fmt.Errorf("skybrightness: airglow: airmass: %w", err)
	}

	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()
	height := scene.Observer.Height()
	kappa := scene.Atmosphere.DiffuseKappa()

	for i := range dst {
		lambda := grid.At(i)

		rayleigh, rErr := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if rErr != nil {
			return 0, fmt.Errorf("skybrightness: airglow: %w", rErr)
		}

		// Masana et al. (2021) Eq. 29: the molecular and aerosol columns carry
		// their own scale heights, and kappa accounts for the light scattered
		// back into the line of sight from the rest of the sky, which a source
		// filling that sky supplies and a point source does not.
		slant, slantErr := atmosphere.ExtendedSourceOpticalDepth(
			rayleigh, unit.OpticalDepth(aerosol.TauAt(lambda)),
			airmass, airmass, height, kappa)
		if slantErr != nil {
			return 0, fmt.Errorf("skybrightness: %s: %w", "airglow", slantErr)
		}

		dst[i] += scratch[i] * float64(atmosphere.Transmission(slant))
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
		Equations: "Leinert Eq. 13 as atmosphere.VanRhijn",
		ValidityDomain: "Above the horizon. The geometry alone is reliable within about 40 " +
			"degrees of the zenith, and extinction is applied beyond it",
		KnownApproximations: []string{
			"A single thin emitting layer, where the real emissions arise between " +
				"about 90 km and 300 km depending on species.",
			"Extinction along the slant path is applied, but not the light " +
				"scattered back into it, so the horizon is still somewhat too faint " +
				"rather than, as before, much too bright.",
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

// StarMap samples extra-atmospheric starlight radiance by direction.
//
// The values are outside the atmosphere: attenuating them is the component's
// job, not the map's, because the same map serves every site and airmass.
type StarMap interface {
	// RadianceAt returns the passband-averaged spectral radiance in the
	// map's own frame, W m^-2 sr^-1 nm^-1 — the response-weighted mean the
	// magnitude systems are defined against, which is what a zero point
	// converts a summed catalogue flux into. The passband it averages over
	// is the one handed to [NewIntegratedStarlight]; a map built for one
	// band and read against another is a silent error this interface
	// cannot catch.
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
	shape []float64 // rescaled so its own passband average is exactly one
	grid  unit.SpectralGrid
	band  magnitude.Passband

	frame frameCache
}

// starlightBandCoverage is the fraction of the passband the spectral grid
// must span before the shape's average over it means anything. A grid
// covering half the band averages the shape over the wrong support and
// rescales it by the wrong factor, which is a quiet error rather than a
// loud one, so the bar is set just short of complete.
const starlightBandCoverage = 0.99

// NewIntegratedStarlight builds the component over a sky map and a spectral
// shape.
//
// A starlight map holds one number per direction, so spreading it across
// wavelengths needs an assumed spectrum: integrated starlight is the summed
// light of stars of every type, and no single blackbody is right. shape gives
// the relative spectral radiance per grid point, normalised however the
// caller likes — only its shape matters.
//
// band is the passband the map's values are averaged over, and it is what
// makes the rescaling exact. The shape is divided by its own passband
// average, so the component adds a spectrum whose average over that same band
// reproduces the map's number by construction. Normalising by the sum of the
// samples instead would tie the answer to how finely the grid is sampled,
// halving the starlight whenever the grid is refined.
func NewIntegratedStarlight(
	sky StarMap,
	shape SpectralRadiance,
	grid unit.SpectralGrid,
	band magnitude.Passband,
) (*IntegratedStarlight, error) {
	if sky == nil {
		return nil, ErrNoStarMap
	}

	if len(shape) != grid.Len() {
		return nil, fmt.Errorf("%w: shape has %d values, grid has %d",
			ErrNoStarMap, len(shape), grid.Len())
	}

	for _, v := range shape {
		if v < 0 {
			return nil, fmt.Errorf("%w: negative spectral shape", ErrNoStarMap)
		}
	}

	mean, err := magnitude.MeanFluxDensity(shape, grid, band, starlightBandCoverage)
	if err != nil {
		return nil, fmt.Errorf("skybrightness: integrated starlight: %w", err)
	}

	if mean <= 0 {
		return nil, fmt.Errorf("%w: spectral shape averages to zero across %q",
			ErrNoStarMap, band.Name)
	}

	norm := make([]float64, len(shape))
	for i, v := range shape {
		norm[i] = v / mean
	}

	return &IntegratedStarlight{sky: sky, shape: norm, grid: grid, band: band}, nil
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

	if len(dst) != grid.Len() {
		return 0, fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	if !s.grid.Equal(grid) {
		return 0, fmt.Errorf("%w: component holds %v, asked for %v",
			ErrNoStarMap, s.grid, grid)
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

	value, err := s.sky.RadianceAt(lon, lat)
	if err != nil || value <= 0 {
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
	height := scene.Observer.Height()
	kappa := scene.Atmosphere.DiffuseKappa()

	// The shape already averages to one across the band, so scaling it by
	// the map's value reproduces that value exactly. Each wavelength is then
	// attenuated by its own optical depth, which is what makes the result
	// redder than the map: extinction is steepest at the blue end.
	for i := range dst {
		lambda := grid.At(i)

		rayleigh, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return 0, fmt.Errorf("skybrightness: starlight: %w", err)
		}

		// Masana et al. (2021) Eq. 29: the molecular and aerosol columns carry
		// their own scale heights, and kappa accounts for the light scattered
		// back into the line of sight from the rest of the sky, which a source
		// filling that sky supplies and a point source does not.
		slant, slantErr := atmosphere.ExtendedSourceOpticalDepth(
			rayleigh, unit.OpticalDepth(aerosol.TauAt(lambda)),
			airmass, airmass, height, kappa)
		if slantErr != nil {
			return 0, fmt.Errorf("skybrightness: %s: %w", "starlight", slantErr)
		}

		dst[i] += value * s.shape[i] * float64(atmosphere.Transmission(slant))
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
