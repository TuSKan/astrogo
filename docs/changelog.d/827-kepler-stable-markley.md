---
type: Changed — BREAKING
pr: 827
---
**`kepler.SolveKepler` is Markley's method on an exactly reduced mean anomaly,
within 2 ulps of the exact root everywhere up to e = 1 − 2⁻⁵³, and 1.75×
faster.** It answers in [−π, π] rather than [0, 2π) and refuses |M| ≥ 2⁵² with
`ErrPhaseUnresolved`; the classical state no longer loses what 1 − e has.
