---
type: Changed
pr: 1070
---
**`skybrightness/dataset`'s tests now cover what a `Sky` reports**, raising the package from 59.5% to 81.9% statement
coverage. The test named for `WithMagSystem` had never called it. The consent-advice test from `Open` now runs
offline on every commit instead of only in the nightly network suite.
