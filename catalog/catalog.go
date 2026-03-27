// Package catalog provides a lightweight astronomical object catalog system.
package catalog

import (
	"errors"
	"sort"
	"strings"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Kind represents the type of an astronomical object.
type Kind string

const (
	KindStar             Kind = "Star"
	KindPlanet           Kind = "Planet"
	KindMoon             Kind = "Moon"
	KindGalaxy           Kind = "Galaxy"
	KindNebula           Kind = "Nebula"
	KindStarCluster      Kind = "StarCluster"
	KindOpenCluster      Kind = "OpenCluster"
	KindGlobularCluster  Kind = "GlobularCluster"
	KindSupernovaRemnant Kind = "SupernovaRemnant"
	KindAsterism         Kind = "Asterism"
	KindDoubleStar       Kind = "DoubleStar"
	KindOther            Kind = "Other"
)

// Target represents an astronomical object in a catalog.
type Target struct {
	ID          string
	Name        string
	Designation string
	SPKID       string
	Kind        Kind
	Coord       coord.ICRS
	Catalog     string
	Aliases     []string
}

// ICRS implements sky.Object for a static catalog Target.
func (t Target) ICRS(_ time.Time) (coord.ICRS, error) {
	return t.Coord, nil
}

// Provider defines the interface for astronomical catalogs.
type Provider interface {
	Name() string
	Resolve(query string) (Target, bool)
	Search(query string) []Target
}

var (
	ErrNotFound  = errors.New("target not found")
	ErrAmbiguous = errors.New("ambiguous target name")
)

// Normalize converts a query to a canonical form for matching.
func Normalize(query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	q = strings.ReplaceAll(q, " ", "")
	if strings.HasPrefix(q, "messier") {
		q = "m" + q[7:]
	}
	return q
}

// Resolver orchestrates multiple providers to find astronomical targets.
type Resolver struct {
	providers []Provider
}

func NewResolver(providers ...Provider) *Resolver {
	return &Resolver{providers: providers}
}

func (r *Resolver) Resolve(query string) (Target, error) {
	q := Normalize(query)
	if q == "" {
		return Target{}, ErrNotFound
	}

	var matches []Target
	for _, p := range r.providers {
		if t, ok := p.Resolve(q); ok {
			matches = append(matches, t)
		}
	}

	if len(matches) > 1 {
		return Target{}, ErrAmbiguous
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	results := r.Search(query)
	if len(results) > 0 {
		return results[0], nil
	}

	return Target{}, ErrNotFound
}

func (r *Resolver) Search(query string) []Target {
	q := Normalize(query)
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
		bestScore := score(q, t.Name)
		for _, alias := range t.Aliases {
			if s := score(q, alias); s > bestScore {
				bestScore = s
			}
		}
		if s := score(q, t.ID); s > bestScore {
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

func score(query, candidate string) float64 {
	if query == "" || candidate == "" {
		return 0
	}
	c := Normalize(candidate)
	if query == c {
		return 1.0
	}
	if strings.HasPrefix(c, query) {
		return 0.8
	}
	if strings.Contains(c, query) {
		return 0.5
	}
	dist := levenshtein(query, c)
	maxLen := len(query)
	if len(c) > maxLen {
		maxLen = len(c)
	}
	lScore := 1.0 - float64(dist)/float64(maxLen)
	if lScore < 0 {
		lScore = 0
	}
	return lScore * 0.3
}

func levenshtein(s, t string) int {
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for j := 1; j <= len(t); j++ {
		for i := 1; i <= len(s); i++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, min(d[i][j-1]+1, d[i-1][j-1]+cost))
		}
	}
	return d[len(s)][len(t)]
}
