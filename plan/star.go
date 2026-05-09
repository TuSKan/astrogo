package plan

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Star represents a fixed sidereal target with optional proper motion,
// parallax, and catalog V-band magnitude.
type Star struct {
	name           string
	coord          coord.ICRS
	pmRA, pmDec    angle.Angle
	parallax       angle.Angle
	radialVelocity float64
	vMag           float64
	hasVMag        bool
	aliases        []string
}

// StarOption configures optional Star fields.
type StarOption func(*Star)

// WithProperMotion sets proper motion in RA and Dec.
func WithProperMotion(pmRA, pmDec angle.Angle) StarOption {
	return func(s *Star) { s.pmRA = pmRA; s.pmDec = pmDec }
}

// WithParallax sets the stellar parallax.
func WithParallax(p angle.Angle) StarOption {
	return func(s *Star) { s.parallax = p }
}

// WithRadialVelocity sets the radial velocity in km/s.
func WithRadialVelocity(rv float64) StarOption {
	return func(s *Star) { s.radialVelocity = rv }
}

// WithStarMagnitude sets the catalog V-band magnitude.
func WithStarMagnitude(v float64) StarOption {
	return func(s *Star) { s.vMag = v; s.hasVMag = true }
}

// WithAliases sets alternative designations (e.g. "M45", "NGC 1976").
func WithAliases(aliases ...string) StarOption {
	return func(s *Star) { s.aliases = aliases }
}

// NewStar creates a fixed sidereal target.
func NewStar(name string, ra, dec angle.Angle, opts ...StarOption) *Star {
	s := &Star{
		name:  name,
		coord: coord.NewICRS(ra, dec),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Star) Name() string { return s.name }

func (s *Star) Position(_ time.Time) (coord.ICRS, error) {
	hasPM := s.pmRA.Radians() != 0 || s.pmDec.Radians() != 0
	hasParallax := s.parallax.Radians() != 0
	if hasPM || hasParallax {
		return coord.NewICRSWithKinematics(
			s.coord.RA(), s.coord.Dec(),
			s.pmRA, s.pmDec,
			s.parallax, s.radialVelocity,
		), nil
	}
	return s.coord, nil
}

func (s *Star) GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error) {
	return computeDetails(s, ctx, props...)
}
