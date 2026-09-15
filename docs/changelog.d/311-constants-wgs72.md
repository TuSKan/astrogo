---
type: Added
pr: 311
---
`constants.WGS72` — the World Geodetic System 1972 realization that every
two-line element set is expressed in, as its own `Set` alongside `WGS84`.
`WGS84Set` also gains the `GeocentricGravitationalConstant` its own comment
named as the fourth defining parameter without carrying it, so the two sets
can be compared member for member.
