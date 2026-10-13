---
type: Fixed
pr: 714
---
**`GetDetails` reports the same rise and set as `SunEvents`, `MoonEvents` and `VisibilityEvents`**, for the pass at the Context's time: the one in progress, or the next. It searched a geometric 0° horizon of its own, 4 to 8 minutes from every other rise and set in `plan`, and often reported a rise already past.
