package plan

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// DeepSkyObject represents a fixed deep-sky target (galaxy, nebula, cluster, etc.).
type DeepSkyObject struct {
	name    string
	kind    string
	aliases []string
	coord   coord.ICRS
	vMag    float64
	hasVMag bool
}

// DSOOption configures optional DeepSkyObject fields.
type DSOOption func(*DeepSkyObject)

// WithDSOMagnitude sets the catalog V-band magnitude.
func WithDSOMagnitude(v float64) DSOOption {
	return func(d *DeepSkyObject) { d.vMag = v; d.hasVMag = true }
}

// WithDSOKind sets the display kind (e.g. "Galaxy", "Nebula").
func WithDSOKind(kind string) DSOOption {
	return func(d *DeepSkyObject) { d.kind = kind }
}

// WithDSOAliases sets alternative designations.
func WithDSOAliases(aliases ...string) DSOOption {
	return func(d *DeepSkyObject) { d.aliases = aliases }
}

// NewDeepSkyObject creates a fixed deep-sky target.
func NewDeepSkyObject(name string, ra, dec angle.Angle, opts ...DSOOption) *DeepSkyObject {
	d := &DeepSkyObject{
		name:  name,
		coord: coord.NewICRS(ra, dec),
	}
	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Name returns the object's display name.
func (d *DeepSkyObject) Name() string { return d.name }

// Position returns the fixed ICRS position (time-independent).
func (d *DeepSkyObject) Position(_ time.Time) (coord.ICRS, error) {
	return d.coord, nil
}

// GetDetails computes observational details using the given coordinate context.
func (d *DeepSkyObject) GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error) {
	return computeDetails(d, ctx, props...)
}
