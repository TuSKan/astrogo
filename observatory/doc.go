// Package observatory models astronomical observing sites and their local metadata.
//
// # Design
//
// This package provides the [Site] type, which encapsulates the physical location
// of an observer (longitude, latitude, elevation), their local horizon limit,
// and time zone information.
//
// # Role in Astrogo
//
// The observatory package serves as a metadata layer for other scientific
// packages. For example:
//   - The `transform` package uses site metadata for topocentric conversions.
//   - The `sky` and `planning` packages use site metadata to determine
//     object visibility and optimal observing windows.
//
// Currently, this package focuses on static site properties. Detailed
// atmospheric models (weather, varying refraction) are deferred to later
// versions or specialized packages.
package observatory
