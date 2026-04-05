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
// Version 1 supports a comprehensive mapping graph covering:
//   - Astrometric <-> Apparent <-> Observed (AltAz): The complete observatory
//     pipeline including proper motion, light deflection, aberration, dynamical 
//     EOP integration (DUT1, polar motion), and atmospheric refraction.
//   - ICRS <-> Galactic: Using the IAU 1958 definition.
//   - ICRS <-> Ecliptic: Using coordinates of the date (IAU 2006).
//
// # Extensibility
//
// Refraction within `transform` is decoupled through `earth.Atmosphere.Model`, 
// enabling standard SOFA integration or explicit bypass configurations without 
// disrupting celestial mechanics.
//
// While v1 uses explicit functions (e.g., [ICRSToAltAz]), the package is
// designed to eventually support a dynamic transformation graph, where
// paths between arbitrary frames are found and executed automatically.
package transform
