---
type: Fixed
pr: 1034
---
**The star map refuses an infinite radiance, and its build cache skips NaN and infinite ones.** `starlight.Load` used to accept a map with an infinite pixel, and its plausibility check passed it, since the contrast was infinite. The build checkpoint the Gaia aggregation resumes from used to read NaN and infinity as radiances.
