package openngc

import (
	"bytes"
	"embed"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

//go:generate go run ./parser/parse.go data/openngc.csv https://raw.githubusercontent.com/mattiaverga/OpenNGC/master/database_files/NGC.csv https://raw.githubusercontent.com/mattiaverga/OpenNGC/master/database_files/addendum.csv

//go:embed data/*
var catalogFS embed.FS

var data []byte

func init() { //nolint:gochecknoinits // loads embedded catalog data
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

// New creates a new OpenNGC catalog provider from embedded data.
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

// Resolve performs exact-match resolution for a query.
func (p *Provider) Resolve(query string) (resolve.Target, bool) {
	q := resolve.Normalize(query)
	if idx, ok := p.byKey[q]; ok {
		return p.targets[idx], true
	}

	return resolve.Target{}, false
}

// Search performs fuzzy search across all NGC/IC objects.
func (p *Provider) Search(query string) []resolve.Target {
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

func parseCSV(data []byte) ([]resolve.Target, error) {
	r := csv.NewReader(bytes.NewReader(data))

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("openngc: read header: %w", err)
	}

	// Build column index map for forward-compatible parsing.
	col := make(map[string]int)
	for i, h := range header {
		col[h] = i
	}

	var targets []resolve.Target

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("openngc: read row: %w", err)
		}

		if len(row) < 6 {
			continue
		}

		id, name, kindStr, raStr, decStr, aliasesStr := row[0], row[1], row[2], row[3], row[4], row[5]

		raDeg, _ := strconv.ParseFloat(raStr, 64)
		decDeg, _ := strconv.ParseFloat(decStr, 64)

		var kind resolve.Kind

		switch kindStr {
		case "Galaxy":
			kind = resolve.KindGalaxy
		case "Nebula":
			kind = resolve.KindNebula
		case "OpenCluster":
			kind = resolve.KindOpenCluster
		case "GlobularCluster":
			kind = resolve.KindGlobularCluster
		case "Star":
			kind = resolve.KindStar
		case "Asterism":
			kind = resolve.KindAsterism
		default:
			kind = resolve.KindOther
		}

		var aliases []string
		if aliasesStr != "" {
			aliases = strings.Split(aliasesStr, ";")
		}

		t := resolve.Target{
			ID:       id,
			Name:     name,
			Kind:     kind,
			Coord:    coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg)),
			HasCoord: true,
			Catalog:  "openngc",
			Aliases:  aliases,
		}

		// V-band magnitude (column present in generated CSV from parser v2+).
		if idx, ok := col["vmag"]; ok && idx < len(row) && row[idx] != "" {
			if v, err := strconv.ParseFloat(row[idx], 64); err == nil {
				t.VMag = v
				t.HasVMag = true
			}
		}

		targets = append(targets, t)
	}

	return targets, nil
}
