---
type: Added
pr: 311
---
`constants.WGS72` — the World Geodetic System 1972 realization every two-line
element set is expressed in, as its own `Set` alongside `WGS84`, carrying its
four defining parameters (a, GM, J2, ω) plus the published flattening.
`ephemeris/satellite` now reads its gravity constants from it instead of holding
a private copy, which is how that copy came to be WGS-84's while the propagator
needed WGS-72's. `WGS84Set` also gains the `GeocentricGravitationalConstant` its
own comment named as a defining parameter without carrying it.
