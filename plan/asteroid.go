package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	mag "github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// SpinAxis holds the pole direction of an asteroid's spin axis (J2000).
type SpinAxis struct {
	RA  float64 // degrees
	Dec float64 // degrees
}

// Asteroid represents a minor planet with phase-curve photometry parameters.
type Asteroid struct {
	provider eph.Provider
	spin     *SpinAxis
	name     string
	H        float64
	G        float64
	G1       float64
	G2       float64
	oblat    float64
	id       eph.ID
	hasG1G2  bool
}

// AsteroidOption configures optional Asteroid fields.
type AsteroidOption func(*Asteroid)

// WithHG sets the classic H,G parameters.
func WithHG(absH, slopeG float64) AsteroidOption {
	return func(a *Asteroid) { a.H = absH; a.G = slopeG }
}

// WithHG1G2 sets the three-parameter HG1G2 phase curve.
func WithHG1G2(absH, g1, g2 float64) AsteroidOption {
	return func(a *Asteroid) {
		a.H = absH
		a.G1 = g1
		a.G2 = g2
		a.hasG1G2 = true
	}
}

// WithSpin sets the spin axis pole direction and oblateness for sHG1G2.
func WithSpin(ra, dec, oblateness float64) AsteroidOption {
	return func(a *Asteroid) {
		a.spin = &SpinAxis{RA: ra, Dec: dec}
		a.oblat = oblateness
	}
}

// NewAsteroid creates an asteroid target.
func NewAsteroid(name string, id eph.ID, provider eph.Provider, opts ...AsteroidOption) *Asteroid {
	a := &Asteroid{
		name:     name,
		id:       id,
		provider: provider,
		G:        0.15, // IAU default
	}
	for _, opt := range opts {
		opt(a)
	}

	return a
}

func (a *Asteroid) Name() string           { return a.name }
func (a *Asteroid) Provider() eph.Provider { return a.provider }
func (a *Asteroid) EphID() eph.ID          { return a.id }

func (a *Asteroid) Position(t time.Time) (coord.ICRS, error) {
	pos, err := eph.Position(a.provider, a.id, t)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("asteroid: ephemeris error for %s: %w", a.name, err)
	}

	icrs, err := eph.ToICRS(pos)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("asteroid: coordinate conversion error for %s: %w", a.name, err)
	}

	return icrs, nil
}

func (a *Asteroid) GeocentricVec(t time.Time) (vector.Vec3, error) {
	v, err := eph.Position(a.provider, a.id, t)
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("asteroid: geocentric: %w", err)
	}

	return v, nil
}

func (a *Asteroid) GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error) {
	return computeDetails(a, ctx, props...)
}

// ApparentMagnitude computes the apparent magnitude using the best available
// model: sHG1G2 → HG1G2 → HG.
func (a *Asteroid) ApparentMagnitude(t time.Time) (float64, error) {
	r, delta, alpha, st, err := a.helioGeometry(t)
	if err != nil {
		return 0, err
	}

	switch {
	case a.hasG1G2 && a.spin != nil && a.oblat > 0:
		ra := angle.Rad(math.Atan2(st.Pos.Y, st.Pos.X))
		dec := angle.Rad(math.Asin(st.Pos.Z / delta))
		cosL := mag.CosAspectAngle(ra, dec, angle.Deg(a.spin.RA), angle.Deg(a.spin.Dec))

		return mag.AsteroidSHG1G2(a.H, a.G1, a.G2, r, delta, alpha, a.oblat, cosL), nil
	case a.hasG1G2:
		return mag.AsteroidHG1G2(a.H, a.G1, a.G2, r, delta, alpha), nil
	default:
		return mag.AsteroidHG(a.H, a.G, r, delta, alpha), nil
	}
}

func (a *Asteroid) ApparentMagnitudeCtx(t time.Time, _ *coord.Context) (float64, error) {
	return a.ApparentMagnitude(t)
}

// helioGeometry computes heliocentric distance r, geocentric distance Δ,
// and phase angle α.
func (a *Asteroid) helioGeometry(t time.Time) (r, delta float64, alpha angle.Angle, st eph.State, err error) {
	st, err = a.provider.State(a.id, t)
	if err != nil {
		return r, delta, alpha, st, fmt.Errorf("asteroid: state: %w", err)
	}

	sunSt, err := a.provider.State(eph.Sun, t)
	if err != nil {
		return r, delta, alpha, st, fmt.Errorf("asteroid: sun state: %w", err)
	}

	delta = st.Distance()
	hx := st.Pos.X - sunSt.Pos.X
	hy := st.Pos.Y - sunSt.Pos.Y
	hz := st.Pos.Z - sunSt.Pos.Z
	r = math.Sqrt(hx*hx + hy*hy + hz*hz)
	sunDist := sunSt.Distance()

	cosA := (r*r + delta*delta - sunDist*sunDist) / (2 * r * delta)
	cosA = clamp(cosA, -1, 1)
	alpha = angle.Rad(math.Acos(cosA))

	return r, delta, alpha, st, nil
}
