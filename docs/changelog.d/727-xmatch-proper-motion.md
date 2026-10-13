---
type: Fixed
pr: 727
---
**`catalog/xmatch.Match` applies each Target's proper motion when it matches by position.** It propagated a `Coord` that no provider fills with a motion, the cause #627 found in `catalog.Resolver`, so a star faster than about 0.125″/yr never matched its own Gaia DR3 row. A row that publishes no motion is now compared at the date it was observed, as the Resolver does.
