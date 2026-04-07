package simbad

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/provider"
	"github.com/TuSKan/astrogo/coord"
)

// ParseCSV parses SIMBAD's TAP output in CSV format into provider.Targets.
// The expected order from BuildResolveQuery is:
// oid, main_id, ra, dec, otype, id (matched alias)
func ParseCSV(r io.Reader) ([]provider.Target, error) {
	reader := csv.NewReader(r)

	// Read header and build column index map
	header, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, fmt.Errorf("simbad: failed to read CSV header: %w", err)
	}

	colIdx := make(map[string]int)
	for i, h := range header {
		colIdx[h] = i
	}

	// Validate presence of minimal required columns
	required := []string{"main_id", "ra", "dec"}
	for _, req := range required {
		if _, ok := colIdx[req]; !ok {
			return nil, fmt.Errorf("simbad: internal error, missing expected column %q", req)
		}
	}

	// Map to hold unique targets because joining with ident table can return
	// multiple rows for the same basic.oid (one row per alias match).
	targetMap := make(map[string]*provider.Target)

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("simbad: failed to read CSV row: %w", err)
		}

		mainID := row[colIdx["main_id"]]

		if existing, ok := targetMap[mainID]; ok {
			// Just append the alias if it's new
			if aliasIdx, exists := colIdx["id"]; exists {
				alias := row[aliasIdx]
				if alias != "" {
					existing.Aliases = append(existing.Aliases, alias)
				}
			}
			continue
		}

		raStr := row[colIdx["ra"]]
		decStr := row[colIdx["dec"]]

		var c *coord.ICRS
		if raStr != "" && decStr != "" {
			raDeg, errRA := strconv.ParseFloat(raStr, 64)
			decDeg, errDec := strconv.ParseFloat(decStr, 64)
			if errRA == nil && errDec == nil {
				c = coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg))
			}
		}

		otype := provider.KindOther
		if tIdx, ok := colIdx["otype"]; ok {
			otype = mapSimbadKind(row[tIdx])
		}

		t := provider.Target{
			ID:      mainID,
			Name:    mainID,
			Kind:    otype,
			Coord:   c,
			Catalog: "SIMBAD",
		}

		if aliasIdx, exists := colIdx["id"]; exists {
			alias := row[aliasIdx]
			if alias != "" {
				t.Aliases = append(t.Aliases, alias)
			}
		}

		targetMap[mainID] = &t
	}

	var results []provider.Target
	for _, t := range targetMap {
		results = append(results, *t)
	}

	return results, nil
}

// mapSimbadKind maps common SIMBAD Object Types (OTypes) to astrogo internal kinds.
func mapSimbadKind(o string) provider.Kind {
	switch o {
	case "Star", "V*", "Em*":
		return provider.KindStar
	case "GlC", "OpC", "Cl*":
		return provider.KindStarCluster // or Globular/Open specifically inside logic
	case "PN", "HII", "Neb":
		return provider.KindNebula
	case "G", "Gal", "AGN":
		return provider.KindGalaxy
	case "SNR":
		return provider.KindSupernovaRemnant
	default:
		return provider.KindOther
	}
}
