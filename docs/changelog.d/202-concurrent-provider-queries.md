---
type: Changed
pr: 202
---
**`catalog.Resolver` queries its providers concurrently.** `Resolve` and `Search`
looped over them one at a time, so a resolver over SIMBAD and OpenNGC cost a CDS
round-trip *plus* a local lookup per query instead of the slower of the two.
Provider-registration order, merging and error reporting are unchanged.
