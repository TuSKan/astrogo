package jpl

import (
	"bufio"
	"regexp"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// targetBodyNameRe matches Horizons' single-match identifying header line,
// present at the top of every unambiguous single-object response — verified
// against live responses for both a major body ("Target body name: Mars
// (499)                      {source: mar099}") and a small body ("Target
// body name: 1685 Toro (1948 OA)             {source: JPL#895}"). The
// parenthetical is not always numeric: small bodies may show a provisional
// designation instead of a NAIF/SPK ID.
var targetBodyNameRe = regexp.MustCompile(`(?m)^Target body name:\s*(.+?)\s*\(([^)]*)\)`)

// parseExactMatch extracts a single resolve.Target from Horizons' "Target
// body name:" header line. It intentionally parses nothing else from the
// response body (orbital elements, physical parameters) — see doc.go for
// why: that portion of Horizons' output has no stable, verified schema.
func parseExactMatch(data string) (resolve.Target, bool) {
	m := targetBodyNameRe.FindStringSubmatch(data)
	if m == nil {
		return resolve.Target{}, false
	}

	name := strings.TrimSpace(m[1])
	idOrDesig := strings.TrimSpace(m[2])

	if name == "" || idOrDesig == "" {
		return resolve.Target{}, false
	}

	t := resolve.Target{
		Catalog: "jpl",
		Name:    name,
		ID:      idOrDesig,
	}

	if isNumericID(idOrDesig) {
		t.SPKID = idOrDesig
	} else {
		t.Designation = idOrDesig
	}

	return t, true
}

// majorBodyMatchMarker identifies Horizons' fixed-width table of candidate
// major bodies (planets, satellites, spacecraft, barycenters) returned for
// an ambiguous query, e.g. `Multiple major-bodies match string "MARS*"`.
const majorBodyMatchMarker = "major-bodies match string"

// cosparDesignationRe matches a COSPAR international designator (e.g.
// "2005-029A"), used to locate the Designation field within a major-body
// match-table row. The ID column (0-10) is reliably fixed-width, but the
// Name/Designation/Alias columns are not: verified against a live response,
// a long body name can overflow its nominal ~35-char field with zero
// separating space from the designation that follows (e.g. "...Orbiter
// (spacec2005-029A..." — Horizons itself already truncated "spacecraft)"
// to "spacec" before concatenating), so fixed-offset slicing silently
// corrupts the Designation. Anchoring on the designation's own recognizable
// pattern is robust to that overflow; only the Name may still show
// Horizons' own upstream truncation in that edge case, which cannot be
// recovered from the text as sent.
var cosparDesignationRe = regexp.MustCompile(`\d{4}-\d{3}[A-Za-z]{1,4}`)

// parseMajorBodyMatchTable parses Horizons' major-body ambiguous-match
// table. The overall table-detection approach (marker text + dashed
// separator line) matches the parser already proven in production against
// live Horizons traffic by ephemeris/jpl/spk's CacheAPI kernel-resolution
// path; the header wording above the table has been observed to vary
// ("Number" vs "ID#"), so detection keys on the marker text, not the header.
func parseMajorBodyMatchTable(data string) ([]resolve.Target, bool) {
	if !strings.Contains(data, majorBodyMatchMarker) {
		return nil, false
	}

	var targets []resolve.Target

	scanner := bufio.NewScanner(strings.NewReader(data))
	inTable := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "-------") && strings.Contains(line, "------------------") {
			inTable = true
			continue
		}

		if !inTable {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(targets) > 0 {
				break
			}

			continue
		}

		if len(line) < 10 {
			continue
		}

		id := strings.TrimSpace(safeSubstr(line, 0, 10))
		if id == "" {
			continue
		}

		rest := safeSubstr(line, 10, -1)

		var name, desig, alias string

		if loc := cosparDesignationRe.FindStringIndex(rest); loc != nil {
			name = strings.TrimSpace(rest[:loc[0]])
			desig = rest[loc[0]:loc[1]]
			alias = strings.TrimSpace(rest[loc[1]:])
		} else {
			name = strings.TrimSpace(rest)
		}

		t := resolve.Target{
			Catalog:     "jpl",
			ID:          id,
			Name:        name,
			Designation: desig,
		}
		if alias != "" {
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
// structurally different table (columns: Record #, Epoch-yr, >MATCH DESIG<,
// Primary Desig, Name) from the major-body match table above — confirmed
// against live responses for both an ambiguous small-body query (multiple
// "73P" fragment records) and a zero-match query ("No matches found.").
const smallBodyIndexMarker = "Small-body Index Search Results"

// whitespaceRunRe splits a small-body index row on runs of 2+ spaces,
// since the table is space-padded rather than strictly fixed-width and a
// body Name may itself contain a single internal space (e.g.
// "Schwassmann-Wachmann 3").
var whitespaceRunRe = regexp.MustCompile(`\s{2,}`)

// parseSmallBodyIndexTable parses Horizons' DASTCOM small-body index table.
// It returns (nil, true) for a recognized-but-empty response (e.g. "No
// matches found."), distinct from (nil, false) when the marker isn't
// present at all.
func parseSmallBodyIndexTable(data string) ([]resolve.Target, bool) {
	if !strings.Contains(data, smallBodyIndexMarker) {
		return nil, false
	}

	var targets []resolve.Target

	scanner := bufio.NewScanner(strings.NewReader(data))
	inTable := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !inTable {
			if strings.HasPrefix(trimmed, "--------") {
				inTable = true
			}

			continue
		}

		if trimmed == "" {
			if len(targets) > 0 {
				break
			}

			continue
		}

		fields := whitespaceRunRe.Split(trimmed, -1)
		if len(fields) < 5 {
			continue
		}

		recordID := fields[0]
		primaryDesig := fields[3]
		name := strings.Join(fields[4:], " ")

		t := resolve.Target{
			Catalog:     "jpl",
			ID:          recordID,
			Name:        name,
			Designation: primaryDesig,
		}

		if isNumericID(recordID) {
			t.SPKID = recordID
		}

		targets = append(targets, t)
	}

	return targets, true
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

// safeSubstr returns s[start:start+length], clamped to s's bounds; length
// == -1 means "to the end of the string".
func safeSubstr(s string, start, length int) string {
	if start >= len(s) {
		return ""
	}

	if length == -1 || start+length > len(s) {
		return s[start:]
	}

	return s[start : start+length]
}
