package simbad

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// identifierVariants returns the spellings SIMBAD might store for a
// user-typed identifier.
//
// SIMBAD right-justifies a catalogue number in a fixed-width field, so the
// Andromeda Galaxy is stored as "M  31" and its Henry Draper number as
// "HD   3969" — two and three spaces, chosen by the width of the field and
// the number, not by any rule a caller can be expected to follow. Nobody
// types those. A user types "M31", "M 31" or "m31".
//
// SIMBAD's ADQL cannot normalise: REPLACE, LOWER, ILIKE and ivo_nocasematch
// are all rejected by its parser, verified against the live service. So the
// normalisation has to happen here, by generating the handful of spellings
// the padding can produce and matching all of them exactly.
//
// Four spaces is past every field width SIMBAD uses for the catalogues in
// common use, and an over-generated variant simply matches nothing.
//
// The "NAME " prefix is how SIMBAD stores common names — "NAME Andromeda
// Galaxy", "NAME Centaurus A" — so prepending it makes a plain-English name
// resolve without a second service.
func identifierVariants(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}

	seen := map[string]bool{q: true, "NAME " + q: true}

	// Split a leading catalogue acronym from the number that follows it, so
	// the padding can be varied between them.
	if i := strings.IndexFunc(q, func(r rune) bool { return r >= '0' && r <= '9' }); i > 0 {
		prefix := strings.TrimRight(q[:i], " ")
		number := q[i:]

		for pad := range maxIdentifierPadding + 1 {
			seen[prefix+strings.Repeat(" ", pad)+number] = true
		}
	}

	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}

	sort.Strings(out) // deterministic, so the query is reproducible and cacheable

	return out
}

// maxIdentifierPadding is the widest gap [identifierVariants] will try
// between a catalogue acronym and its number.
const maxIdentifierPadding = 4

// quoteADQL renders values as a comma-separated ADQL string list.
func quoteADQL(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}

	return strings.Join(quoted, ", ")
}

// BuildResolveQuery constructs an ADQL query that identifies exactly one
// object by name.
//
// # Why this is not a LIKE
//
// It was. `WHERE ident.id LIKE '%<query>%'` matched any object carrying the
// query as a substring of any of its identifiers, and returned TOP N of them
// with no ORDER BY. For "M31" that is 15,843 rows — every CXOM31 J… Chandra
// source in the galaxy — and the ten fetched did not include M31 itself. The
// provider's own scoring then ranked ten wrong answers and returned the best,
// which is how Resolve("M87") came back with an object 70 degrees away in
// Cassiopeia while Resolve("M42") came back with no coordinates at all.
//
// Matching exactly against [identifierVariants] costs one query, returns one
// object, and returns nothing at all when the name is unknown — which is the
// answer a caller can act on.
//
// The subquery is what keeps the alias list: filtering the joined `ident`
// directly would return only the identifier that matched, so the object's
// other names would vanish. Selecting the oid first and joining afterwards
// leaves the fan-out intact.
func BuildResolveQuery(req resolve.ObjectRequest) string {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	variants := identifierVariants(req.Query)
	if len(variants) == 0 {
		return ""
	}

	return fmt.Sprintf(`SELECT TOP %d
		basic.oid,
		basic.main_id,
		basic.ra,
		basic.dec,
		basic.otype,
		basic.pmra,
		basic.pmdec,
		basic.plx_value,
		basic.rvz_radvel,
		ident.id,
		allfluxes.V
	FROM basic
	JOIN ident ON basic.oid = ident.oidref
	LEFT JOIN allfluxes ON basic.oid = allfluxes.oidref
	WHERE basic.oid IN (SELECT oidref FROM ident WHERE id IN (%s))`,
		limit, quoteADQL(variants))
}

// BuildSearchQuery constructs an ADQL query for a freeform search, where
// several answers are the point rather than a failure.
//
// Still a LIKE, because that is what a search is — but anchored at the start
// rather than wrapped in wildcards on both sides, and ordered, so that the
// rows returned are the ones a person would recognise and are the same rows
// on every run. The unordered TOP N it replaces was not reproducible: two
// identical calls for "M42" returned different objects.
//
// Brightest first, since between two objects whose identifiers both begin
// with the query, the brighter is overwhelmingly the one meant. Nulls sort
// last so an object with no V magnitude never displaces one that has it.
func BuildSearchQuery(req resolve.ObjectRequest) string {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	safeQ := strings.ReplaceAll(strings.TrimSpace(req.Query), "'", "''")

	return fmt.Sprintf(`SELECT TOP %d
		basic.oid,
		basic.main_id,
		basic.ra,
		basic.dec,
		basic.otype,
		basic.pmra,
		basic.pmdec,
		basic.plx_value,
		basic.rvz_radvel,
		ident.id,
		allfluxes.V AS vmag
	FROM basic
	JOIN ident ON basic.oid = ident.oidref
	LEFT JOIN allfluxes ON basic.oid = allfluxes.oidref
	WHERE ident.id LIKE '%s%%'
	ORDER BY vmag ASC, basic.main_id ASC`, limit, safeQ)
}

// BuildBrightQuery constructs an ADQL query enumerating every object
// SIMBAD carries brighter than req.MaxVMag, ordered brightest-first. Unlike
// BuildResolveQuery this never joins `ident` — a bright-magnitude browse has
// no name to match against, so there's no alias-per-row fan-out to dedupe,
// and each result row already corresponds to exactly one star.
//
// allfluxes.V is aliased to vmag and ORDER BY references that alias rather
// than the qualified column: SIMBAD's live TAP parser rejects a qualified
// table.column reference in ORDER BY ("Incorrect ADQL query: Encountered
// '.'"), confirmed directly against the real service — every other clause
// here works fine qualified, this is specifically an ORDER BY restriction.
func BuildBrightQuery(req resolve.BrightRequest) string {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	return fmt.Sprintf(`SELECT TOP %d
		basic.oid,
		basic.main_id,
		basic.ra,
		basic.dec,
		basic.otype,
		basic.pmra,
		basic.pmdec,
		basic.plx_value,
		basic.rvz_radvel,
		allfluxes.V AS vmag
	FROM basic
	JOIN allfluxes ON basic.oid = allfluxes.oidref
	WHERE allfluxes.V < %f
	ORDER BY vmag ASC`, limit, req.MaxVMag)
}

// TAPRequest builds the form values for a POST TAP query.
func TAPRequest(adql string) url.Values {
	v := url.Values{}
	v.Set("REQUEST", "doQuery")
	v.Set("LANG", "ADQL")
	// CSV is straightforward to parse without a full VOTable XML parser.
	v.Set("FORMAT", "csv")
	v.Set("QUERY", adql)

	return v
}
