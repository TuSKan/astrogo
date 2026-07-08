// Package vizier provides a resolve.ConeSearcher implementation targeting the
// CDS VizieR catalog service via ADQL TAP.
//
// # Current status
//
// [Provider.ConeSearch] queries any VizieR table registered in this
// package's schema registry (tables.go), selected via
// [resolve.ConeRequest].Table. An empty Table defaults to the 2MASS
// point-source catalog (II/246/out), this package's original behavior.
// Querying a table not in the registry returns [ErrUnknownTable] rather
// than guessing that table's RA/Dec/designation column names.
//
// Tables registered today:
//
//   - II/246/out — 2MASS Point Source Catalog (default)
//   - I/239/hip_main — Hipparcos main catalog
//   - I/355/gaiadr3 — Gaia DR3 (VizieR mirror)
//
// Adding a table is a data change (a new tables.go registry entry with
// verified column names), not an API change — the registry can grow
// without touching [resolve.ConeRequest] or [Provider.ConeSearch]'s
// signature.
package vizier
