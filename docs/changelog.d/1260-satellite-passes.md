---
type: Changed — BREAKING
pr: 1260
---
**`plan.SatellitePasses(sat, site, w, opts...)`** takes the `*Satellite` that `NewSatellite` builds, a `*Site`, a `Window`, and `plan.WithMinElevation`, 0° by default, in place of seven positional arguments and a `*coord.Geodetic`. A nil site is `ErrNilSite`.
