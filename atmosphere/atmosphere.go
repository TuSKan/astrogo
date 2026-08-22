package atmosphere

import (
	"errors"
	"math"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/unit"
)

// ErrAtmosphereBuilder is returned by Builder.Build when the accumulated
// inputs are invalid (out-of-range pressure/temperature/AOD/SSA/etc.).
var ErrAtmosphereBuilder = errors.New("atmosphere: invalid Atmosphere")

// Sentinel components Builder.Build's aggregated error may join, wrapped
// via errors.Join so a caller can errors.Is against any specific
// violation.
var (
	errSurfacePressure    = errors.New("Surface: pressure must be > 0 hPa")
	errSurfaceTemperature = errors.New("Surface: temperature must be > 0 K")
	errAerosolAOD         = errors.New("Aerosol: AOD must be >= 0")
	errAerosolSSA         = errors.New("Aerosol: single-scattering albedo must be in [0,1]")
	errAerosolAsymmetry   = errors.New("Aerosol: asymmetry parameter must be in [-1,1]")
	errCloudFraction      = errors.New("AddCloud: Fraction must be in [0,1]")
	errCloudOpticalDepth  = errors.New("AddCloud: OpticalDepth must be >= 0")
)

// CloudPhase distinguishes the thermodynamic phase of a cloud layer.
type CloudPhase uint8

// The three cloud phases.
const (
	CloudLiquid CloudPhase = iota
	CloudIce
	CloudMixed
)

// String implements fmt.Stringer.
func (p CloudPhase) String() string {
	switch p {
	case CloudLiquid:
		return "Liquid"
	case CloudIce:
		return "Ice"
	case CloudMixed:
		return "Mixed"
	default:
		return "CloudPhase(unknown)"
	}
}

// CloudMorphology is a coarse structural descriptor of a cloud layer.
type CloudMorphology uint8

// The four cloud morphologies.
const (
	MorphologyUnknown CloudMorphology = iota
	MorphologyStratiform
	MorphologyCumuliform
	MorphologyCirriform
)

// String implements fmt.Stringer.
func (m CloudMorphology) String() string {
	switch m {
	case MorphologyUnknown:
		return "Unknown"
	case MorphologyStratiform:
		return "Stratiform"
	case MorphologyCumuliform:
		return "Cumuliform"
	case MorphologyCirriform:
		return "Cirriform"
	default:
		return "Unknown"
	}
}

// CloudUncertainty carries a cloud layer's own uncertainty on its
// fraction and optical depth.
type CloudUncertainty struct {
	FractionRelSigma     float64
	OpticalDepthRelSigma float64
}

// CloudLayer describes one cloud layer. Fraction (sky cover) and
// OpticalDepth are deliberately distinct fields of distinct types — sky
// cover and opacity are never collapsed to one scalar, enforced by the
// compiler.
type CloudLayer struct {
	Fraction        unit.CloudFraction
	BaseAlt, TopAlt unit.AltitudeM
	OpticalDepth    unit.CloudOpticalDepth
	Phase           CloudPhase
	EffRadius       unit.EffectiveRadiusUM
	Albedo          unit.SpectralAlbedo
	Asymmetry       unit.AsymmetryParameter
	Morphology      CloudMorphology
	Uncertainty     CloudUncertainty
	Source          SourceRef
}

// Aerosol is the broadband aerosol description of an Atmosphere. Phase 1 of
// Sky Brightness V2 treats it as broadband (one AOD/SSA/g/Angstrom set);
// a future phase may add a wavelength-dependent SSA table and a vertical
// profile behind the same accessor.
type Aerosol struct {
	OpticalDepth           unit.AerosolOpticalDepth
	ReferenceWavelength    unit.WavelengthNM
	AngstromExp            unit.AngstromExponent
	SingleScatteringAlbedo unit.SingleScatteringAlbedo
	Asymmetry              unit.AsymmetryParameter
	BoundaryLayerHeight    unit.AltitudeM
}

// TauAt returns the aerosol optical depth at wavelength lambda, via the
// Angstrom power law tau(lambda) = tau(lambda0) * (lambda/lambda0)^-alpha.
func (a Aerosol) TauAt(lambda unit.WavelengthNM) unit.AerosolOpticalDepth {
	if a.ReferenceWavelength <= 0 || lambda <= 0 {
		return a.OpticalDepth
	}

	ratio := float64(lambda) / float64(a.ReferenceWavelength)

	return unit.AerosolOpticalDepth(float64(a.OpticalDepth) * pow(ratio, -float64(a.AngstromExp)))
}

