package jpl

import (
	"bufio"
	"regexp"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// targetBodyNameRe matches Horizons' single-match identifying header line,
// present at the top of every unambiguous single-object response. Verified
// against live responses, it takes these forms:
//
//	Target body name: Mars (499)                      {source: mar099}
//	Target body name: Voyager 1 (spacecraft) (-31)    {source: Voyager_1_ST+refit2022_m}
//	Target body name: 101955 Bennu (1999 RQ36) (2101955) {source: ORX_merged_DE424}
//	Target body name: 1685 Toro (1948 OA)             {source: JPL#895}
//	Target body name: 1P/Halley                       {source: JPL#75}
//
// The identifier is the last parenthetical, not the first: a name may carry
// parentheticals of its own, and taking the first made Voyager 1's ID
// "spacecraft". It is not always numeric, since a small body may show a
// provisional designation instead of a NAIF/SPK ID, and a comet may show no
// parenthetical at all.
var targetBodyNameRe = regexp.MustCompile(`(?m)^Target body name:[ \t]*(.*)$`)

// recordNumberRe matches the DASTCOM record number a small body's
// single-match response opens with, verified live as both
// "Rec #:90000030        Soln.date: ..." and "Rec #:    2688 (+COV) ...".
// It is the only identifier Horizons prints for a comet such as 1P/Halley,
// whose "Target body name" line has no parenthetical.
var recordNumberRe = regexp.MustCompile(`(?m)^Rec #:[ \t]*(\d+)`)

// parseExactMatch extracts a single resolve.Target from Horizons' "Target
// body name:" header line. It intentionally parses nothing else from the
// response body (orbital elements, physical parameters) — see doc.go for
// why: that portion of Horizons' output has no stable, verified schema.
func parseExactMatch(data string) (resolve.Target, bool) {
	m := targetBodyNameRe.FindStringSubmatch(data)
	if m == nil {
		return resolve.Target{}, false
	}

	header, _, _ := strings.Cut(m[1], "{source:")
	name, id := splitTrailingParenthetical(strings.TrimSpace(header))

	fromRecord := false

	if id == "" {
		if r := recordNumberRe.FindStringSubmatch(data); r != nil {
			id, fromRecord = r[1], true
		}
	}

	if name == "" || id == "" {
		return resolve.Target{}, false
	}

	t := resolve.Target{
		Catalog: "jpl",
		Name:    name,
		ID:      id,
	}

	switch {
	case fromRecord:
		// A DASTCOM record number, which is not an SPK-ID.
	case isNumericID(id):
		t.SPKID = id
	default:
		t.Designation = id
	}

	return t, true
}

// splitTrailingParenthetical splits "Voyager 1 (spacecraft) (-31)" into
// "Voyager 1 (spacecraft)" and "-31". A string not ending in a
// parenthetical comes back whole, with an empty second result.
func splitTrailingParenthetical(s string) (rest, inner string) {
	if !strings.HasSuffix(s, ")") {
		return s, ""
	}

	open := strings.LastIndex(s, "(")
	if open < 0 {
		return s, ""
	}

	return strings.TrimSpace(s[:open]), strings.TrimSpace(s[open+1 : len(s)-1])
}

// majorBodyMatchMarker identifies Horizons' fixed-width table of candidate
// major bodies (planets, satellites, spacecraft, barycenters) returned for
// an ambiguous query, e.g. `Multiple major-bodies match string "MARS*"`.
const majorBodyMatchMarker = "major-bodies match string"

// parseMajorBodyMatchTable parses Horizons' major-body ambiguous-match
// table. Its four columns, ID, name, designation and aliases, are read by
// position, from the spans its separator line gives (see readTable): the
// header wording above the table has been observed to vary ("Number" vs
// "ID#"), so detection keys on the marker text, not the header.
//
// A table that does not have four columns is not recognized, so a changed
// layout surfaces as ErrNotImplemented rather than as misread fields.
func parseMajorBodyMatchTable(data string) ([]resolve.Target, bool) {
	if !strings.Contains(data, majorBodyMatchMarker) {
		return nil, false
	}

	_, rows, found := readTable(data)
	if !found {
		return nil, true
	}

	const (
		colID = iota
		colName
		colDesignation
		colAliases
		numColumns
	)

	var targets []resolve.Target

	for _, row := range rows {
		if len(row) != numColumns {
			return nil, false
		}

		id := row[colID]
		if id == "" {
			continue
		}

		t := resolve.Target{
			Catalog:     "jpl",
			ID:          id,
			Name:        row[colName],
			Designation: row[colDesignation],
		}
		if alias := row[colAliases]; alias != "" {
			t.Aliases = strings.Split(alias, "/")
		}

		if isNumericID(id) {
			t.SPKID = id
		}

		targets = append(targets, t)
	}

	return targets, true
}

// smallBodyIndexMarker identifies Horizons' DASTCOM small-body index search
// results, returned when a query matches zero or more comets/asteroids,
// e.g. `JPL/DASTCOM            Small-body Index Search Results`. This is a
// structurally different table from the major-body one above, and its
// columns differ between searches — confirmed against live responses:
//
//	Record #  Epoch-yr  >MATCH DESIG<  Primary Desig  Name       (DES = 73P;)
//	Record #  Epoch-yr  Primary Desig  >MATCH NAME<              (NAME = Halley;)
//
// and against a zero-match query ("No matches found.").
const smallBodyIndexMarker = "Small-body Index Search Results"

// parseSmallBodyIndexTable parses Horizons' DASTCOM small-body index table,
// finding its columns by their headers, since their number and order depend
// on the search.
//
// A row's ID is its DASTCOM record number, which is what Horizons takes to
// select that record ("90000030" selects 1P/Halley's 1986 apparition). It is
// not an SPK-ID, so SPKID stays empty: SBDB gives 2688 Halley, record 2688,
// the SPK-ID 20002688, and 1P/Halley the SPK-ID 1000036.
//
// It returns (nil, true) for a recognized-but-empty response (e.g. "No
// matches found."), distinct from (nil, false) when the marker isn't
// present at all or the table has no record-number column.
func parseSmallBodyIndexTable(data string) ([]resolve.Target, bool) {
	if !strings.Contains(data, smallBodyIndexMarker) {
		return nil, false
	}

	header, rows, found := readTable(data)
	if !found {
		return nil, true
	}

	colRecord, colDesignation, colName := -1, -1, -1

	for i, h := range header {
		switch strings.Trim(h, "<> ") {
		case "Record #":
			colRecord = i
		case "Primary Desig":
			colDesignation = i
		case "Name", "MATCH NAME":
			colName = i
		}
	}

	if colRecord < 0 {
		return nil, false
	}

	// Every row has a cell per column (see readTable), so only a column
	// the table lacks needs guarding.
	cell := func(row []string, i int) string {
		if i < 0 {
			return ""
		}

		return row[i]
	}

	var targets []resolve.Target

	for _, row := range rows {
		recordID := cell(row, colRecord)
		if recordID == "" {
			continue
		}

		t := resolve.Target{
			Catalog:     "jpl",
			ID:          recordID,
			Name:        cell(row, colName),
			Designation: cell(row, colDesignation),
		}

		targets = append(targets, t)
	}

	return targets, true
}

// readTable reads the first table in data: a header line, a separator line
// of one dash run per column, and the rows below it up to the first blank
// line. It returns the header and each row split into one trimmed cell per
// column, and found is false when data has no separator line.
//
// A cell runs from the start of its column's dashes to the start of the
// next column's, not to the end of its own dashes. Horizons lets a long
// value fill the gap between columns, and for a long enough name it then
// runs straight into the next column ("Mars Reconnaissance Orbiter
// (spacec2005-029A"), so slicing at the dashes' end would cut it. The first
// cell starts at the line's beginning, since IDs are right-aligned, and the
// last runs to the line's end.
func readTable(data string) (header []string, rows [][]string, found bool) {
	scanner := bufio.NewScanner(strings.NewReader(data))

	var (
		previous string
		starts   []int
	)

	for scanner.Scan() {
		line := scanner.Text()

		if starts == nil {
			if isSeparator(line) {
				starts = columnStarts(line)
				header = splitColumns(previous, starts)
			}

			previous = line

			continue
		}

		if strings.TrimSpace(line) == "" {
			if len(rows) > 0 {
				break
			}

			continue
		}

		rows = append(rows, splitColumns(line, starts))
	}

	return header, rows, starts != nil
}

// isSeparator reports whether line is a table's separator: nothing but
// dashes and spaces, with at least two dash runs.
func isSeparator(line string) bool {
	if strings.Trim(line, "- ") != "" {
		return false
	}

	return len(columnStarts(line)) >= 2
}

// columnStarts returns the byte offset at which each dash run in a
// separator line starts.
func columnStarts(sep string) []int {
	var starts []int

	for i := range len(sep) {
		if sep[i] == '-' && (i == 0 || sep[i-1] != '-') {
			starts = append(starts, i)
		}
	}

	return starts
}

// splitColumns cuts line at the given column starts; see readTable.
func splitColumns(line string, starts []int) []string {
	cells := make([]string, len(starts))

	for i := range starts {
		lo := starts[i]
		if i == 0 {
			lo = 0
		}

		hi := len(line)
		if i+1 < len(starts) {
			hi = min(starts[i+1], len(line))
		}

		if lo < hi {
			cells[i] = strings.TrimSpace(line[lo:hi])
		}
	}

	return cells
}

// isNumericID reports whether s is an integer literal (optionally signed) —
// Horizons uses signed integer NAIF/SPK IDs for major bodies and spacecraft,
// but a small body's identifying parenthetical/primary-designation field can
// be a non-numeric provisional designation (e.g. "1948 OA").
func isNumericID(s string) bool {
	if s == "" {
		return false
	}

	for i, r := range s {
		if r == '-' && i == 0 {
			continue
		}

		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}
