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
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	astrotime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// Sentinel errors for the moonlight component.
var (
	// ErrNoSolarSpectrum is returned when a ScatteredMoonlight is built
	// without a solar spectrum. There is no default: the absolute scale of
	// the ROLO model depends on which solar irradiance reference it is
	// paired with, so choosing one silently would hide a decision that
	// belongs to the caller.
	ErrNoSolarSpectrum = errors.New("skybrightness: moonlight needs a solar spectrum on the ROLO bands")

	// ErrNoEphemeris is returned when a scene has no ephemeris provider.
	ErrNoEphemeris = errors.New("skybrightness: moonlight needs an ephemeris provider in the scene")
)

// ScatteredMoonlight is the moonlight [Component]: sunlight reflected by the
// Moon and scattered once by the atmosphere into the line of sight.
//
//   - Lunar reflectance: ROLO model 311g, Kieffer & Stone (2005) Eq. 10,
//     via [magnitude.ROLOReflectance].
//   - Atmospheric scattering: molecular and aerosol single scattering,
//     using atmosphere's Rayleigh optical depth and combined phase
//     function, both after Winkler (2022) and Bucholtz (1995).
//   - Radiative transfer: [atmosphere.SingleScatteredRadiance], corrected for
//     higher scattering orders by [atmosphere.MultipleScatteringFactor].
//
// The name says scattered because that is the whole quantity: direct
// moonlight is not sky brightness, and what makes the sky bright near a
// bright Moon is the fraction of that light redirected into the observer's
// line of sight by air molecules and aerosols.
//
// This is deliberately NOT Krisciunas & Schaefer (1991). That model is a
// V-band fit and cannot answer a spectral question; reproducing a correct V
// magnitude from a wrong spectrum is exactly the failure mode this module
// exists to avoid.
//
// A value is safe for concurrent use. It caches the per-scene geometry —
// which does not depend on the viewing direction — behind a read-write
// lock, so a full-sky evaluation computes the Moon's position once rather
// than once per direction.
type ScatteredMoonlight struct {
	solar []float64

	mu     sync.RWMutex
	cached *moonGeometry

	// scratch holds the two working buffers a single direction needs: the
	// per-band scattered radiance and its resampling onto the caller's
	// grid. The Component contract forbids allocating per call, and a
	// full-sky evaluation runs this on the order of 10^4 times.
	scratch sync.Pool
}

// moonScratch is one direction's working space.
type moonScratch struct {
	bands []float64
	grid  []float64
}

// moonGeometry is everything about a scene that does not vary with the
// viewing direction.
type moonGeometry struct {
	observer *coord.Geodetic
	at       time.Time

	direction  coord.AltAz
	airmass    float64
	phaseAngle angle.Angle

	// irradiance is the lunar spectral irradiance at the observer on the
	// 32 ROLO bands, W m^-2 nm^-1.
	irradiance []float64

	// extrapolated records that the phase angle fell outside ROLO's
	// fitted range.
	extrapolated bool

	// aboveHorizon is false when the Moon contributes nothing.
	aboveHorizon bool
}

// NewScatteredMoonlight builds the moonlight component from the solar
// spectral irradiance at 1 AU, sampled at [magnitude.ROLOBands] and given in
// W m^-2 nm^-1.
//
// The solar spectrum is required rather than defaulted. This package ships
// no solar irradiance reference, and the choice between them — Kurucz,
// Thuillier, TSIS-1 HSRS, whichever the caller's calibration chain already
// uses — moves the absolute lunar irradiance by more than the ROLO model's
// own accuracy at some wavelengths. It travels into the provenance record.
func NewScatteredMoonlight(solarSpectrum []float64) (*ScatteredMoonlight, error) {
	bands := magnitude.ROLOBands()

	if len(solarSpectrum) != len(bands) {
		return nil, fmt.Errorf("%w: got %d values, want %d",
			ErrNoSolarSpectrum, len(solarSpectrum), len(bands))
	}

	for i, v := range solarSpectrum {
		if v < 0 || math.IsNaN(v) {
			return nil, fmt.Errorf("%w: band %d (%v nm) is %g",
				ErrNoSolarSpectrum, i, bands[i], v)
		}
	}

	m := &ScatteredMoonlight{solar: append([]float64(nil), solarSpectrum...)}
	m.scratch.New = func() any { return &moonScratch{bands: make([]float64, len(bands))} }

	return m, nil
}