// SurfaceOptical is the ground reflectance under an Atmosphere. Albedo is
// broadband in Phase 1 — a genuinely wavelength-resolved spectral
// albedo/BRDF is future scope, behind this same accessor.
type SurfaceOptical struct {
	Albedo       unit.SpectralAlbedo
	SnowFraction float64 // [0,1]
}

// UniformAlbedo returns a SurfaceOptical with a flat broadband albedo and
// no snow.
func UniformAlbedo(a float64) SurfaceOptical {
	return SurfaceOptical{Albedo: unit.SpectralAlbedo(a)}
}

// HorizonProfile reports the local horizon altitude at a given azimuth,
// for terrain/obstruction screening.
type HorizonProfile interface {
	AltitudeAt(az angle.Angle) angle.Angle
}

// VerticalProfile is a placeholder for a layered pressure/temperature/
// humidity/molecular-number-density profile; it carries no fields yet.
// It exists now so Atmosphere's shape does not change again when a future
// phase starts consuming it.
type VerticalProfile struct{}

// Provenance records where an Atmosphere came from and how current it is.
type Provenance struct {
	Source   SourceRef
	IssueAt  time.Time // when the state was issued (nowcast/forecast)
	LeadTime time.Duration
}

// Atmosphere is an immutable, richer atmospheric description than
// Refraction (which is scoped to refraction-model inputs only): surface
// pressure/temperature, aerosol, clouds, surface reflectance, terrain
// horizon, and provenance. Construct one only via NewBuilder or a
// package-level default constructor.
//
// Renamed from State (v0.14.0-era design work, still unreleased) so the
// package's richer, general-purpose type gets the name a reader reaches for
// first — "the atmosphere" — while the narrower refraction-model input
// struct, which used to hold this name, becomes Refraction (see
// refraction.go). This is a same-release swap: Go cannot alias one
// identifier to two meanings at once, so Refraction's rename carries no
// deprecation cycle either — see refraction.go's doc comment.
//
// Atmosphere composes a Refraction as its own surface-conditions field
// (surface, below) rather than duplicating pressure/temperature as
// separate raw fields: Refraction stays the single, general-purpose
// representation of "conditions a refraction model needs," reused here
// instead of reinvented. Surface()/Refraction() convert at the one boundary
// where the two types' unit conventions differ (Kelvin here vs. Celsius in
// Refraction; see Builder.Surface's doc comment).
//
// Originally introduced for Sky Brightness V2 (docs/skybrightness.md §8)
// but general-purpose — any consumer needing aerosol/cloud/surface-optical
// atmospheric state (a future weather or seeing constraint, for instance)
// uses this same type rather than reinventing it.
type Atmosphere struct {
	surface       Refraction // pressure/temperature/humidity/wavelength/refraction model
	profile       VerticalProfile
	ozone         unit.OzoneColumnDU
	pwv           unit.PrecipitableWaterMM
	aerosol       Aerosol
	clouds        []CloudLayer
	groundOptical SurfaceOptical
	horizon       HorizonProfile
	provenance    Provenance
	issuedAt      time.Time

	// diffuseKappa scales the optical depth an extended source is attenuated
	// by; see DiffuseKappa. Zero means unset and reads as DefaultDiffuseKappa.
	diffuseKappa float64
}

// DiffuseKappa returns the factor the optical depth is scaled by when
// attenuating a source that fills the sky rather than a point one.
//
// A star loses everything scattered out of the line of sight. A source
// covering the whole sky does not: what is scattered out of one direction is
// replaced by light scattered in from every other, so attenuating an extended
// source by the full optical depth overstates the loss. Masana et al. (2021)
// Section 7 handles this by replacing the optical depth with an effective one,
// tau_eff = kappa * tau, and notes that kappa depends on the aerosol albedo
// and asymmetry parameter with typical values from 0.5 to 0.9 (Hong et al.
// 1998). Duriscoe (2013) uses 0.75 for diffuse sources, after Kwon (1989);
// the GAMBONS web service uses 0.5.
//
// It is not a fudge factor for matching a reference. It stands in for the
// scattered term of Masana et al. Eq. 8, whose exact form is a double integral
// over the hemisphere for every direction of observation.
func (s *Atmosphere) DiffuseKappa() float64 {
	if s.diffuseKappa <= 0 {
		return DefaultDiffuseKappa
	}

	return s.diffuseKappa
}

// Surface returns the surface pressure and temperature.
func (s *Atmosphere) Surface() (unit.PressureHPa, unit.TemperatureK) {
	return unit.PressureHPa(s.surface.Pressure), unit.TemperatureK(s.surface.Temperature + 273.15)
}

