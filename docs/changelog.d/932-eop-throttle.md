---
type: Fixed
pr: 932
---
**With no Earth orientation loaded, a UT1, UTC or EOP lookup no longer takes a lock or allocates.** Every such call waited on one mutex and built a fresh error for an answer that cannot change until the retry cooldown ends; it now reads a stored answer, about half the time per call and none of the 2 allocations.
