// Package transform provides astronomical coordinate transformations.
//
// # Design
//
// The package implements explicit, high-precision transformations between
// the fundamental astronomical coordinate frames defined in package `coord`.
//
// It uses the standard-grade SOFA algorithms (via the internal `gofaext`
// package) to ensure scientific correctness, including precession-nutation
// models (IAU 2006/2000A) and atmospheric refraction where appropriate.
//
// # Supported Transformations
//
// Version 1 supports:
//   - ICRS <-> AltAz (Observed): Includes Earth rotation and refraction.
//   - ICRS <-> Galactic: Using the IAU 1958 definition.
//   - ICRS <-> Ecliptic: Using coordinates of the date (IAU 2006).
//
// # Assumptions
//
//   - The current implementation assumes DUT1=0 and no polar motion (XP=0, YP=0).
//   - AltAz transforms assume a standard atmosphere (1013.25 hPa, 15°C)
//     for refraction unless otherwise noted in future versions.
//
// # Future Extensibility
//
// While v1 uses explicit functions (e.g., [ICRSToAltAz]), the package is
// designed to eventually support a dynamic transformation graph, where
// paths between arbitrary frames are found and executed automatically.
package transform
