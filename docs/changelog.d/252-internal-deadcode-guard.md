---
type: Added
pr: 252
---
A guard reporting exported symbols in `internal/` packages that nothing in the
module names — narrow on purpose: the broad version #106 proposed reports 173
of 581 exported functions, almost all of them public API (#252).
