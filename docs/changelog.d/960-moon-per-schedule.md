---
type: Changed
pr: 960
---
**`plan.SwapOptimizedStrategy` computes the Moon's place once for each placement it scores.** A process-wide cache of 32 instants recomputed it on every pass, 2,100 times for the 100 instants a 100-block schedule needs. The default configuration is 52% faster on 100 blocks, with 89% fewer allocations. Every score is unchanged to the last bit.
