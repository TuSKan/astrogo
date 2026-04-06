package simbad

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/TuSKan/astrogo/catalog"
)

const tapSyncURL = "http://simbad.cds.unistra.fr/simbad/sim-tap/sync"

// BuildResolveQuery constructs an ADQL query to resolve an object by name
// from SIMBAD's TAP service. It joins the `basic` and `ident` tables.
func BuildResolveQuery(req catalog.ObjectRequest) string {
	// A naive query that looks up the object in the ident table 
	// and fetches core properties from the basic table.
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	q := catalog.Normalize(req.Query)
	// SIMBAD requires precise spaces for some ID types but we do a fuzzy LIKE for now
	// To be perfectly robust, one would use lowercase match or regex, but standard ADQL
	// supports LIKE or basic LIKE. SIMBAD's ADQL implementation supports lower() string functions.
	
	// Ensure we handle single quotes safely
	safeQ := strings.ReplaceAll(q, "'", "''")

	query := fmt.Sprintf(`SELECT TOP %d 
		basic.oid,
		basic.main_id,
		basic.ra,
		basic.dec,
		basic.otype,
		ident.id
	FROM basic 
	JOIN ident ON basic.oid = ident.oidref 
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
