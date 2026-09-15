---
type: Changed
pr: 314
---
`ephemeris/satellite` reads its gravity constants from `constants.WGS72` instead
of holding a private copy — the copy is how the package came to use WGS-84's
values while the propagator was handed WGS-72's. `constants.WGS72` gains `J2`,
one of that standard's four defining parameters, which its own doc comment
already named.
