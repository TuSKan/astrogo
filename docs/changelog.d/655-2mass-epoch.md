---
type: Fixed
pr: 655
---
**`catalog/vizier` stamped every 2MASS row J2000**, though each position is where its source was on the night 2MASS observed it, between 1997 and 2001. Each row now carries its own observation date from the table's `JD` column, so `catalog.Resolver` can match a fast star's 2MASS row; and a merged Target's `Epoch` now comes with its `Coord`, from the provider whose position won.