// ID implements [Component].
func (m *ScatteredMoonlight) ID() ComponentID { return Moonlight }

// AddRadiance implements [Component].
func (m *ScatteredMoonlight) AddRadiance(
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

	geom, err := m.geometry(scene)
	if err != nil {
		return 0, err
	}

	flags := ApproximateMultipleScattering
	if geom.extrapolated {
		flags |= ExtrapolatedModel
	}

	// Below the horizon the Moon illuminates nothing the observer can see,
	// and above the horizon a line of sight below it is looking at ground.
	if !geom.aboveHorizon || dir.Alt() <= 0 {
		return flags, nil
	}

	viewAirmass, err := atmosphere.Airmass(dir.Alt())
	if err != nil {
		return 0, fmt.Errorf("skybrightness: moonlight: view airmass: %w", err)
	}

	// The scattering angle is the angle between the direction the light was
	// travelling and the direction it travels after scattering, which for a
	// source at geom.direction seen along dir is the separation between the
	// two lines of sight.
	scatter := angle.Acos(clamp(geom.direction.ToUnitVector().Dot(dir.ToUnitVector()), -1, 1))

	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()

	bands := magnitude.ROLOBands()

	scratch, _ := m.scratch.Get().(*moonScratch)
	defer m.scratch.Put(scratch)

	if cap(scratch.grid) < grid.Len() {
		scratch.grid = make([]float64, grid.Len())
	}

	scattered, resampled := scratch.bands, scratch.grid[:grid.Len()]

	clear(scattered)

	for i, lambda := range bands {
		if geom.irradiance[i] <= 0 {
			continue
		}

		rayleigh, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return 0, fmt.Errorf("skybrightness: moonlight: %w", err)
		}

		aer := unit.OpticalDepth(aerosol.TauAt(lambda))

		phase, err := atmosphere.CombinedPhaseFunction(scatter.Radians(),
			rayleigh, aer, float64(aerosol.Asymmetry), atmosphere.RayleighDepolarisation)
		if err != nil {
			return 0, fmt.Errorf("skybrightness: moonlight: %w", err)
		}

		// Aerosol removes light by absorption as well as scattering; the
		// single-scattering albedo is the share that scatters. Molecular
		// extinction at optical wavelengths is scattering outright.
		extinction := rayleigh + aer
		scattering := rayleigh + aer*unit.OpticalDepth(aerosol.SingleScatteringAlbedo)

		l, err := atmosphere.SingleScatteredRadiance(geom.irradiance[i], phase,
			scattering, extinction, geom.airmass, viewAirmass)
		if err != nil {
			return 0, fmt.Errorf("skybrightness: moonlight: %w", err)
		}

		// Photons scattered twice or more still reach the observer, and
		// single scattering misses them. Winkler (2022) quantifies the
		// shortfall against SAAO measurements as proportional to the
		// molecular optical depth.
		multiple, err := atmosphere.MultipleScatteringFactor(rayleigh)
		if err != nil {
			return 0, fmt.Errorf("skybrightness: moonlight: %w", err)
		}

		scattered[i] = l * multiple
	}

	// One resample, from the model's own 32 bands onto the caller's grid.
	// Doing it here rather than per band keeps the physics on the grid the
	// coefficients were fitted for.
	if err := grid.Resample(resampled, bands, scattered, 0); err != nil {
		return 0, fmt.Errorf("skybrightness: moonlight: resample: %w", err)
	}

	for i := range dst {
		dst[i] += resampled[i]
	}

	return flags, nil
}

