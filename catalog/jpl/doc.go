// Package jpl provides a resolve.Provider implementation targeting the
// NASA JPL Horizons system.
//
// # Current status
//
// [Provider.ResolveObject] reaches the live horizons.api endpoint and parses
// its JSON-enveloped "result" field for the three recognized response
// shapes, verified against live Horizons traffic:
//
//   - An ambiguous major-body match (planets, satellites, spacecraft,
//     barycenters) — a fixed-width table, yielding one [resolve.Target] per
//     row.
//   - An ambiguous small-body match (comets/asteroids) — JPL/DASTCOM's
//     "Small-body Index Search Results" index table, a structurally
//     different table from the major-body one above, yielding one
//     [resolve.Target] per row.
//   - An unambiguous single match (major or small body) — Horizons' stable
//     "Target body name: <name> (<id-or-designation>)" identifying header
//     line, yielding exactly one [resolve.Target].
//
// A response matching none of these shapes but carrying non-blank result
// text returns [ErrNotImplemented] rather than a guessed/fabricated Target.
// In particular, this provider deliberately does not parse the body of a
// single-match response beyond its identifying header line — the physical
// parameters / orbital elements printout that follows has no stable,
// verified schema. Use [ephemeris/jpl] or [catalog/sbdb] for orbital
// elements and physical parameters.
//
// This package is exclusively for resolving static object identities. For
// resolving actual time-dependent position states and astrometric
// geometries, use the ephemeris/jpl package instead — that path is fully
// implemented and does not depend on this one.
package jpl
