---
type: Changed
pr: 917
---
**The benchmarks no longer time whatever Earth orientation the machine holds.** `BenchmarkUTCToUT1` now measures a pinned zero model, so the
CI benchmark history steps once. Pinned zero and table variants of UT1, `Time.EOP`, `NewContext`, `SetTime` and a scheduler are added,
and the 13 discarded errors in benchmarks now fail them.
