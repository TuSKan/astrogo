package simbad

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// BuildResolveQuery constructs an ADQL query to resolve an object by name
// from SIMBAD's TAP service. It joins the `basic` and `ident` tables.
func BuildResolveQuery(req resolve.ObjectRequest) string {
	// A naive query that looks up the object in the ident table
	// and fetches core properties from the basic table.
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	// ADQL and postgres handle text, but SIMBAD enforces case-sensitivity on LIKE.
	// Preserving original casing.
	q := req.Query

	// Ensure we handle single quotes safely
	safeQ := strings.ReplaceAll(q, "'", "''")

	query := fmt.Sprintf(`SELECT TOP %d 
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
	WHERE ident.id LIKE '%%%s%%'`, limit, safeQ)

	return query
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
