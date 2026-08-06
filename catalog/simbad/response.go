package simbad

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// ErrMissingColumn indicates a required column is missing from the SIMBAD response.
var ErrMissingColumn = errors.New("simbad: missing expected column")

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
			return nil, fmt.Errorf("%w: %q", ErrMissingColumn, req)
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

		displayName, _ := friendlyName(mainID)

		t := resolve.Target{
			ID:       mainID,
			Name:     displayName,
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
					t.HasRadialVelocity = true
				}
			}
			// V-band magnitude from allfluxes table. The column name is
			// "V" (uppercase) in SIMBAD's live TAP response — confirmed
			// directly against the real service, not "v".
			if vmagIdx, ok := colIdx["V"]; ok && row[vmagIdx] != "" {
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

// ParseBrightCSV parses the response of BuildBrightQuery — one row per star
// (no `ident` join, so unlike ParseCSV there's no alias fan-out to dedupe),
// preserving the ADQL response's brightest-first row order directly into the
// result slice rather than through a map (whose iteration order is
// unspecified).
func ParseBrightCSV(r io.Reader) ([]resolve.Target, error) {
	reader := csv.NewReader(r)

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

	required := []string{"main_id", "ra", "dec"}
	for _, req := range required {
		if _, ok := colIdx[req]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrMissingColumn, req)
		}
	}

	var results []resolve.Target

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("simbad: failed to read CSV row: %w", err)
		}

		mainID := row[colIdx["main_id"]]

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

		displayName, _ := friendlyName(mainID)

		t := resolve.Target{
			ID:       mainID,
			Name:     displayName,
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
					t.HasRadialVelocity = true
				}
			}
		}

		// BuildBrightQuery aliases allfluxes.V to vmag (see its doc comment
		// for why: SIMBAD's live TAP parser rejects a qualified
		// table.column reference in ORDER BY), so the response column is
		// named "vmag", not "v"/"V".
		if vmagIdx, ok := colIdx["vmag"]; ok && row[vmagIdx] != "" {
			if v, err := strconv.ParseFloat(row[vmagIdx], 64); err == nil {
				t.VMag = v
				t.HasVMag = true
			}
		}

		results = append(results, t)
	}

	return results, nil
}

// mapSimbadKind maps common SIMBAD Object Types (OTypes) to astrogo internal kinds.
func mapSimbadKind(o string) resolve.Kind {
	switch o {
	case "Star":
		return resolve.KindStar
	case "**":
		return resolve.KindDoubleStar
	case "GlC", "OpC", "Cl*":
		return resolve.KindStarCluster // or Globular/Open specifically inside logic
	case "PN", "HII", "Neb":
		return resolve.KindNebula
	case "G", "Gal", "AGN":
		return resolve.KindGalaxy
	case "SNR":
		return resolve.KindSupernovaRemnant
	}

	// SIMBAD's own OTYPES nomenclature marks every single-star
	// classification with a "*" somewhere in the code — usually a suffix
	// (V* variable, Em* emission-line, SB* spectroscopic binary, PM*
	// high-proper-motion, dS* delta Scuti, RG* red giant, WD* white
	// dwarf, ...) but sometimes internal (s*b/s*r blue/red supergiant) —
	// confirmed live against the real TAP service that ordinary bright
	// stars (Sirius, Canopus, Vega, Rigel, ...) come back with codes like
	// these, not the small hand-picked set this switch checked before,
	// which silently mapped nearly every real star to KindOther. "**"
	// (double/multiple star system) is excluded above since it has its
	// own Kind.
	if strings.Contains(o, "*") && o != "**" {
		return resolve.KindStar
	}

	return resolve.KindOther
}
