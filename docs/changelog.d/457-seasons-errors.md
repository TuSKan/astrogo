---
type: Fixed
pr: 457
---
**`plan.Seasons` returns a failed refinement** rather than skipping that
season, which left a year missing an equinox or solstice with a nil error
(#453).
