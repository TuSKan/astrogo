package openngc

import (
	"bytes"
	"embed"
	"encoding/csv"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/provider"
	"github.com/TuSKan/astrogo/coord"
)

//go:generate go run ./parser/parse.go data/openngc.csv https://raw.githubusercontent.com/mattiaverga/OpenNGC/master/database_files/NGC.csv https://raw.githubusercontent.com/mattiaverga/OpenNGC/master/database_files/addendum.csv

//go:embed data/*
var catalogFS embed.FS

var data []byte

func init() {
	// Dynamically attempt to load the catalog buffer if the user generated it locally.
	// Bypasses the strict compiler block when ignored in CI.
	d, err := catalogFS.ReadFile("data/openngc.csv")
	if err == nil {
		data = d
	}
}

// Record represents a raw entry in the OpenNGC dataset.
type Record struct {
	ID      string
	Name    string
	Kind    provider.Kind
	RA      string
	Dec     string
	Aliases []string
}

// Provider implements the provider.Provider interface for OpenNGC.
type Provider struct {
	targets []provider.Target
	byKey   map[string]int
}

func New() *Provider {
	targets, err := parseCSV(data)
	if err != nil {
		log.Printf("openngc: failed to parse embedded data: %v", err)
		return &Provider{byKey: make(map[string]int)}
	}
	p := &Provider{
		targets: targets,
		byKey:   make(map[string]int),
	}
	for i, t := range targets {
		p.byKey[provider.Normalize(t.ID)] = i
		if t.Name != "" {
			p.byKey[provider.Normalize(t.Name)] = i
		}
		for _, a := range t.Aliases {
			p.byKey[provider.Normalize(a)] = i
		}
	}
	return p
}

func (p *Provider) Name() string { return "openngc" }

func (p *Provider) Resolve(query string) (provider.Target, bool) {
	q := provider.Normalize(query)
	if idx, ok := p.byKey[q]; ok {
		return p.targets[idx], true
	}
	return provider.Target{}, false
}

func (p *Provider) Search(query string) []provider.Target {
	q := provider.Normalize(query)
	if q == "" {
		return nil
	}
	var results []provider.Target
	for _, t := range p.targets {
		if strings.Contains(provider.Normalize(t.Name), q) ||
			strings.Contains(provider.Normalize(t.ID), q) {
			results = append(results, t)
			continue
		}
		for _, a := range t.Aliases {
			if strings.Contains(provider.Normalize(a), q) {
				results = append(results, t)
				break
			}
		}
	}
	return results
}

func parseCSV(data []byte) ([]provider.Target, error) {
	r := csv.NewReader(bytes.NewReader(data))
	if _, err := r.Read(); err != nil {
		return nil, err
	}
	var targets []provider.Target
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(row) < 6 {
			continue
		}
		id, name, kindStr, raStr, decStr, aliasesStr := row[0], row[1], row[2], row[3], row[4], row[5]

		raDeg, _ := strconv.ParseFloat(raStr, 64)
		decDeg, _ := strconv.ParseFloat(decStr, 64)

		var kind provider.Kind
		switch kindStr {
		case "Galaxy":
			kind = provider.KindGalaxy
		case "Nebula":
			kind = provider.KindNebula
		case "OpenCluster":
			kind = provider.KindOpenCluster
		case "GlobularCluster":
			kind = provider.KindGlobularCluster
		case "Star":
			kind = provider.KindStar
		case "Asterism":
			kind = provider.KindAsterism
		default:
			kind = provider.KindOther
		}
		var aliases []string
		if aliasesStr != "" {
			aliases = strings.Split(aliasesStr, ";")
		}
		targets = append(targets, provider.Target{
			ID:      id,
			Name:    name,
			Kind:    kind,
			Coord:   coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg)),
			Catalog: "openngc",
			Aliases: aliases,
		})
	}
	return targets, nil
}
