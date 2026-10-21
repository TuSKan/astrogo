---
type: Changed — BREAKING
pr: 828
---
**`plan.Oppositions` scans the alignment with the Sun at a step Kepler's second
law bounds, and refines each opposition on the apparent places: the same
oppositions as its daily sweep, 175–320× faster.** It needs the Sun as one of
its two bodies (`ErrOppositionNeedsSun`); `EventSolver.Find` no longer reports a
conjunction's wrap as an opposition.
