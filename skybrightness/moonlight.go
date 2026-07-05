package skybrightness

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
)

// ErrNilContext is returned when a time/observer-dependent component is
// evaluated without a coord.Context.
var ErrNilContext = errors.New("skybrightness: nil coord.Context")

// defaultExtinctionV is the default V-band atmospheric extinction coefficient
// (mag/airmass), the value Krisciunas & Schaefer (1991) used for Mauna Kea.
const defaultExtinctionV = 0.172

const degToRad = math.Pi / 180

// Moonlight is the scattered-moonlight component, implementing the closed-form
// V-band model of Krisciunas & Schaefer (1991), PASP 103, 1033. The model is
// accurate to ~8–23% away from full Moon. The contribution is zero when the
// Moon is below the horizon.
type Moonlight struct {
	provider eph.Provider
	k        float64
}

// MoonlightOption configures optional Moonlight fields.
type MoonlightOption func(*Moonlight)

// WithExtinction sets the V-band atmospheric extinction coefficient
// (mag/airmass). The default is 0.172 (KS 1991, Mauna Kea).
func WithExtinction(k float64) MoonlightOption {
	return func(m *Moonlight) { m.k = k }
}

// WithProvider sets the ephemeris provider used for Moon and Sun positions.
// The default is ephemeris.Default().
func WithProvider(p eph.Provider) MoonlightOption {
	return func(m *Moonlight) { m.provider = p }
}

// NewMoonlight creates a scattered-moonlight component.
func NewMoonlight(opts ...MoonlightOption) Moonlight {
	m := Moonlight{provider: eph.Default(), k: defaultExtinctionV}
	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// Radiance returns the scattered-moonlight radiance toward altaz at the epoch
// carried by ctx. It is zero when the Moon is below the horizon.
func (m Moonlight) Radiance(altaz coord.AltAz, ctx *coord.Context) (Nanolambert, error) {
	if ctx == nil {
		return 0, ErrNilContext
	}

	t := ctx.Time()

	moonVec, err := eph.Position(m.provider, eph.Moon, t)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: moon position: %w", err)
	}

	moonICRS, err := eph.ToICRS(moonVec)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: moon ICRS: %w", err)
	}

	moonAA, err := ctx.ICRSToAltAz(moonICRS)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: moon alt-az: %w", err)
	}

	// Moon below the horizon: no scattered moonlight.
	if moonAA.Alt().Degrees() <= 0 {
		return 0, nil
	}

	sunVec, err := eph.Position(m.provider, eph.Sun, t)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: sun position: %w", err)
	}

	sunICRS, err := eph.ToICRS(sunVec)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: sun ICRS: %w", err)
	}

	// Lunar phase angle α (degrees): 0 at full Moon. The Sun is effectively at
	// infinity, so α ≈ 180° − (geocentric Sun–Moon elongation).
	alpha := 180 - coord.Separation(sunICRS, moonICRS).Degrees()

	// Moon–target separation ρ in the local horizontal frame.
	rho := separationAltAz(altaz, moonAA).Degrees()

	zMoon := math.Pi/2 - moonAA.Alt().Radians()
	zTarget := math.Pi/2 - altaz.Alt().Radians()

	b := moonBrightnessNL(rho, alpha, zMoon, zTarget, m.k)
	if b < 0 {
		b = 0
	}

	return Nanolambert(b), nil
}

// moonBrightnessNL evaluates the Krisciunas & Schaefer (1991) scattered-moonlight
// brightness in nanolamberts. Inputs use the natural units of the paper:
//   - rhoDeg:   Moon–target angular separation (degrees)
//   - alphaDeg: lunar phase angle (degrees; 0 = full Moon)
//   - zMoon, zTarget: zenith angles of the Moon and target (radians)
//   - k:        V-band extinction coefficient (mag/airmass)
//
// Model (KS 1991, eqs. 15, 18, 20, 21, 3):
//
//	f(ρ)   = 10^5.36·(1.06 + cos²ρ) + 10^(6.15 − ρ/40)   [Rayleigh + Mie/aureole]
//	I*(α)  = 10^(−0.4·(3.84 + 0.026·|α| + 4e−9·α⁴))       [lunar illuminance]
//	X(z)   = (1 − 0.96·sin²z)^(−1/2)                       [airmass]
//	B_moon = f(ρ)·I*·10^(−0.4·k·X(zMoon))·(1 − 10^(−0.4·k·X(zTarget)))
func moonBrightnessNL(rhoDeg, alphaDeg, zMoon, zTarget, k float64) float64 {
	cosRho := math.Cos(rhoDeg * degToRad)
	fRho := math.Pow(10, 5.36)*(1.06+cosRho*cosRho) + math.Pow(10, 6.15-rhoDeg/40)

	a := math.Abs(alphaDeg)
	iStar := math.Pow(10, -0.4*(3.84+0.026*a+4e-9*a*a*a*a))

	xMoon := ksAirmass(zMoon)
	xTarget := ksAirmass(zTarget)

	return fRho * iStar * math.Pow(10, -0.4*k*xMoon) * (1 - math.Pow(10, -0.4*k*xTarget))
}

// ksAirmass is the Krisciunas & Schaefer (1991) airmass X(z) = (1 − 0.96·sin²z)^(−1/2),
// which (unlike sec z) stays finite at the horizon (X → 5.0 at z = 90°). z is in radians.
func ksAirmass(z float64) float64 {
	s := math.Sin(z)

	return 1.0 / math.Sqrt(1-0.96*s*s)
}

// separationAltAz returns the great-circle angle between two horizontal directions.
func separationAltAz(a, b coord.AltAz) angle.Angle {
	va := a.ToUnitVector()
	vb := b.ToUnitVector()

	return angle.Atan2(va.Cross(vb).Norm(), va.Dot(vb))
}
