---
type: Fixed
pr: 486
---
**The scheduler scored a block with the Earth turned to the nearest
constraint step**, not to the block's midpoint, whenever the block was not an
even number of steps long. It also now evaluates through one hourly `Context`,
which makes the built-in strategies 13–59× faster (#481).
