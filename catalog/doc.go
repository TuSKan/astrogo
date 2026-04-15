// Package catalog provides a lightweight astronomical object catalog system
// with unified resolution across multiple remote and local providers.
//
// # Architecture
//
// The top-level [Resolver] orchestrates one or more data sources to find
// celestial targets by name. Supported sources are selected via [Source]
// constants at construction time:
//
//   - [OpenNGC]  — embedded NGC/IC catalog (zero I/O, via go:embed)
//   - [SIMBAD]   — CDS SIMBAD (ADQL/TAP)
//   - [MAST]     — STScI Mikulski Archive (CAOM REST)
//   - [JPL]      — NASA JPL Horizons
//   - [SBDB]     — NASA JPL Small-Body Database
//   - [Gaia]     — ESA Gaia DR3 (ADQL/TAP)
//   - [VizieR]   — CDS VizieR multi-catalog (ADQL/TAP)
//
// # Core types
//
// The shared data types ([Target], [Provider], [Kind], [ObjectRequest]) are
// defined in the [catalog/resolve] subpackage and re-exported here as type
// aliases for convenience.
//
// Each provider subpackage (simbad, gaia, mast, etc.) implements the
// [resolve.Provider] interface, which supports both exact name resolution
// and spatial cone searches where applicable.
//
// # Caching
//
// The [resolve.Cache] layer persists resolved results locally using Apache
// Arrow columnar batches, enabling offline replay and deduplication.
package catalog
