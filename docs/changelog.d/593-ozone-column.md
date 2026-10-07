---
type: Fixed
pr: 593
---
**`atmosphere.Builder.Ozone` accepted a negative or non-finite column**, which would make ozone emit rather than absorb. `Build` now refuses one.
