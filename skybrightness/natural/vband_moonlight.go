package natural

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// ErrNilAstroContext is returned when VBandMoonlight is evaluated with a
// nil EvalInput.Astro.
var ErrNilAstroContext = errors.New("natural: nil coord.Context")

// DefaultMoonExtinctionV is the default V-band atmospheric extinction
// coefficient (mag/airmass) — the value Krisciunas & Schaefer (1991) used
// for Mauna Kea. Carried verbatim from astrogo v1.
const DefaultMoonExtinctionV = 0.172

const degToRad = math.Pi / 180

// VBandMoonlight is astrogo v1's scattered-moonlight component,
// re-implemented against the new Component interface: the closed-form
// V-band model of Krisciunas & Schaefer (1991), PASP 103, 1033, accurate
// to ~8-23% away from full Moon. Zero when the Moon is below the horizon.
//
// Named for what's scientifically distinct about it — a broadband V-band
// model, not spectral — the same convention ConstantAirglow's name
// follows, rather than for the paper it cites (that citation lives in
// Algorithm's AlgorithmRef, not the type name). This protects against
// collision with *any* future spectral moonlight model, not just Jones et
// al. 2013 specifically: "broadband V vs. spectral" is a structural
// distinction, independent of which paper a future model implements. See
// docs/skybrightness.md §15 — a brand-new type, not a compatibility shim.
type VBandMoonlight struct {
	provider eph.Provider
	k        float64
}

// VBandMoonlightOption configures optional VBandMoonlight fields.
type VBandMoonlightOption func(*VBandMoonlight)

// WithMoonExtinction sets the V-band atmospheric extinction coefficient
// (mag/airmass). The default is DefaultMoonExtinctionV.
func WithMoonExtinction(k float64) VBandMoonlightOption {
	return func(m *VBandMoonlight) { m.k = k }
}

// WithMoonProvider sets the ephemeris provider used for Moon and Sun
// positions. The default is ephemeris.Default(); an explicit nil is
// treated the same way.
func WithMoonProvider(p eph.Provider) VBandMoonlightOption {
	return func(m *VBandMoonlight) {
		if p == nil {
			p = eph.Default()
		}

		m.provider = p
	}
}

// NewVBandMoonlight creates a scattered-moonlight component.
func NewVBandMoonlight(opts ...VBandMoonlightOption) *VBandMoonlight {
	m := &VBandMoonlight{provider: eph.Default(), k: DefaultMoonExtinctionV}
	for _, opt := range opts {
		opt(m)
	}

	return m
}

// ID implements skybrightness.Component.
func (m *VBandMoonlight) ID() skybrightness.ComponentID {
	return skybrightness.MoonScattered
}

// Algorithm implements skybrightness.Component.
func (m *VBandMoonlight) Algorithm() skybrightness.AlgorithmRef {
	return skybrightness.AlgorithmRef{
		Name: "natural.VBandMoonlight", Version: "1.0.0",
		Citation: "Krisciunas & Schaefer (1991), PASP 103, 1033",
	}
}

