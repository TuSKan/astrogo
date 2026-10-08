package simbad

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// csvBody prepares a CSV response for parsing, refusing one that is a web page.
//
// An archive serves its own failures as HTML with a 200, and encoding/csv does
// not obviously reject that: a web page is lines of text, some with commas in
// them, so whether it errors depends on the page. Measured against SIMBAD's
// parsers it did error, but on the header check -- "missing expected column:
// main_id" -- which reads as the service having changed its schema rather than
// as the service being down. That is the same misdiagnosis #301 fixed on the
// VOTable side, and it is why the check is here rather than left to luck.
//
// The body is peeked, not consumed, so the reader handed back is still whole.
func csvBody(r io.Reader) (*bufio.Reader, error) {
	br := bufio.NewReader(r)

	// Enough to clear a byte order mark and any leading blank lines. A
	// document that has not declared itself HTML by then is not HTML.
	head, _ := br.Peek(512)

	if remote.LooksLikeHTML(head) {
		return nil, fmt.Errorf("simbad: %w", remote.ErrNotServingData)
	}

	return br, nil
}

// ErrMissingColumn indicates a required column is missing from the SIMBAD response.
var ErrMissingColumn = errors.New("simbad: missing expected column")

// ErrEmptyQuery marks a resolve request with nothing to match on. An empty
// name cannot identify an object, and matching it against everything is how
// the substring query this replaced returned an arbitrary one.
var ErrEmptyQuery = errors.New("simbad: empty query")

// ParseCSV parses SIMBAD's TAP output in CSV format into resolve.Targets.
// The expected order from BuildResolveQuery is:
// oid, main_id, ra, dec, otype, id (matched alias)
func ParseCSV(r io.Reader) ([]resolve.Target, error) {
	body, err := csvBody(r)
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(body)

	// Read header and build column index map
	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
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

		otype := rowKind(row, colIdx)

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
					// SIMBAD's rvz_radvel is km/s.
					t.RadialVelocity = unit.KmPerSec(v)
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
	body, err := csvBody(r)
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(body)

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
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

		otype := rowKind(row, colIdx)

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
					// SIMBAD's rvz_radvel is km/s.
					t.RadialVelocity = unit.KmPerSec(v)
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

// rowKind classifies a row by its otype and, when the query joined it, the
// otype's place in SIMBAD's hierarchy (otype_path).
func rowKind(row []string, colIdx map[string]int) resolve.Kind {
	var otype, path string

	if i, ok := colIdx["otype"]; ok && i < len(row) {
		otype = row[i]
	}

	if i, ok := colIdx["otype_path"]; ok && i < len(row) {
		path = row[i]
	}

	return simbadKind(otype, path)
}

// simbadKind classifies a SIMBAD object by its place in SIMBAD's own
// object-type hierarchy, the path its otypedef table gives each of the 226
// codes: "* > Ev* > LP*" for a long-period variable, "G > AGN > SyG > Sy2" for
// a Seyfert 2 galaxy, "Cl* > OpC" for an open cluster, "ISM > SNR" for a
// supernova remnant.
//
// The path rather than the code, because the code does not say. This used to
// match strings: a few literal codes, and any code containing '*' taken for a
// star. A candidate drops the '*' ("LP?", "EB?", "bC?"), so 109 stars brighter
// than V 7 came back as KindOther, Aldebaran among them; every galaxy code but
// "G" and "AGN" did too, M33 ("GiG") and M77 ("Sy2") included; associations
// ("As*") became stars; and three of the literal cases ("Star", "Gal", "Neb")
// are not SIMBAD codes at all (#601).
//
// A root that classifies only a detection (X, UV, Rad, IR, ...) or something
// that is not an object (a lensing event, a region) is KindOther: SIMBAD has
// not said what the source is, and neither can this. With no path, an otype
// otypedef does not list, the code stands in for a one-level path, which still
// places the hierarchy's roots ("*", "G", "Cl*").
func simbadKind(otype, path string) resolve.Kind {
	if strings.TrimSpace(path) == "" {
		path = otype
	}

	nodes := strings.Split(path, ">")
	root := strings.TrimSpace(nodes[0])
	leaf := strings.TrimSpace(nodes[len(nodes)-1])

	switch root {
	case "*":
		switch leaf {
		case "**":
			return resolve.KindDoubleStar
		case "PN":
			// SIMBAD files planetary nebulae under evolved stars; what an
			// observer sees is the nebula.
			return resolve.KindNebula
		}

		return resolve.KindStar
	case "Cl*":
		switch leaf {
		case "OpC":
			return resolve.KindOpenCluster
		case "GlC":
			return resolve.KindGlobularCluster
		}

		return resolve.KindStarCluster
	case "As*":
		// Associations, moving groups and streams: a star cluster, as
		// OpenNGC's "*Ass" is (#599).
		return resolve.KindStarCluster
	case "G", "GrG", "ClG", "SCG", "PCG", "PaG", "IG", "PoG":
		return resolve.KindGalaxy
	case "ISM", "PoC":
		if leaf == "SNR" {
			return resolve.KindSupernovaRemnant
		}

		return resolve.KindNebula
	}

	return resolve.KindOther
}
