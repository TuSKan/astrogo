package resolve

import (
	"context"
	"iter"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// Capability describes what a remote catalog can do.
type Capability string

const (
	// CapObjectResolution indicates that the provider can resolve object names or IDs.
	CapObjectResolution Capability = "ObjectResolution"
	// CapConeSearch indicates that the provider can perform cone searches.
	CapConeSearch Capability = "ConeSearch"
	// CapFullCatalog indicates that the provider can provide full catalog data.
	CapFullCatalog Capability = "FullCatalog"
)

// ObjectRequest represents a request to resolve a specific object name or ID.
type ObjectRequest struct {
	// ID is the unique identifier of the target.
	ID string
	// Query is the name or identifier of the target to resolve.
	Query string
	// Limit is the maximum number of results to return.
	Limit int
}

// ConeRequest represents a spatial query around a specific coordinate.
type ConeRequest struct {
	// ID is the unique identifier of the target.
	ID string
	// Table selects which catalog table a ConeSearcher queries, for
	// providers that support more than one (e.g. catalog/vizier). The
	// empty string means "use the provider's default table" — existing
	// callers that never set this field keep their current behavior
	// unchanged. Providers that don't support table selection ignore this
	// field entirely.
	Table string
	// Center is the coordinate to search around.
	Center coord.ICRS
	// Radius is the search radius.
	Radius angle.Angle
	// Limit is the maximum number of results to return.
	Limit int
}

// ObjectResolver is an advanced remote catalog provider that handles
// asynchronous, cancellable requests natively.
type ObjectResolver interface {
	// Capabilities returns the capabilities of the catalog provider.
	Capabilities() []Capability
	// ResolveObject resolves an object by name or identifier.
	ResolveObject(ctx context.Context, req ObjectRequest) SeqIterator[Target]
}

// ConeSearcher allows radial spatial queries against standard coordinate spaces.
type ConeSearcher interface {
	// Capabilities returns the capabilities of the catalog provider.
	Capabilities() []Capability
	// ConeSearch searches for targets within a given radius of a center coordinate.
	ConeSearch(ctx context.Context, req ConeRequest) SeqIterator[Target]
}

// SeqIterator is an alias for iter.Seq2 for explicit documentation of expected return type.
type SeqIterator[T any] iter.Seq2[T, error]

// SliceSeq converts an in-memory slice to a standard SeqIterator.
func SliceSeq[T any](items []T) SeqIterator[T] {
	return func(yield func(T, error) bool) {
		for _, v := range items {
			if !yield(v, nil) {
				return
			}
		}
	}
}
