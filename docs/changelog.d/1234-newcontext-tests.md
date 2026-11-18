---
type: Fixed
pr: 1234
---
**Tests:** `coord`'s EOP-source test and `plan`'s sunlit-pass test build against `coord.NewContext`'s two results. Each was written against the single result it returned before, and merged after the change, so neither package's tests built on main.
