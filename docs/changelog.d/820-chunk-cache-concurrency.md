---
type: Changed
pr: 820
---
**Goroutines sharing one kernel no longer queue on its chunk cache.** A hit
takes no lock and a miss is read with none held, so sixteen goroutines
evaluating the Moon from de440s take 481 ns per state in aggregate where they
took 2,052 — what the same kernel costs in memory with no cache at all.
