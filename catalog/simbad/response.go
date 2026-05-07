package simbad

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// ParseCSV parses SIMBAD's TAP output in CSV format into resolve.Targets.
// The expected order from BuildResolveQuery is:
// oid, main_id, ra, dec, otype, id (matched alias)
func ParseCSV(r io.Reader) ([]resolve.Target, error) {
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
	targetMap := make(map[string]*resolve.Target)

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

		var c coord.ICRS
		hasCoord := false
		if raStr != "" && decStr != "" {
			raDeg, errRA := strconv.ParseFloat(raStr, 64)
			decDeg, errDec := strconv.ParseFloat(decStr, 64)
			if errRA == nil && errDec == nil {
				c = coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg))
				hasCoord = true
			}
		}

		otype := resolve.KindOther
		if tIdx, ok := colIdx["otype"]; ok {
			otype = mapSimbadKind(row[tIdx])
		}

		t := resolve.Target{
			ID:       mainID,
			Name:     mainID,
			Kind:     otype,
			Coord:    c,
			HasCoord: hasCoord,
			Catalog:  "SIMBAD",
		}

		if hasCoord {
			t.Epoch = time.FromJD(2451545.0, time.UTC) // Default SIMBAD Epoch (J2000)

			if pmRAStr, ok := colIdx["pmra"]; ok && row[pmRAStr] != "" {
				if v, err := strconv.ParseFloat(row[pmRAStr], 64); err == nil {
					t.PmRA = angle.Arcsec(v / 1000.0)
				}
			}
			if pmDecStr, ok := colIdx["pmdec"]; ok && row[pmDecStr] != "" {
				if v, err := strconv.ParseFloat(row[pmDecStr], 64); err == nil {
					t.PmDec = angle.Arcsec(v / 1000.0)
				}
			}
			if plxStr, ok := colIdx["plx_value"]; ok && row[plxStr] != "" {
				if v, err := strconv.ParseFloat(row[plxStr], 64); err == nil {
					t.Parallax = angle.Arcsec(v / 1000.0)
				}
			}
			if rvStr, ok := colIdx["rvz_radvel"]; ok && row[rvStr] != "" {
				if v, err := strconv.ParseFloat(row[rvStr], 64); err == nil {
					t.RadialVelocity = v
				}
			}
			// V-band magnitude from allfluxes table.
			if vmagIdx, ok := colIdx["v"]; ok && row[vmagIdx] != "" {
				if v, err := strconv.ParseFloat(row[vmagIdx], 64); err == nil {
					t.VMag = v
					t.HasVMag = true
				}
			}
		}

		if aliasIdx, exists := colIdx["id"]; exists {
			alias := row[aliasIdx]
			if alias != "" {
				t.Aliases = append(t.Aliases, alias)
			}
		}

		targetMap[mainID] = &t
	}

	var results []resolve.Target
	for _, t := range targetMap {
		results = append(results, *t)
	}

	return results, nil
}

// mapSimbadKind maps common SIMBAD Object Types (OTypes) to astrogo internal kinds.
func mapSimbadKind(o string) resolve.Kind {
	switch o {
	case "Star", "V*", "Em*":
		return resolve.KindStar
	case "GlC", "OpC", "Cl*":
		return resolve.KindStarCluster // or Globular/Open specifically inside logic
	case "PN", "HII", "Neb":
		return resolve.KindNebula
	case "G", "Gal", "AGN":
		return resolve.KindGalaxy
	case "SNR":
		return resolve.KindSupernovaRemnant
	default:
		return resolve.KindOther
	}
}
