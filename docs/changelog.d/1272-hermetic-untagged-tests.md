---
type: Changed
pr: 1272
---
**The untagged test suite runs offline, against an empty cache**, in every package whose tests link `remote`, so a test that needs the network or a cached file fails on every machine and not only on a CI runner. `ephemeris/jpl` and the kernel suites under it, which read NAIF's published kernels by design, are the listed exceptions.
