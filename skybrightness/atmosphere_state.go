package skybrightness

import (
	"errors"
	"math"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
)

// ErrAtmosphereBuilder is returned by AtmosphereBuilder.Build when the
// accumulated inputs are invalid (out-of-range pressure/temperature/AOD/
// SSA/etc.).
var ErrAtmosphereBuilder = errors.New("skybrightness: invalid AtmosphereState")

// Sentinel components AtmosphereBuilder.Build's aggregated error may join,
// wrapped via errors.Join so a caller can errors.Is against any specific
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

// CloudMorphology is a coarse structural descriptor of a cloud layer, used
// by the Phase 5 stochastic broken-cloud description.
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
// fraction and optical depth, feeding GroupCloud.
type CloudUncertainty struct {
	FractionRelSigma     float64
	OpticalDepthRelSigma float64
}

// CloudLayer describes one cloud layer. Fraction (sky cover) and
// OpticalDepth are deliberately distinct fields of distinct types — the
// mandate's "never collapse cloud state to one scalar" rule, enforced by
// the compiler (docs/skybrightness.md §8).
type CloudLayer struct {
	Fraction        CloudFraction
	BaseAlt, TopAlt AltitudeM
	OpticalDepth    CloudOpticalDepth
	Phase           CloudPhase
	EffRadius       EffectiveRadiusUM
	Albedo          SpectralAlbedo
	Asymmetry       AsymmetryParameter
	Morphology      CloudMorphology
	Uncertainty     CloudUncertainty
	Source          SourceRef
}

// AerosolState is the aerosol description of an AtmosphereState. Phase 1
// treats it as broadband (one AOD/SSA/g/Angstrom set); Phase 3 adds a
// wavelength-dependent SSA table and a vertical profile behind the same
// accessor.
type AerosolState struct {
	OpticalDepth           AerosolOpticalDepth
	ReferenceWavelength    WavelengthNM
	AngstromExp            AngstromExponent
	SingleScatteringAlbedo SingleScatteringAlbedo
	Asymmetry              AsymmetryParameter
	BoundaryLayerHeight    AltitudeM
}

// TauAt returns the aerosol optical depth at wavelength lambda, via the
// Angstrom power law tau(lambda) = tau(lambda0) * (lambda/lambda0)^-alpha.
func (a AerosolState) TauAt(lambda WavelengthNM) AerosolOpticalDepth {
	if a.ReferenceWavelength <= 0 || lambda <= 0 {
		return a.OpticalDepth
	}

	ratio := float64(lambda) / float64(a.ReferenceWavelength)

	return AerosolOpticalDepth(float64(a.OpticalDepth) * pow(ratio, -float64(a.AngstromExp)))
}

// SurfaceOptical is the ground reflectance under an AtmosphereState.
// Albedo is broadband in Phase 1 — a genuinely wavelength-resolved
// spectral albedo/BRDF is Phase 3+ scope, behind this same accessor.
type SurfaceOptical struct {
	Albedo       SpectralAlbedo
	SnowFraction float64 // [0,1]
}

// UniformAlbedo returns a SurfaceOptical with a flat broadband albedo and
// no snow.
func UniformAlbedo(a float64) SurfaceOptical {
	return SurfaceOptical{Albedo: SpectralAlbedo(a)}
}

// HorizonProfile reports the local horizon altitude at a given azimuth,
// for terrain/obstruction screening (Phase 4+).
type HorizonProfile interface {
	AltitudeAt(az angle.Angle) angle.Angle
}

// VerticalProfile is a placeholder for the layered pressure/temperature/
// humidity/molecular-number-density profile the mandate asks for; it
// carries no fields before Phase 3, which is when atmos/molecular.go
// starts consuming it. It exists now so AtmosphereState's shape does not
// change again when Phase 3 lands.
type VerticalProfile struct{}

