---
type: Changed — BREAKING
pr: 868
---
**`kepler.FromPerihelion` and `eph.ElementsFromPerihelion` take the osculation
epoch before the perihelion time.** `Epoch` reports it and `MeanAnomaly` is the
mean anomaly there; positions are unchanged. Pass the perihelion time for both
to keep the old behavior.
