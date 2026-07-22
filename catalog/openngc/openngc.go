package openngc

import (
	"context"
	"log"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// Record represents a raw entry in the OpenNGC dataset.
type Record struct {
	ID      string
	Name    string
	Kind    resolve.Kind
	RA      string
	Dec     string
	Aliases []string
}

// Provider implements the resolve.Provider interface for OpenNGC.
type Provider struct {
	byKey   map[string]int
	targets []resolve.Target
}

// New creates a new OpenNGC catalog provider — like every other astrogo
// catalog provider, it does its own network access rather than reading
// build-time embedded data. It fetches and merges the two upstream source
// CSVs if remote.EnableDownloads(remote.OpenNGC, ...) has been called,
// reusing a local cache untouched when a HEAD probe shows nothing changed
// upstream. If downloads aren't enabled, or the fetch fails for any other
// reason, New returns an empty, warning-logged provider — the same
// degraded behavior as any other catalog provider whose backing source is
// unreachable.
func New() *Provider {
	targets, err := fetch(context.Background())
	if err != nil {
		log.Printf("openngc: %v", err)
		return &Provider{byKey: make(map[string]int)}
	}

	p := &Provider{
		targets: targets,
		byKey:   make(map[string]int),
	}
	for i, t := range targets {
		p.byKey[resolve.Normalize(t.ID)] = i
		if t.Name != "" {
			p.byKey[resolve.Normalize(t.Name)] = i
		}

		for _, a := range t.Aliases {
			p.byKey[resolve.Normalize(a)] = i
		}
	}

	return p
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "openngc" }

// Resolve performs exact-match resolution for a query. ctx is accepted for
// resolve.Provider conformance only — resolution runs over the in-memory
// index built once at New(), with no I/O to cancel.
func (p *Provider) Resolve(_ context.Context, query string) (resolve.Target, bool) {
	q := resolve.Normalize(query)
	if idx, ok := p.byKey[q]; ok {
		return p.targets[idx], true
	}

	return resolve.Target{}, false
}

// Search performs fuzzy search across all NGC/IC objects. ctx is accepted
// for resolve.Provider conformance only — see Resolve.
func (p *Provider) Search(_ context.Context, query string) []resolve.Target {
	q := resolve.Normalize(query)
	if q == "" {
		return nil
	}

	var results []resolve.Target

	for _, t := range p.targets {
		if strings.Contains(resolve.Normalize(t.Name), q) ||
			strings.Contains(resolve.Normalize(t.ID), q) {
			results = append(results, t)
			continue
		}

		for _, a := range t.Aliases {
			if strings.Contains(resolve.Normalize(a), q) {
				results = append(results, t)
				break
			}
		}
	}

	return results
}
