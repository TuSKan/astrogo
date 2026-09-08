---
type: Changed
pr: 226
---
`ephemeris.ApparentState` iterates light time to convergence instead of a flat
five passes — measured, nothing in the solar system needs more than three, and
the Moon settles in one. That halves the cost of the apparent-place path this
release puts on the scheduler's hot path (#117).
