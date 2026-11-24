// Package catalog provides a lightweight astronomical object catalog system
// with unified resolution across multiple remote and local providers.
//
// # Architecture
//
// The top-level [Resolver] orchestrates one or more data sources to find
// celestial targets by name. Supported sources are selected via [Source]
// constants at construction time:
//
//   - [OpenNGC]  — the OpenNGC NGC/IC catalog, fetched on its first query
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
// Each provider keeps what it has resolved in a [resolve.Cache], by default
// the in-memory [resolve.MapCache], so a repeated query is answered without
// asking the service again.
//
// # Concurrency
//
// A [Resolver] is not changed by a query, so one may serve any number of
// goroutines at once. It asks its providers concurrently, so a provider
// written elsewhere and handed to it must be safe for concurrent use, as the
// ones this module builds are. A [resolve.MapCache] is safe for concurrent
// use.
package catalog
