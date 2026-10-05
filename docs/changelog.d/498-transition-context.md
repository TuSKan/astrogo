---
type: Added
pr: 498
---
**`plan.TransitionContext.ContextAt`**, the `coord.Context` for an instant at
the site, which the built-in strategies serve from their cache, so
`BasicTransitionModel` slews without a full SOFA rebuild per instant: a
slewing schedule runs 6.6× faster (#485).
