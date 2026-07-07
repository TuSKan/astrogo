// Package jpl provides a resolve.Provider implementation targeting the
// NASA JPL Horizons system.
//
// # Current status
//
// [Provider.ResolveObject] reaches the live horizons.api endpoint and
// decodes its JSON envelope, but does not yet parse the "result" field —
// Horizons has no stable schema for that field (it ranges from a
// major-body match table to a full small-body orbital-elements printout,
// and even the match-table header wording has been observed to differ
// across responses). Rather than guess at a shape and risk silently
// extracting the wrong identifier, every successful, decodable response
// currently returns [ErrNotImplemented]. [Provider.Resolve] and
// [Provider.Search] surface this as ok=false / an empty result.
//
// This package is exclusively for resolving static object identities. For
// resolving actual time-dependent position states and astrometric
// geometries, use the ephemeris/jpl package instead — that path is fully
// implemented and does not depend on this one.
package jpl
