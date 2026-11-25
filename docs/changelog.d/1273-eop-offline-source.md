---
type: Fixed
pr: 1273
---
**`epoch.LoadEOP` offline recorded a cached bulletin as fetched from the network**, so `epoch.EOPSource` said "network" for a program that never went online; its network step now asks offline mode first. A cached bulletin that cannot be read is now reported as that, not as no bulletin cached.
