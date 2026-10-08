---
type: Fixed
pr: 615
---
**`catalog/mast` stored the name of the service it relayed a lookup to ("SIMBAD", "NED") as an alias**, so `xmatch` and `catalog.Resolver` matched unrelated MAST results as one object (M31 with M33 and Vega). The name is no longer recorded.
