---
type: Fixed
pr: 782
---
**`ephemeris/kepler.SolveKepler` converges just before perihelion on orbits close to parabolic.** With the mean anomaly taken in [0, 2π), Kepler's equation's rounding near 2π exceeded its tolerance there, and for e ≥ 0.9999 about one instant in two thousand failed with `ErrKeplerNoConverge`; `plan.VisibleTonight` failed on comet C/2026 C1 that way. It now solves in (−π, π] and still returns E in [0, 2π).
