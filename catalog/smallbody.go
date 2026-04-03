package catalog

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/TuSKan/astrogo/coord"
)

// DOC: https://ssd-api.jpl.nasa.gov/doc/sbdb_query.html
// API: https://ssd-api.jpl.nasa.gov/sbdb_query.api

var sbdbQueryAPI = "https://ssd-api.jpl.nasa.gov/sbdb_query.api"

// Orbit Class Codes
// Code	Name	Kind	Description
// IEO	Atira	asteroid	An asteroid orbit contained entirely within the orbit of the Earth (Q < 0.983 au). Also known as an Interior Earth Object
// ATE	Aten	asteroid	Near-Earth asteroid orbits similar to that of 2062 Aten (a < 1.0 au; Q > 0.983 au)
// APO	Apollo	asteroid	Near-Earth asteroid orbits which cross the Earth’s orbit similar to that of 1862 Apollo (a > 1.0 au; q < 1.017 au)
// AMO	Amor	asteroid	Near-Earth asteroid orbits similar to that of 1221 Amor (1.017 au < q < 1.3 au)
// MCA	Mars-crossing Asteroid	asteroid	Asteroids that cross the orbit of Mars constrained by (1.3 au < q < 1.666 au; a < 3.2 au)
// IMB	Inner Main-belt Asteroid	asteroid	Asteroids with orbital elements constrained by (a < 2.0 au; q > 1.666 au)
// MBA	Main-belt Asteroid	asteroid	Asteroids with orbital elements constrained by (2.0 au < a < 3.2 au; q > 1.666 au)
// OMB	Outer Main-belt Asteroid	asteroid	Asteroids with orbital elements constrained by (3.2 au < a < 4.6 au)
// TJN	Jupiter Trojan	asteroid	Asteroids trapped in Jupiter’s L4/L5 Lagrange points (4.6 au < a < 5.5 au; e < 0.3)
// AST	Asteroid	asteroid	Asteroid orbit not matching any defined orbit class
// CEN	Centaur	 	Objects with orbits between Jupiter and Neptune (5.5 au < a < 30.1 au)
// TNO	TransNeptunian Object	 	Objects with orbits outside Neptune (a > 30.1 au)
// PAA	Parabolic “Asteroid”	 	“Asteroids” (objects other than comets) on parabolic orbits (e = 1.0)
// HYA	Hyperbolic “Asteroid”	 	“Asteroids” (objects other than comets) on hyperbolic orbits (e > 1.0)
// ETc	Encke-type Comet	comet	Encke-type comet, as defined by Levison and Duncan (Tj > 3; a < aJ)
// JFc	Jupiter-family Comet	comet	Jupiter-family comet, as defined by Levison and Duncan (2 < Tj < 3)
// JFC	Jupiter-family Comet*	comet	Jupiter-family comet, classical definition (P < 20 y)
// CTc	Chiron-type Comet	comet	Chiron-type comet, as defined by Levison and Duncan (Tj > 3; a > aJ)
// HTC	Halley-type Comet*	comet	Halley-type comet, classical definition (20 y < P < 200 y)
// PAR	Parabolic Comet	comet	Comets on parabolic orbits (e = 1.0)
// HYP	Hyperbolic Comet	comet	Comets on hyperbolic orbits (e > 1.0)
// COM	Comet	comet	Comet orbit not matching any defined orbit class

// Orbit class descriptions above use the following symbols to indicate orbit parameters:
// e - eccentricity
// q - perihelion distance
// a - semimajor axis
// Q - aphelion distance
// P - orbital period
// Tj - Jupiter Tisserand parameter
// aJ - Jupiter nominal semimajor axis

// SBDBQuery represents a query to the JPL Small-Body Database.
type SBDBQuery struct {
	Kind      string // Limit results to either asteroids-only (a) or comets-only (c)
	Group     string // Limit results to NEOs-only (neo) or PHAs-only (pha)
	Class     string // Limit results to small-bodies with orbits of the specified class (or classes). Allowable values are valid 3-character orbit-class codes. If specifying more than one class, separate entities with a comma (e.g., TJN,CEN). Codes are case-sensitive.
	Satelite  string // Limit results to small-bodies with at least one known satellite. true or 1, false or 0
	Fragments string // Exclude all comet fragments (if any) from results. true or 1, false or 0
	Custom    string // Custom field constraints. Maximum length is 2048 characters.
}