// Eval implements skybrightness.Component.
func (m *VBandMoonlight) Eval(_ context.Context, in skybrightness.EvalInput, out skybrightness.SpectralField) (skybrightness.ComponentReport, error) {
	if in.Astro == nil {
		return skybrightness.ComponentReport{}, ErrNilAstroContext
	}

	t := in.Astro.Time()

	moonVec, err := eph.Position(m.provider, eph.Moon, t)
	if err != nil {
		return skybrightness.ComponentReport{}, fmt.Errorf("natural: moon position: %w", err)
	}

	moonICRS, err := eph.ToICRS(moonVec)
	if err != nil {
		return skybrightness.ComponentReport{}, fmt.Errorf("natural: moon ICRS: %w", err)
	}

	moonAA, err := in.Astro.ICRSToAltAz(moonICRS)
	if err != nil {
		return skybrightness.ComponentReport{}, fmt.Errorf("natural: moon alt-az: %w", err)
	}

	values := make([]unit.SpectralRadiance, len(in.Directions))

	if moonAA.Alt().Degrees() > 0 {
		sunVec, err := eph.Position(m.provider, eph.Sun, t)
		if err != nil {
			return skybrightness.ComponentReport{}, fmt.Errorf("natural: sun position: %w", err)
		}

		sunICRS, err := eph.ToICRS(sunVec)
		if err != nil {
			return skybrightness.ComponentReport{}, fmt.Errorf("natural: sun ICRS: %w", err)
		}

		// Lunar phase angle alpha (degrees): 0 at full Moon. The Sun is
		// effectively at infinity, so alpha ~= 180 - (geocentric
		// Sun-Moon elongation).
		alpha := 180 - coord.Separation(sunICRS, moonICRS).Degrees()
		zMoon := math.Pi/2 - moonAA.Alt().Radians()

		for i, altaz := range in.Directions {
			if altaz.Alt().Degrees() <= 0 {
				continue
			}

			rho := separationAltAz(altaz, moonAA).Degrees()
			zTarget := math.Pi/2 - altaz.Alt().Radians()

			nl := moonBrightnessNL(rho, alpha, zMoon, zTarget, m.k)
			if nl < 0 {
				nl = 0
			}

			values[i] = unit.SpectralRadiance(nl)
		}
	}

	fillFlat(in.Grid, out, values)

	return skybrightness.ComponentReport{
		Assumptions: []string{
			"V-band closed-form fit, accurate to ~8-23% away from full Moon (KS 1991)",
			"Garstang nanolambert convention, not SI radiance — meaningful only via VegaSurfaceBrightness against TopHatJohnsonV",
		},
		Uncertainty: skybrightness.ComponentUncertainty{RelSigma: 0.15, Group: skybrightness.GroupNatural, Kind: skybrightness.Aleatoric},
		Provenance: skybrightness.ComponentProvenance{
			Component: skybrightness.MoonScattered, Algorithm: m.Algorithm(),
		},
		Quality: skybrightness.QualityFlagApproximatePhysics,
	}, nil
}

// moonBrightnessNL evaluates the Krisciunas & Schaefer (1991)
// scattered-moonlight brightness in nanolamberts. Inputs use the natural
// units of the paper:
//   - rhoDeg:   Moon-target angular separation (degrees)
//   - alphaDeg: lunar phase angle (degrees; 0 = full Moon)
//   - zMoon, zTarget: zenith angles of the Moon and target (radians)
//   - k:        V-band extinction coefficient (mag/airmass)
//
// Model (KS 1991, eqs. 15, 18, 20, 21, 3):
//
//	f(rho)  = 10^5.36*(1.06 + cos^2(rho)) + 10^(6.15 - rho/40)   [Rayleigh + Mie/aureole]
//	I*(a)   = 10^(-0.4*(3.84 + 0.026*|a| + 4e-9*a^4))            [lunar illuminance]
//	X(z)    = (1 - 0.96*sin^2(z))^(-1/2)                          [airmass]
//	B_moon  = f(rho)*I* * 10^(-0.4*k*X(zMoon)) * (1 - 10^(-0.4*k*X(zTarget)))
func moonBrightnessNL(rhoDeg, alphaDeg, zMoon, zTarget, k float64) float64 {
	cosRho := math.Cos(rhoDeg * degToRad)
	fRho := math.Pow(10, 5.36)*(1.06+cosRho*cosRho) + math.Pow(10, 6.15-rhoDeg/40)

	a := math.Abs(alphaDeg)
	iStar := math.Pow(10, -0.4*(3.84+0.026*a+4e-9*a*a*a*a))

	xMoon := ksAirmass(zMoon)
	xTarget := ksAirmass(zTarget)

	return fRho * iStar * math.Pow(10, -0.4*k*xMoon) * (1 - math.Pow(10, -0.4*k*xTarget))
}

// ksAirmass is the Krisciunas & Schaefer (1991) airmass
// X(z) = (1 - 0.96*sin^2(z))^(-1/2), which (unlike sec z) stays finite at
// the horizon (X -> 5.0 at z = 90deg). z is in radians.
func ksAirmass(z float64) float64 {
	s := math.Sin(z)

	return 1.0 / math.Sqrt(1-0.96*s*s)
}

// separationAltAz returns the great-circle angle between two horizontal
// directions.
func separationAltAz(a, b coord.AltAz) angle.Angle {
	va := a.ToUnitVector()
	vb := b.ToUnitVector()

	return angle.Atan2(va.Cross(vb).Norm(), va.Dot(vb))
}