// AtmosphereState is an immutable description of the atmosphere and
// surface for one evaluation. Construct one only via AtmosphereBuilder or
// ClimatologyDefaultAtmosphere.
type AtmosphereState struct {
	pressure    PressureHPa
	temperature TemperatureK
	profile     VerticalProfile
	ozone       OzoneColumnDU
	pwv         PrecipitableWaterMM
	aerosol     AerosolState
	clouds      []CloudLayer
	surface     SurfaceOptical
	horizon     HorizonProfile
	provenance  AtmosphereProvenance
	issuedAt    time.Time
}

// Surface returns the surface pressure and temperature.
func (s *AtmosphereState) Surface() (PressureHPa, TemperatureK) { return s.pressure, s.temperature }

// Profile returns the layered vertical profile (placeholder before Phase 3).
func (s *AtmosphereState) Profile() VerticalProfile { return s.profile }

// Ozone returns the total-column ozone amount.
func (s *AtmosphereState) Ozone() OzoneColumnDU { return s.ozone }

// PrecipitableWater returns the precipitable water vapour column.
func (s *AtmosphereState) PrecipitableWater() PrecipitableWaterMM { return s.pwv }

// Aerosol returns the broadband aerosol state.
func (s *AtmosphereState) Aerosol() AerosolState { return s.aerosol }

// Clouds returns a defensive copy of the cloud layers — mutating the
// returned slice never affects the AtmosphereState.
func (s *AtmosphereState) Clouds() []CloudLayer {
	cp := make([]CloudLayer, len(s.clouds))
	copy(cp, s.clouds)

	return cp
}

// SurfaceOptical returns the ground reflectance.
func (s *AtmosphereState) SurfaceOptical() SurfaceOptical { return s.surface }

// Horizon returns the terrain/obstruction horizon profile, or nil if none
// was set.
func (s *AtmosphereState) Horizon() HorizonProfile { return s.horizon }

// Provenance returns this state's provenance.
func (s *AtmosphereState) Provenance() AtmosphereProvenance { return s.provenance }

// Age returns now minus the state's issue time (zero if IssuedAt was never
// set, e.g. a Climatology state).
func (s *AtmosphereState) Age(now time.Time) time.Duration {
	if s.issuedAt.IsZero() {
		return 0
	}

	return now.Sub(s.issuedAt)
}

// AtmosphereBuilder builds an immutable AtmosphereState.
type AtmosphereBuilder struct {
	s    AtmosphereState
	errs []error
}

// NewAtmosphereBuilder starts a builder with standard-atmosphere defaults
// (1013.25 hPa, 288.15 K), no aerosol, no clouds, zero albedo.
func NewAtmosphereBuilder() *AtmosphereBuilder {
	return &AtmosphereBuilder{s: AtmosphereState{pressure: 1013.25, temperature: 288.15}}
}

// Surface sets surface pressure (hPa) and temperature (K).
func (b *AtmosphereBuilder) Surface(pressureHPa, tempK float64) *AtmosphereBuilder {
	if pressureHPa <= 0 {
		b.errs = append(b.errs, errSurfacePressure)
	}

	if tempK <= 0 {
		b.errs = append(b.errs, errSurfaceTemperature)
	}

	b.s.pressure, b.s.temperature = PressureHPa(pressureHPa), TemperatureK(tempK)

	return b
}

// Aerosol sets a broadband aerosol state: optical depth at referenceNM,
// the Angstrom exponent, single-scattering albedo, and asymmetry
// parameter.
func (b *AtmosphereBuilder) Aerosol(aod, referenceNM, angstrom, ssa, g float64) *AtmosphereBuilder {
	if aod < 0 {
		b.errs = append(b.errs, errAerosolAOD)
	}

	if ssa < 0 || ssa > 1 {
		b.errs = append(b.errs, errAerosolSSA)
	}

	if g < -1 || g > 1 {
		b.errs = append(b.errs, errAerosolAsymmetry)
	}

	b.s.aerosol.OpticalDepth = AerosolOpticalDepth(aod)
	b.s.aerosol.ReferenceWavelength = WavelengthNM(referenceNM)
	b.s.aerosol.AngstromExp = AngstromExponent(angstrom)
	b.s.aerosol.SingleScatteringAlbedo = SingleScatteringAlbedo(ssa)
	b.s.aerosol.Asymmetry = AsymmetryParameter(g)

	return b
}