// Refraction returns this Atmosphere's own refraction conditions —
// pressure, temperature, humidity, wavelength, and refraction model — as a
// Refraction value, ready to hand to a RefractionModel. No model argument
// is needed: Atmosphere owns one via Builder.Refraction (zero value means
// no model was set, which RefractionModel implementations already treat as
// "let the caller's own default apply").
func (s *Atmosphere) Refraction() Refraction { return s.surface }

// Profile returns the layered vertical profile (placeholder today).
func (s *Atmosphere) Profile() VerticalProfile { return s.profile }

// Ozone returns the total-column ozone amount.
func (s *Atmosphere) Ozone() unit.OzoneColumnDU { return s.ozone }

// PrecipitableWater returns the precipitable water vapour column.
func (s *Atmosphere) PrecipitableWater() unit.PrecipitableWaterMM { return s.pwv }

// Aerosol returns the broadband aerosol state.
func (s *Atmosphere) Aerosol() Aerosol { return s.aerosol }

// Clouds returns a defensive copy of the cloud layers — mutating the
// returned slice never affects the Atmosphere.
func (s *Atmosphere) Clouds() []CloudLayer {
	cp := make([]CloudLayer, len(s.clouds))
	copy(cp, s.clouds)

	return cp
}

// SurfaceOptical returns the ground reflectance.
func (s *Atmosphere) SurfaceOptical() SurfaceOptical { return s.groundOptical }

// Horizon returns the terrain/obstruction horizon profile, or nil if none
// was set.
func (s *Atmosphere) Horizon() HorizonProfile { return s.horizon }

// Provenance returns this Atmosphere's provenance.
func (s *Atmosphere) Provenance() Provenance { return s.provenance }

// Age returns now minus the state's issue time (zero if it was never
// set, e.g. a climatology default).
func (s *Atmosphere) Age(now time.Time) time.Duration {
	if s.issuedAt.IsZero() {
		return 0
	}

	return now.Sub(s.issuedAt)
}

// Builder builds an immutable Atmosphere.
type Builder struct {
	s    Atmosphere
	errs []error
}

// NewBuilder starts a builder with standard-atmosphere defaults (1013.25
// hPa, 288.15 K), no aerosol, no clouds, zero albedo.
func NewBuilder() *Builder {
	return &Builder{s: Atmosphere{surface: Refraction{Pressure: 1013.25, Temperature: 288.15 - 273.15}}}
}

// Surface sets surface pressure (hPa) and temperature (K) on the Atmosphere's
// embedded Refraction. tempK is converted to Celsius internally — the one
// explicit unit conversion this composition needs, since Refraction (shared
// with coord/plan's hot refraction-call paths) uses Celsius while
// Atmosphere's own Surface() accessor stays in Kelvin for consistency with
// every other skybrightness-native unit.
func (b *Builder) Surface(pressureHPa, tempK float64) *Builder {
	if pressureHPa <= 0 {
		b.errs = append(b.errs, errSurfacePressure)
	}

	if tempK <= 0 {
		b.errs = append(b.errs, errSurfaceTemperature)
	}

	b.s.surface.Pressure = pressureHPa
	b.s.surface.Temperature = tempK - 273.15

	return b
}

// Refraction sets the remaining fields of the Atmosphere's embedded
// Refraction — the model, relative humidity [0,1], and wavelength in
// micrometres — for a caller who wants explicit refraction-model control
// alongside the sky-brightness Atmosphere. Pressure/temperature stay owned
// by Surface; this only touches the fields Surface doesn't set.
func (b *Builder) Refraction(model RefractionModel, humidityFrac, wavelengthUM float64) *Builder {
	b.s.surface.Model = model
	b.s.surface.Humidity = humidityFrac
	b.s.surface.Wavelength = wavelengthUM

	return b
}

// Aerosol sets a broadband aerosol state: optical depth at referenceNM,
// the Angstrom exponent, single-scattering albedo, and asymmetry
// parameter.
func (b *Builder) Aerosol(aod, referenceNM, angstrom, ssa, g float64) *Builder {
	if aod < 0 {
		b.errs = append(b.errs, errAerosolAOD)
	}

	if ssa < 0 || ssa > 1 {
		b.errs = append(b.errs, errAerosolSSA)
	}

	if g < -1 || g > 1 {
		b.errs = append(b.errs, errAerosolAsymmetry)
	}

	b.s.aerosol.OpticalDepth = unit.AerosolOpticalDepth(aod)
	b.s.aerosol.ReferenceWavelength = unit.WavelengthNM(referenceNM)
	b.s.aerosol.AngstromExp = unit.AngstromExponent(angstrom)
	b.s.aerosol.SingleScatteringAlbedo = unit.SingleScatteringAlbedo(ssa)
	b.s.aerosol.Asymmetry = unit.AsymmetryParameter(g)

	return b
}

