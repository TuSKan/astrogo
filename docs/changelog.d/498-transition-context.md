---
type: Added
pr: 498
---
**`plan.TransitionContext.ContextAt`**, an optional `coord.Context` source the
built-in strategies fill with their cache, so `BasicTransitionModel` slews
without a full SOFA rebuild per instant: a slewing schedule runs 6.6× faster
(#485).
