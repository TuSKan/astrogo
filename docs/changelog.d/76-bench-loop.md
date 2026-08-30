---
type: Changed
pr: 76
---
**Benchmarks use `testing.B.Loop` instead of `b.N`.** 19 loops across 15
files, and the 20 now-dead `b.ResetTimer()` calls that preceded them —
`b.Loop` resets the timer on its first call.