// Provenance implements [Component].
func (m *ScatteredMoonlight) Provenance() Provenance {
	return Provenance{
		Model:            "ROLO lunar reflectance with molecular and aerosol single scattering",
		Version:          "311g",
		PrimaryReference: "Kieffer, H.H. & Stone, T.C. (2005), AJ 129, 2887, doi:10.1086/430185",
		SecondaryReferences: []string{
			"Winkler, H. (2022), MNRAS 514, 208, doi:10.1093/mnras/stac1387",
			"Bucholtz, A. (1995), Appl. Opt. 34, 2765",
			"Henyey, L.G. & Greenstein, J.L. (1941), ApJ 93, 70",
			"Pickering, K.A. (2002), DIO 12, 20 (airmass)",
		},
		Equations: "Kieffer & Stone Eq. 10; Winkler Eq. 9, 10, 12, 13; " +
			"single-scattering transfer derived in atmosphere.SingleScatteredRadiance",
		ValidityDomain: "Lunar phase angle 1.55 to 97 degrees; ROLO's 32 bands span " +
			"350 to 2383.6 nm and the model is undefined outside them. Optical " +
			"wavelengths, clear sky, Moon above the horizon.",
		KnownApproximations: []string{
			"Multiple scattering enters as Winkler's broadband empirical factor " +
				"1 + 4.5*tau_R, fitted at one site under low aerosol loading, not as " +
				"a radiative-transfer solution.",
			"Selenographic libration angles are not supplied, costing at most " +
				"0.03 in ln A by Kieffer & Stone Table 5's own accounting.",
			"The selenographic longitude of the Sun is taken as the signed phase " +
				"angle, exact to the Sun's excursion from the lunar equator.",
			"The Moon-observer distance is geocentric, not topocentric: up to " +
				"3.3% in irradiance.",
			"ROLO is defined at 32 discrete bands; values between them are a " +
				"linear interpolation and are not reliable across the telluric " +
				"water bands near 943 and 1400 nm.",
			"A homogeneous, plane-parallel atmosphere with airmass in place of " +
				"sec(z); no cloud, no ground reflection.",
		},
		ExpectedAccuracy: "ROLO's own absolute scale is 5-10%, and the solar spectrum " +
			"the caller supplies carries its own. The single-scattering omission " +
			"dominates both near a bright Moon.",
	}
}

// geometry returns the direction-independent part of the calculation,
// recomputing it only when the scene changes.
func (m *ScatteredMoonlight) geometry(scene *Scene) (*moonGeometry, error) {
	m.mu.RLock()
	cached := m.cached
	m.mu.RUnlock()

	if cached.matches(scene) {
		return cached, nil
	}

	fresh, err := m.computeGeometry(scene)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.cached = fresh
	m.mu.Unlock()

	return fresh, nil
}

// matches reports whether this geometry was computed for the given scene.
func (g *moonGeometry) matches(scene *Scene) bool {
	return g != nil && g.at.Equal(scene.Time) && g.observer == scene.Observer
}

