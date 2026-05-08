// Package catalog provides a lightweight astronomical object catalog system.
package catalog

import (
	"errors"
	"sort"

	"github.com/TuSKan/astrogo/catalog/gaia"
	"github.com/TuSKan/astrogo/catalog/jpl"
	"github.com/TuSKan/astrogo/catalog/mast"
	"github.com/TuSKan/astrogo/catalog/norad"
	"github.com/TuSKan/astrogo/catalog/openngc"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/catalog/sbdb"
	"github.com/TuSKan/astrogo/catalog/simbad"
	"github.com/TuSKan/astrogo/catalog/vizier"
)

// Source represents an astronomical data provider type.
type Source int

const (
	OpenNGC Source = iota
	SIMBAD
	MAST
	JPL
	SBDB
	Gaia
	VizieR
	NORAD
)

var (
	ErrNotFound  = errors.New("target not found")
	ErrAmbiguous = errors.New("ambiguous target name")
)

// Export core types directly via Type Aliasing to break cyclic dependencies natively.
type Target = resolve.Target
type Provider = resolve.Provider
type Kind = resolve.Kind
type ObjectRequest = resolve.ObjectRequest
type SeqIterator[T any] = resolve.SeqIterator[T]

// Resolver orchestrates multiple providers to find astronomical targets.
type Resolver struct {
	providers []Provider
}

// NewResolver instantiates remote and local catalog implementations securely.
func NewResolver(sources ...Source) *Resolver {
	var providers []Provider
	for _, src := range sources {
		switch src {
		case OpenNGC:
			providers = append(providers, openngc.New())
		case SIMBAD:
			providers = append(providers, simbad.New())
		case MAST:
			providers = append(providers, mast.New())
		case JPL:
			providers = append(providers, jpl.New())
		case SBDB:
			providers = append(providers, sbdb.New())
		case Gaia:
			providers = append(providers, gaia.New())
		case VizieR:
			providers = append(providers, vizier.New())
		case NORAD:
			providers = append(providers, norad.New())
		}
	}
	return &Resolver{providers: providers}
}

func (r *Resolver) Resolve(query string) (Target, error) {
	q := resolve.Normalize(query)
	if q == "" {
		return Target{}, ErrNotFound
	}

	// First-match-wins: providers are tried in priority order.
	// If a provider resolves the query, return immediately.
	for _, p := range r.providers {
		if t, ok := p.Resolve(query); ok {
			return t, nil
		}
	}

	// Fall back to fuzzy search across all providers.
	results := r.Search(query)
	if len(results) > 0 {
		return results[0], nil
	}

	return Target{}, ErrNotFound
}

func (r *Resolver) Search(query string) []Target {
	q := resolve.Normalize(query)
	if q == "" {
		return nil
	}

	var all []Target
	for _, p := range r.providers {
		all = append(all, p.Search(query)...)
	}

	unique := make([]Target, 0, len(all))
	seen := make(map[string]bool)
	for _, t := range all {
		key := t.Catalog + ":" + t.ID
		if !seen[key] {
			seen[key] = true
			unique = append(unique, t)
		}
	}

	type scoredTarget struct {
		t     Target
		score float64
	}
	scored := make([]scoredTarget, len(unique))
	for i, t := range unique {
		bestScore := resolve.Score(q, t.Name)
		for _, alias := range t.Aliases {
			if s := resolve.Score(q, alias); s > bestScore {
				bestScore = s
			}
		}
		if s := resolve.Score(q, t.ID); s > bestScore {
			bestScore = s
		}
		scored[i] = scoredTarget{t, bestScore}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	limit := 10
	if len(scored) < limit {
		limit = len(scored)
	}

	final := make([]Target, limit)
	for i := 0; i < limit; i++ {
		final[i] = scored[i].t
	}

	return final
}
