---
type: Fixed
pr: 549
---
**`plan.SwapOptimizedStrategy` could schedule a block before the slew to it had finished.**
Its swap and gap-insertion moves checked the transition into the block they moved but not the one out of it, and left stale `SetupTime`s; every move is now re-timed against its real neighbours.
