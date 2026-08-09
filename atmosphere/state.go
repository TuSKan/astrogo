package atmosphere

import (
	"errors"
	"math"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/unit"
)

// ErrStateBuilder is returned by Builder.Build when the accumulated
// inputs are invalid (out-of-range pressure/temperature/AOD/SSA/etc.).
var ErrStateBuilder = errors.New("atmosphere: invalid State")

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

// Aerosol is the broadband aerosol description of a State. Phase 1 of
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

// SurfaceOptical is the ground reflectance under a State. Albedo is
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
// It exists now so State's shape does not change again when a future
// phase starts consuming it.
type VerticalProfile struct{}

// Provenance records where a State came from and how current it is.
type Provenance struct {
	Source   SourceRef
	IssueAt  time.Time // when the state was issued (nowcast/forecast)
	LeadTime time.Duration
}

// State is an immutable, richer atmospheric description than Atmosphere
// (which is scoped to refraction inputs only): surface pressure/
// temperature, aerosol, clouds, surface reflectance, terrain horizon, and
// provenance. Construct one only via NewBuilder or a package-level
// default constructor. Originally introduced for Sky Brightness V2
// (docs/skybrightness.md §8) but general-purpose — any consumer needing
// aerosol/cloud/surface-optical atmospheric state (a future weather or
// seeing constraint, for instance) uses this same type rather than
// reinventing it.
type State struct {
	pressure    unit.PressureHPa
	temperature unit.TemperatureK
	profile     VerticalProfile
	ozone       unit.OzoneColumnDU
	pwv         unit.PrecipitableWaterMM
	aerosol     Aerosol
	clouds      []CloudLayer
	surface     SurfaceOptical
	horizon     HorizonProfile
	provenance  Provenance
	issuedAt    time.Time
}

// Surface returns the surface pressure and temperature.
func (s *State) Surface() (unit.PressureHPa, unit.TemperatureK) { return s.pressure, s.temperature }

// Profile returns the layered vertical profile (placeholder today).
func (s *State) Profile() VerticalProfile { return s.profile }

// Ozone returns the total-column ozone amount.
func (s *State) Ozone() unit.OzoneColumnDU { return s.ozone }

// PrecipitableWater returns the precipitable water vapour column.
func (s *State) PrecipitableWater() unit.PrecipitableWaterMM { return s.pwv }

// Aerosol returns the broadband aerosol state.
func (s *State) Aerosol() Aerosol { return s.aerosol }

// Clouds returns a defensive copy of the cloud layers — mutating the
// returned slice never affects the State.
func (s *State) Clouds() []CloudLayer {
	cp := make([]CloudLayer, len(s.clouds))
	copy(cp, s.clouds)

	return cp
}

// SurfaceOptical returns the ground reflectance.
func (s *State) SurfaceOptical() SurfaceOptical { return s.surface }

// Horizon returns the terrain/obstruction horizon profile, or nil if none
// was set.
func (s *State) Horizon() HorizonProfile { return s.horizon }

// Provenance returns this state's provenance.
func (s *State) Provenance() Provenance { return s.provenance }

// Age returns now minus the state's issue time (zero if it was never
// set, e.g. a climatology default).
func (s *State) Age(now time.Time) time.Duration {
	if s.issuedAt.IsZero() {
		return 0
	}

	return now.Sub(s.issuedAt)
}

// Builder builds an immutable State.
type Builder struct {
	s    State
	errs []error
}

// NewBuilder starts a builder with standard-atmosphere defaults (1013.25
// hPa, 288.15 K), no aerosol, no clouds, zero albedo.
func NewBuilder() *Builder {
	return &Builder{s: State{pressure: 1013.25, temperature: 288.15}}
}

// Surface sets surface pressure (hPa) and temperature (K).
func (b *Builder) Surface(pressureHPa, tempK float64) *Builder {
	if pressureHPa <= 0 {
		b.errs = append(b.errs, errSurfacePressure)
	}

	if tempK <= 0 {
		b.errs = append(b.errs, errSurfaceTemperature)
	}

	b.s.pressure, b.s.temperature = unit.PressureHPa(pressureHPa), unit.TemperatureK(tempK)

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
	b.s.surface = s
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

// Build validates the accumulated inputs and returns an immutable State.
func (b *Builder) Build() (*State, error) {
	if len(b.errs) > 0 {
		return nil, errors.Join(append([]error{ErrStateBuilder}, b.errs...)...)
	}

	out := b.s // copy the value; the builder's slice fields (clouds) are only appended to, never shared onward
	out.clouds = append([]CloudLayer(nil), b.s.clouds...)

	return &out, nil
}

// StandardDefault returns a deterministic, offline, site-elevation-aware
// default State: no aerosol, no clouds, pressure/temperature from the
// ICAO ISA barometric profile (AtAltitude) at heightM, zero surface
// albedo.
func StandardDefault(heightM float64) *State {
	env := AtAltitude(heightM)

	s := State{
		pressure:    unit.PressureHPa(env.Pressure),
		temperature: unit.TemperatureK(env.Temperature + 273.15),
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
