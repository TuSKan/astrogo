---
type: Added
pr: 254
---
`ephemeris.AstrometricState` returns the astrometric place — the target
retarded by light time with the observer left where it is — which is the
quantity JPL Horizons publishes as quantity 1 and the one a consumer applying
its own aberration needs (#254).
