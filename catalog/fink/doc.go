// Package fink provides a catalog provider for the FINK/ZTF Solar System Object
// Fink Table (SSOFT), which contains fitted sHG1G2 phase-curve parameters
// (Carry et al. 2024) for ~95,000 asteroids observed by the ZTF survey.
//
// The provider supports two access modes:
//   - Fast single-object queries via the /api/v1/ssoft JSON endpoint
//   - Bulk parquet table download (~60 MB) with in-memory indexing by IAU number and name
//
// Resolved targets are populated with H, G1, G2, spin axis (α₀, δ₀), and
// oblateness (R) parameters using r-band (filter 2) as the primary band.
//
// The SSOFT version defaults to "2025.04" (the API defaults to the current
// calendar month, which may not be published yet). Use NewWithVersion to
// specify a different release.
//
// API: https://api.ztf.fink-portal.org/api/v1/ssoft
// Swagger: https://api.ztf.fink-portal.org/swagger.json
// Reference: Carry et al. (2024), A&A, 689, A252. doi:10.1051/0004-6361/202449789
package fink
