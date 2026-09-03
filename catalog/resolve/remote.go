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
	// CapMagnitudeBrowse indicates that the provider can enumerate objects
	// brighter than a magnitude bound, independent of name or position.
	CapMagnitudeBrowse Capability = "MagnitudeBrowse"
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

// BrightRequest bounds a magnitude-filtered bulk query — the counterpart to
// ObjectRequest (name-driven) and ConeRequest (position-driven): neither a
// name nor a position, just "everything brighter than this."
type BrightRequest struct {
	// MaxVMag is the magnitude bound: only objects brighter than (numerically
	// less than) this value are returned.
	MaxVMag float64
	// Limit is the maximum number of results to return.
	Limit int
}

// BrightObjectSearcher lets a provider bulk-list every object it knows about
// brighter than a magnitude bound, independent of name or position.
type BrightObjectSearcher interface {
	// Capabilities returns the capabilities of the catalog provider.
	Capabilities() []Capability
	// SearchBright returns every object brighter than req.MaxVMag.
	SearchBright(ctx context.Context, req BrightRequest) SeqIterator[Target]
}

// SeqIterator is an alias for iter.Seq2 for explicit documentation of expected return type.
type SeqIterator[T any] iter.Seq2[T, error]

// Drain collects a streaming iterator into a slice, stopping once limit items
// have been taken, and returns the first error the iterator yields.
//
// # Why this exists as one function
//
// Four providers — simbad, jpl, mast and sbdb — each carried their own copy of
// this loop, and every copy discarded the error. Three wrote
//
//	if err == nil {
//		targets = append(targets, t)
//	}
//
// which drops a transport failure without even a log line, and the fourth
// logged it to the global log package before dropping it. The caller then saw
// an empty slice and reported "not found". One shape, four copies, one defect.
//
// The streaming iterators themselves were always correct; it was this drain
// step that lost the error. Centralising it means the next provider inherits
// the fix rather than the bug.
//
// # Partial results are discarded with the error
//
// Rows taken before the failure are not returned alongside it. A short answer
// that looks complete is exactly how the original defect produced wrong
// results instead of visible ones — a caller checking only len() would treat
// a truncated page as the whole catalog.
//
// A limit of zero or less collects everything the iterator offers.
func Drain[T any](seq SeqIterator[T], limit int) ([]T, error) {
	var (
		out     []T
		failure error
	)

	seq(func(v T, err error) bool {
		if err != nil {
			failure = err

			return false
		}

		out = append(out, v)

		return limit <= 0 || len(out) < limit
	})

	if failure != nil {
		return nil, failure
	}

	return out, nil
}

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
