package vizier

import "github.com/TuSKan/astrogo/catalog/resolve"

// tableSchema describes a VizieR table's ADQL column names for the fields
// ConeSearch needs, keyed by VizieR table identifier (e.g. "II/246/out").
type tableSchema struct {
	// RACol/DecCol are the table's decimal-degree ICRS RA/Dec column names.
	RACol, DecCol string
	// DesigCol selects the ADQL expression used as each row's
	// designation/name. It is normally a bare column name (e.g. "HIP"),
	// but for II/246/out it is the double-quoted identifier `"2MASS"` —
	// that table genuinely has a column literally named "2MASS" holding
	// the point-source designation, which is why the original
	// (pre-registry) query aliased it directly.
	DesigCol string
	// Kind is the resolve.Kind assigned to every row from this table.
	Kind resolve.Kind
}

// defaultTable is used when a ConeRequest doesn't specify Table, preserving
// this package's original single-table behavior exactly.
const defaultTable = "II/246/out"

// tableSchemas is the registry of VizieR tables ConeSearch knows how to
// query. Column names below are verified against VizieR's live TAP service
// (http://tapvizier.u-strasbg.fr/TAPVizieR/tap/sync), not guessed —
// querying an unregistered table returns ErrUnknownTable rather than
// assuming generic column names that may not exist for that table. Adding
// a table here is a data change, not an API change.
//
//nolint:gochecknoglobals // read-only registry, populated once at init
var tableSchemas = map[string]tableSchema{
	// 2MASS Point Source Catalog — the package's original hardcoded table.
	defaultTable: {RACol: "raj2000", DecCol: "dej2000", DesigCol: `"2MASS"`, Kind: resolve.KindStar},
	// Hipparcos main catalog.
	"I/239/hip_main": {RACol: "RAICRS", DecCol: "DEICRS", DesigCol: "HIP", Kind: resolve.KindStar},
	// Gaia DR3 (VizieR mirror of the Gaia archive).
	"I/355/gaiadr3": {RACol: "RA_ICRS", DecCol: "DE_ICRS", DesigCol: "DR3Name", Kind: resolve.KindStar},
}
