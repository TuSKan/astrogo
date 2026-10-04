---
type: Fixed
pr: 464
---
**The scheduler returns a transition overhead it cannot compute** instead of
skipping the candidate, so a block no longer goes unplaced with no reason, or
placed with no setup time when greedy's refined estimate failed (#454).