// DiffuseScattering sets the effective-optical-depth factor kappa used when
// attenuating sources that fill the sky. See [Atmosphere.DiffuseKappa].
//
// Values outside (0, 1] are ignored: kappa above one would mean an extended
// source is dimmed more than a point source in the same air, which is the
// wrong way round, and zero or negative would mean the atmosphere brightens it.
func (b *Builder) DiffuseScattering(kappa float64) *Builder {
	if kappa > 0 && kappa <= 1 {
		b.s.diffuseKappa = kappa
	}

	return b
}

// Ozone sets the total-column ozone amount, in Dobson units.
func (b *Builder) Ozone(du float64) *Builder {
	b.s.ozone = unit.OzoneColumnDU(du)
	return b
}

// PrecipitableWater sets precipitable water vapour, in millimetres.
func (b *Builder) PrecipitableWater(mm float64) *Builder {
	b.s.pwv = unit.PrecipitableWaterMM(mm)
	return b
}

// BoundaryLayer sets the aerosol boundary-layer height, in metres.
func (b *Builder) BoundaryLayer(m float64) *Builder {
	b.s.aerosol.BoundaryLayerHeight = unit.AltitudeM(m)
	return b
}

// Clear removes every cloud layer — an explicit clear-sky state.
func (b *Builder) Clear() *Builder {
	b.s.clouds = nil
	return b
}

// AddCloud appends one cloud layer.
func (b *Builder) AddCloud(l CloudLayer) *Builder {
	if l.Fraction < 0 || l.Fraction > 1 {
		b.errs = append(b.errs, errCloudFraction)
	}

	if l.OpticalDepth < 0 {
		b.errs = append(b.errs, errCloudOpticalDepth)
	}

	b.s.clouds = append(b.s.clouds, l)

	return b
}

// SurfaceAlbedo sets the ground reflectance.
func (b *Builder) SurfaceAlbedo(s SurfaceOptical) *Builder {
	b.s.groundOptical = s
	return b
}

// Horizon sets the terrain/obstruction horizon profile.
func (b *Builder) Horizon(h HorizonProfile) *Builder {
	b.s.horizon = h
	return b
}

// Source attaches this state's provenance.
func (b *Builder) Source(ref SourceRef) *Builder {
	b.s.provenance.Source = ref
	return b
}

// IssuedAt sets the state's issue time (for nowcast/forecast use).
func (b *Builder) IssuedAt(t time.Time) *Builder {
	b.s.issuedAt = t
	b.s.provenance.IssueAt = t

	return b
}

// LeadTime sets a forecast state's lead time.
func (b *Builder) LeadTime(d time.Duration) *Builder {
	b.s.provenance.LeadTime = d
	return b
}

// Build validates the accumulated inputs and returns an immutable
// Atmosphere.
func (b *Builder) Build() (*Atmosphere, error) {
	if len(b.errs) > 0 {
		return nil, errors.Join(append([]error{ErrAtmosphereBuilder}, b.errs...)...)
	}

	out := b.s // copy the value; the builder's slice fields (clouds) are only appended to, never shared onward
	out.clouds = append([]CloudLayer(nil), b.s.clouds...)

	return &out, nil
}

// StandardDefault returns a deterministic, offline, site-elevation-aware
// default Atmosphere: no aerosol, no clouds, pressure/temperature from the
// ICAO ISA barometric profile (AtAltitude) at heightM, zero surface
// albedo. Its zero aerosol is exact, not approximate — this is the
// Rayleigh-only reference case, the Atmosphere counterpart to
// atmos.RayleighOnly's transmission model. For a real, named aerosol
// regime instead of the zero-aerosol baseline, see RuralAerosol/
// UrbanAerosol/DesertAerosol/MaritimeAerosol.
func StandardDefault(heightM float64) *Atmosphere {
	s := Atmosphere{
		surface: AtAltitude(heightM),
		provenance: Provenance{
			Source: SourceRef{
				Name:     "ICAO ISA barometric profile (standard default)",
				Fidelity: FidelityPrior,
			},
		},
	}

	return &s
}

func pow(x, y float64) float64 {
	if x <= 0 {
		return 0
	}

	return math.Pow(x, y)
}
