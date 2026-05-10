package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	mag "github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Comet represents a cometary target with M1/k1 total magnitude parameters.
type Comet struct {
	provider eph.Provider
	name     string
	M1       float64
	K1       float64
	M2       float64
	K2       float64
	id       eph.ID
}

// CometOption configures optional Comet fields.
type CometOption func(*Comet)

// WithNuclearMagnitude sets the nuclear magnitude parameters.
func WithNuclearMagnitude(m2, k2 float64) CometOption {
	return func(c *Comet) { c.M2 = m2; c.K2 = k2 }
}

// NewCometWithOptions creates a comet target with optional parameters.
func NewComet(name string, id eph.ID, provider eph.Provider, m1, k1 float64, opts ...CometOption) *Comet {
	c := &Comet{
		name:     name,
		id:       id,
		provider: provider,
		M1:       m1,
		K1:       k1,
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Comet) Name() string           { return c.name }
func (c *Comet) Provider() eph.Provider { return c.provider }
func (c *Comet) EphID() eph.ID          { return c.id }

func (c *Comet) Position(t time.Time) (coord.ICRS, error) {
	pos, err := eph.Position(c.provider, c.id, t)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("comet: ephemeris error for %s: %w", c.name, err)
	}

	icrs, err := eph.ToICRS(pos)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("comet: coordinate conversion error for %s: %w", c.name, err)
	}

	return icrs, nil
}

func (c *Comet) GeocentricVec(t time.Time) (vector.Vec3, error) {
	return eph.Position(c.provider, c.id, t)
}

func (c *Comet) GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error) {
	return computeDetails(c, ctx, props...)
}

// ApparentMagnitude computes total apparent magnitude using M1/k1.
func (c *Comet) ApparentMagnitude(t time.Time) (float64, error) {
	r, delta, err := c.distances(t)
	if err != nil {
		return 0, err
	}

	return mag.CometApparent(c.M1, c.K1, r, delta), nil
}

func (c *Comet) ApparentMagnitudeCtx(t time.Time, _ *coord.Context) (float64, error) {
	return c.ApparentMagnitude(t)
}

// distances returns heliocentric and geocentric distances.
func (c *Comet) distances(t time.Time) (r, delta float64, err error) {
	st, err := c.provider.State(c.id, t)
	if err != nil {
		return 0, 0, err
	}

	sunSt, err := c.provider.State(eph.Sun, t)
	if err != nil {
		return 0, 0, err
	}

	delta = st.Distance()
	hx := st.Pos.X - sunSt.Pos.X
	hy := st.Pos.Y - sunSt.Pos.Y
	hz := st.Pos.Z - sunSt.Pos.Z
	r = math.Sqrt(hx*hx + hy*hy + hz*hz)

	return r, delta, nil
}
