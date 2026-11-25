---
type: Fixed
pr: 1273
---
**`epoch.LoadEOP` offline recorded a cached bulletin as fetched from the network**, so `epoch.EOPSource` said "network" for a program that never went online. Its network step now asks offline mode first, as it already asked the download policy.
