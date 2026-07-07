// Package vizier provides a resolve.ConeSearcher implementation targeting the
// CDS VizieR catalog service via ADQL TAP.
//
// # Current status
//
// [Provider.ConeSearch] currently queries a single hardcoded table — the
// 2MASS point-source catalog (II/246/out) — as a generic fallback baseline.
// It does not yet support selecting an arbitrary VizieR table/catalog per
// request; ConeRequest has no field for one. Returned targets carry the
// 2MASS designation, RA/Dec, and Kind [resolve.KindStar].
package vizier
