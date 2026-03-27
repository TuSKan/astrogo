package catalog

import (
	"strings"

	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/coord"
)

// BuiltinProvider provides Solar System bodies.
type BuiltinProvider struct {
	targets []Target
	byKey   map[string]int
}

// NewBuiltinProvider creates a new provider with Sun, Moon, and planets.
func NewBuiltinProvider() *BuiltinProvider {
	bp := &BuiltinProvider{
		byKey: make(map[string]int),
	}

	for _, b := range body.Bodies {
		kind := KindOther
		switch b.Kind {
		case body.KindStar:
			kind = KindStar
		case body.KindPlanet:
			kind = KindPlanet
		case body.KindMoon:
			kind = KindMoon
		}

		t := Target{
			ID:      strings.ToLower(b.Name),
			Name:    b.Name,
			Kind:    kind,
			Catalog: "builtin",
			Coord:   coord.ICRS{},
		}

		idx := len(bp.targets)
		bp.targets = append(bp.targets, t)
		bp.byKey[Normalize(t.ID)] = idx
		bp.byKey[Normalize(t.Name)] = idx
	}

	return bp
}

func (p *BuiltinProvider) Name() string { return "builtin" }

func (p *BuiltinProvider) Resolve(query string) (Target, bool) {
	q := Normalize(query)
	if idx, ok := p.byKey[q]; ok {
		return p.targets[idx], true
	}
	return Target{}, false
}

func (p *BuiltinProvider) Search(query string) []Target {
	q := Normalize(query)
	if q == "" {
		return nil
	}
	var results []Target
	for _, t := range p.targets {
		if score(q, t.Name) > 0.4 || score(q, t.ID) > 0.4 {
			results = append(results, t)
		}
	}
	return results
}
