package vizier

import (
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"
)

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
	// Epoch is this table's native reference epoch for RA/Dec — the three
	// tables registered below genuinely differ (2MASS ~J2000, Hipparcos
	// J1991.25, Gaia DR3 J2016.0), so this cannot be a single package-wide
	// constant; every row from a table is stamped with its own.
	Epoch time.Time
}

// defaultTable is used when a ConeRequest doesn't specify Table, preserving
// this package's original single-table behavior exactly.
const defaultTable = "II/246/out"

// Standard reference epochs for the tables below, expressed as two-part
// Julian dates matching each survey's own documented catalog epoch.
var (
	epoch2MASS     = time.J2000                         // "raj2000"/"dej2000" column names state this explicitly
	epochHipparcos = time.FromJD(2448349.0625, time.TT) // J1991.25, the Hipparcos catalog's own reference epoch
	epochGaiaDR3   = time.FromJD(2457388.5, time.TT)    // J2016.0, Gaia DR3's reference epoch
)

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
	defaultTable: {RACol: "raj2000", DecCol: "dej2000", DesigCol: `"2MASS"`, Kind: resolve.KindStar, Epoch: epoch2MASS},
	// Hipparcos main catalog.
	"I/239/hip_main": {RACol: "RAICRS", DecCol: "DEICRS", DesigCol: "HIP", Kind: resolve.KindStar, Epoch: epochHipparcos},
	// Gaia DR3 (VizieR mirror of the Gaia archive).
	"I/355/gaiadr3": {RACol: "RA_ICRS", DecCol: "DE_ICRS", DesigCol: "DR3Name", Kind: resolve.KindStar, Epoch: epochGaiaDR3},
}
