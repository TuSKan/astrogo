---
type: Added
pr: 1256
---
**Gaia rows carry their Hipparcos-2 and Tycho-2 identifiers from Gaia's own cross-match**, `HIP n` and `TYC r-n-c`, wherever the match is unambiguous, so they join SIMBAD's stars by identifier. `gaia.Provider.ResolveIdentifiers` resolves those identifiers too: a star that lists its HIP number but no Gaia designation still brings in its Gaia row through `catalog.Resolver`.
