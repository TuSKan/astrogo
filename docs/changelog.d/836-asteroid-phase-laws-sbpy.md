---
type: Fixed
pr: 836
---
**The asteroid phase laws' tests could not fail on a wrong spline knot or
mapping coefficient**: they read back their own tables. All four laws are now
held to sbpy's published values at 1e-8 mag, which they meet to 5e-9.
