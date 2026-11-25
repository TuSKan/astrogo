---
type: Changed — BREAKING
pr: 1267
---
**Downloads are allowed by default, of any size, and every one is logged at `Warn`**, naming the endpoint, the cache key and the size; a cache hit logs nothing. `remote.SetPolicy` is the one control, and a test or program that must stay offline calls `remote.SetOffline(true)`. Kernels `spk.CacheAPI` has Horizons generate are logged the same way.