// SmallBodyProvider provides metadata and indexing for asteroids and comets.
type SmallBodyProvider struct {
	targets []Target
	byKey   map[string]int
}

// NewSmallBodyProvider creates a provider from a JPL SBDB Query API CSV reader.
// Expected columns (minimal): full_name, pdes, name, spkid, kind
func NewSmallBodyProvider(query SBDBQuery) (*SmallBodyProvider, error) {
	api, err := url.Parse(sbdbQueryAPI)
	if err != nil {
		return nil, fmt.Errorf("catalog: failed to parse SBDB URL: %w", err)
	}
	params := api.Query()

	if query.Kind != "" {
		params.Set("sb-kind", query.Kind)
	}
	if query.Group != "" {
		params.Set("sb-group", query.Group)
	}
	if query.Class != "" {
		params.Set("sb-class", query.Class)
	}
	if query.Satelite != "" {
		params.Set("sb-sat", query.Satelite)
	}
	if query.Fragments != "" {
		params.Set("sb-xfrag", query.Fragments)
	}
	if query.Custom != "" {
		params.Set("sb-cdata", query.Custom)
	}
	api.RawQuery = params.Encode()

	r, err := http.Get(api.String())
	if err != nil {
		return nil, fmt.Errorf("catalog: failed to query SBDB: %w", err)
	}
	defer r.Body.Close()

	reader := csv.NewReader(r.Body)
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("catalog: failed to read SBDB header: %w", err)
	}

	col := make(map[string]int)
	for i, h := range header {
		col[h] = i
	}

	// Required columns check
	required := []string{"full_name", "spkid"}
	for _, req := range required {
		if _, ok := col[req]; !ok {
			return nil, fmt.Errorf("catalog: missing required column %q in SBDB CSV", req)
		}
	}

	p := &SmallBodyProvider{
		byKey: make(map[string]int),
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("catalog: failed to read SBDB row: %w", err)
		}

		id := row[col["spkid"]]
		fullName := row[col["full_name"]]
		spkid := row[col["spkid"]]

		// Optional columns
		name := ""
		if idx, ok := col["name"]; ok {
			name = row[idx]
		}
		pdes := ""
		if idx, ok := col["pdes"]; ok {
			pdes = row[idx]
		}
		kindStr := "Asteroid"
		if idx, ok := col["kind"]; ok {
			kindStr = row[idx]
		}

		t := Target{
			ID:          id,
			Name:        fullName,
			Designation: pdes,
			SPKID:       spkid,
			Kind:        Kind(kindStr),
			Catalog:     "sbdb",
			Coord:       coord.ICRS{}, // Small bodies are dynamic, no fixed ICRS
		}

		idx := len(p.targets)
		p.targets = append(p.targets, t)

		// Indexing
		p.index(Normalize(t.ID), idx)
		p.index(Normalize(t.Name), idx)
		if t.Designation != "" {
			p.index(Normalize(t.Designation), idx)
		}
		if t.SPKID != "" {
			p.index(Normalize(t.SPKID), idx)
		}
		if name != "" {
			p.index(Normalize(name), idx)
		}
	}

	return p, nil
}

func (p *SmallBodyProvider) index(key string, idx int) {
	if _, ok := p.byKey[key]; !ok {
		p.byKey[key] = idx
	}
}

func (p *SmallBodyProvider) Name() string { return "sbdb" }

func (p *SmallBodyProvider) Resolve(query string) (Target, bool) {
	q := Normalize(query)
	if idx, ok := p.byKey[q]; ok {
		return p.targets[idx], true
	}
	return Target{}, false
}

func (p *SmallBodyProvider) Search(query string) []Target {
	q := Normalize(query)
	if q == "" {
		return nil
	}
	var results []Target
	seen := make(map[string]bool)
	for _, t := range p.targets {
		if seen[t.ID] {
			continue
		}
		// Match against name, designation, SPKID
		if strings.Contains(Normalize(t.Name), q) ||
			strings.Contains(Normalize(t.Designation), q) ||
			strings.Contains(Normalize(t.SPKID), q) {
			results = append(results, t)
			seen[t.ID] = true
			if len(results) >= 10 {
				break
			}
		}
	}
	return results
}
