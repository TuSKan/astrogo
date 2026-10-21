---
type: Changed — BREAKING
pr: 823
---
**`ephemeris.ID` is a NAIF code, and nothing else.** Planets' centers and
their system barycenters have their own names (`eph.Jupiter` is 599,
`eph.JupiterBarycenter` 5), providers answer the code asked for or refuse it,
a satellite is −100000 − its NORAD number, and `SmallBodyID` reports validity
beside the ID. The default provider's Earth is the geocenter again, not the
Earth-Moon barycenter.