// Ozone sets the total-column ozone amount, in Dobson units.
func (b *AtmosphereBuilder) Ozone(du float64) *AtmosphereBuilder {
	b.s.ozone = OzoneColumnDU(du)
	return b
}

// PrecipitableWater sets precipitable water vapour, in millimetres.
func (b *AtmosphereBuilder) PrecipitableWater(mm float64) *AtmosphereBuilder {
	b.s.pwv = PrecipitableWaterMM(mm)
	return b
}

// BoundaryLayer sets the aerosol boundary-layer height, in metres.
func (b *AtmosphereBuilder) BoundaryLayer(m float64) *AtmosphereBuilder {
	b.s.aerosol.BoundaryLayerHeight = AltitudeM(m)
	return b
}

// Clear removes every cloud layer — an explicit clear-sky state.
func (b *AtmosphereBuilder) Clear() *AtmosphereBuilder {
	b.s.clouds = nil
	return b
}

// AddCloud appends one cloud layer.
func (b *AtmosphereBuilder) AddCloud(l CloudLayer) *AtmosphereBuilder {
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
func (b *AtmosphereBuilder) SurfaceAlbedo(s SurfaceOptical) *AtmosphereBuilder {
	b.s.surface = s
	return b
}

// Horizon sets the terrain/obstruction horizon profile.
func (b *AtmosphereBuilder) Horizon(h HorizonProfile) *AtmosphereBuilder {
	b.s.horizon = h
	return b
}

// Source attaches this state's provenance.
func (b *AtmosphereBuilder) Source(ref SourceRef) *AtmosphereBuilder {
	b.s.provenance.Source = ref
	return b
}

// IssuedAt sets the state's issue time (for Nowcast/Forecast modes).
func (b *AtmosphereBuilder) IssuedAt(t time.Time) *AtmosphereBuilder {
	b.s.issuedAt = t
	b.s.provenance.IssueAt = t

	return b
}

// LeadTime sets a Forecast state's lead time.
func (b *AtmosphereBuilder) LeadTime(d time.Duration) *AtmosphereBuilder {
	b.s.provenance.LeadTime = d
	return b
}

// Build validates the accumulated inputs and returns an immutable
// AtmosphereState.
func (b *AtmosphereBuilder) Build() (*AtmosphereState, error) {
	if len(b.errs) > 0 {
		return nil, errors.Join(append([]error{ErrAtmosphereBuilder}, b.errs...)...)
	}

	out := b.s // copy the value; the builder's slice fields (clouds) are only appended to, never shared onward
	out.clouds = append([]CloudLayer(nil), b.s.clouds...)

	return &out, nil
}

// ClimatologyDefaultAtmosphere returns a deterministic, offline,
// site-elevation-aware default AtmosphereState: no aerosol, no clouds,
// pressure/temperature from the ICAO ISA barometric profile
// (atmosphere.AtAltitude) at the site's height, zero surface albedo. This
// is ModeClimatology's baseline — Phase 3 replaces the aerosol/cloud
// fields with a real climatological dataset behind the same constructor
// shape.
func ClimatologyDefaultAtmosphere(site *coord.Geodetic) *AtmosphereState {
	h := 0.0
	if site != nil {
		h = site.Height()
	}

	env := atmosphere.AtAltitude(h)

	s := AtmosphereState{
		pressure:    PressureHPa(env.Pressure),
		temperature: TemperatureK(env.Temperature + 273.15),
		provenance: AtmosphereProvenance{
			Source: SourceRef{
				Name:     "ICAO ISA barometric profile (climatology default)",
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