// computeGeometry resolves the Moon's position, phase and irradiance.
func (m *ScatteredMoonlight) computeGeometry(scene *Scene) (*moonGeometry, error) {
	if scene.Ephemeris == nil {
		return nil, ErrNoEphemeris
	}

	at := astrotime.FromGo(scene.Time)

	moon, err := scene.Ephemeris.State(eph.Moon, at)
	if err != nil {
		return nil, fmt.Errorf("skybrightness: moonlight: moon state: %w", err)
	}

	sun, err := scene.Ephemeris.State(eph.Sun, at)
	if err != nil {
		return nil, fmt.Errorf("skybrightness: moonlight: sun state: %w", err)
	}

	moonICRS, err := eph.ToICRS(moon.Pos)
	if err != nil {
		return nil, fmt.Errorf("skybrightness: moonlight: moon direction: %w", err)
	}

	altaz, err := coord.NewContext(at, scene.Observer, scene.Atmosphere.Refraction()).ICRSToAltAz(moonICRS)
	if err != nil {
		return nil, fmt.Errorf("skybrightness: moonlight: moon altaz: %w", err)
	}

	geom := &moonGeometry{
		observer:     scene.Observer,
		at:           scene.Time,
		direction:    altaz,
		aboveHorizon: altaz.Alt() > 0,
		irradiance:   make([]float64, len(magnitude.ROLOBands())),
	}

	if !geom.aboveHorizon {
		return geom, nil
	}

	geom.airmass, err = atmosphere.Airmass(altaz.Alt())
	if err != nil {
		return nil, fmt.Errorf("skybrightness: moonlight: moon airmass: %w", err)
	}

	geom.phaseAngle = phaseAngleAt(moon.Pos, sun.Pos)

	reflectance := make([]float64, len(geom.irradiance))

	err = magnitude.ROLOReflectance(reflectance, magnitude.ROLOGeometry{
		PhaseAngle:     geom.phaseAngle,
		SolarLongitude: solarSelenographicLongitude(moon.Pos, sun.Pos, geom.phaseAngle),
	})
	if err != nil && !errors.Is(err, magnitude.ErrROLOPhaseRange) {
		return nil, fmt.Errorf("skybrightness: moonlight: %w", err)
	}

	geom.extrapolated = err != nil

	// Distances: the Sun-Moon leg in AU, the Moon-observer leg in km. The
	// latter is geocentric rather than topocentric, which is at most an
	// Earth radius out — up to 3.3% in irradiance, inside ROLO's own stated
	// absolute accuracy but recorded in the provenance all the same.
	kmPerAU := constants.IAU.AstronomicalUnit.Value / 1e3

	if err := magnitude.ROLOIrradiance(geom.irradiance, reflectance, m.solar,
		moon.Pos.Sub(sun.Pos).Norm(), moon.Pos.Norm()*kmPerAU); err != nil {
		return nil, fmt.Errorf("skybrightness: moonlight: %w", err)
	}

	return geom, nil
}

// phaseAngleAt returns the Sun-Moon-observer angle from geocentric position
// vectors: the angle at the Moon between the directions to the Sun and to
// the observer.
func phaseAngleAt(moon, sun vector.Vec3) angle.Angle {
	toObserver := moon.MulScalar(-1)
	toSun := sun.Sub(moon)

	denom := toObserver.Norm() * toSun.Norm()
	if denom == 0 {
		return 0
	}

	return angle.Acos(clamp(toObserver.Dot(toSun)/denom, -1, 1))
}

// solarSelenographicLongitude returns the selenographic longitude of the
// Sun, signed negative before full Moon after Kieffer & Stone (2005).
//
// This is a geometric identity rather than an approximation of one. The
// Moon's prime meridian is defined by the mean sub-Earth point, so the
// selenographic longitude of the Sun is the angle at the Moon between the
// directions to the Earth and to the Sun — which is the phase angle. The
// only departures are the Sun's small excursion from the lunar equator,
// under 1.5 degrees, and libration in longitude, which Eq. 10 already
// carries as its own separate c2 and c4 terms.
//
// The sign says which limb is lit: negative while waxing, when the Moon
// leads the Sun. That is read from the sense of the Sun-to-Moon rotation
// about the celestial pole, which agrees with the ecliptic pole except
// where the two bodies are nearly aligned — near full Moon, where the
// longitude is near zero and the sign does not matter, and near new Moon,
// which is far outside ROLO's validity range.
func solarSelenographicLongitude(moon, sun vector.Vec3, phase angle.Angle) angle.Angle {
	if sun.Cross(moon).Z > 0 {
		return -phase // Moon east of the Sun: waxing
	}

	return phase
}

// clamp confines v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
