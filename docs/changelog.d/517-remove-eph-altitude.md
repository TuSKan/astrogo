---
type: Removed
pr: 517
---
**`ephemeris.Altitude` no longer exists.** It returned geocentric distance less
a 6371 km mean radius, as a `float64` in km: 6 to 7 km off the WGS84 height
for the ISS, and for a planet not an altitude at all. A satellite provider is a
`*satellite.Satellite`, whose `Altitude` returns the WGS84 height as a
`unit.Length`; for any other geocentric state,
`coord.FromECEF(ctx.ICRSToITRS(pos), coord.WGS84())` with a `coord.Context` at
the epoch.
