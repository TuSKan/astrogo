---
type: Fixed
pr: 777
---
**`catalog/sbdb`'s `SearchBright` says when its default limit cut a list short.** Without a `Limit` it stopped at 50 asteroids and 50 comets silently; it now asks for one more, and when there are more, follows the 50 with an error wrapping `resolve.ErrTruncated` and does not cache the cut list, as `catalog/simbad` already did. `plan.VisibleTonight` passes that on as `ErrIncomplete`, which with SBDB as a source it now returns at any magLimit of 2 or more, until #779 is fixed.
