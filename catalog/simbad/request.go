package simbad

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

const tapSyncURL = "http://simbad.cds.unistra.fr/simbad/sim-tap/sync"

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

// TAPRequest generates the URL-encoded body for a POST TAP query.
func TAPRequest(adql string) string {
	v := url.Values{}
	v.Set("REQUEST", "doQuery")
	v.Set("LANG", "ADQL")
	// CSV is straightforward to parse without a full VOTable XML parser.
	v.Set("FORMAT", "csv")
	v.Set("QUERY", adql)
	return v.Encode()
}
