---
type: Fixed
pr: 734
---
**`plan.SwapOptimizedStrategy.TabuTenure` can disable tabu search.** It was documented to at 0, but every value up to and including zero became the default of 3. Zero still means 3; a negative value now disables it.
