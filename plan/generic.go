package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// GenericBody represents a moving body with an ephemeris provider but no
// specific photometric model. Unlike [Planet], it does NOT implement
// [MagnitudeComputer], so GetDetails will not populate a (wrong) magnitude.
//
// This is the fallback type used by [FromCatalog] when a provider is present
// but the body is not a recognized planet, comet, or asteroid.
type GenericBody struct {
	provider eph.Provider
	name     string
	id       eph.ID
}

// NewGenericBody creates a generic moving-body target.
func NewGenericBody(name string, id eph.ID, provider eph.Provider) *GenericBody {
	return &GenericBody{name: name, id: id, provider: provider}
}

// Name returns the body's display name.
func (g *GenericBody) Name() string { return g.name }

// Provider returns the ephemeris provider.
func (g *GenericBody) Provider() eph.Provider { return g.provider }

// EphID returns the NAIF ephemeris identifier.
func (g *GenericBody) EphID() eph.ID { return g.id }

// Position returns the geometric ICRS direction at the given time — see
// [apparentVec] for why this is not the apparent place that
// [GenericBody.GeocentricVec] gives.
func (g *GenericBody) Position(t time.Time) (coord.ICRS, error) {
	icrs, err := geometricICRS(g.provider, g.id, t)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("generic body %s: %w", g.name, err)
	}

	return icrs, nil
}

// GeocentricVec returns the apparent geocentric position vector — see
// [apparentVec].
func (g *GenericBody) GeocentricVec(t time.Time) (vector.Vec3, error) {
	v, err := apparentVec(g.provider, g.id, t)
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("generic body %s: %w", g.name, err)
	}

	return v, nil
}

// GetDetails returns the target details.
func (g *GenericBody) GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error) {
	return computeDetails(g, ctx, props...)
}
