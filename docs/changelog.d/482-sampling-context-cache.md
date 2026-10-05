---
type: Changed
pr: 482
---
**`VisibleIntervals`, `TransitEstimate`, `ObservableWindows` and `Find` are
6–40× faster**: they built a full `coord.Context` per sample, and now derive
each from an hourly one as the event solver does (≲0.1″; #480).
