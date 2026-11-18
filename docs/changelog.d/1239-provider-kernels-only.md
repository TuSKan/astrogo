---
type: Changed — BREAKING
pr: 1239
---
**`ephemeris.NewProvider` builds kernel providers only.** Each of `ephemeris.Satellites`, `ephemeris.Stations`, `ephemeris.WithTLE`, `ephemeris.ErrTLERequired`, `ephemeris.ErrNotImplemented` and `jpl.ErrNotImplemented` was removed. A satellite comes from `satellite.NewFromTLE`, which returns the concrete `*satellite.Satellite` with its `ID`.
