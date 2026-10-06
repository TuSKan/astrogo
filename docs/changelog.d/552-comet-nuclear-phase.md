---
type: Changed — BREAKING
pr: 552
---
**`magnitude.CometNuclearApparent` now takes the phase coefficient and phase angle**, `(M2, k2, pc, r, delta, phase)`, and `plan.WithNuclearMagnitude` takes `pc`.
JPL's nuclear magnitude includes PC·β, which was missing and made 13P/Olbers's nucleus 0.21 mag bright against Horizons; SBDB's `PC` is now parsed into `resolve.Target.PC`.
Callers pass SBDB's PC, or zero where it publishes none.
