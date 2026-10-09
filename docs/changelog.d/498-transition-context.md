---
type: Changed
pr: 498
---
**A slewing schedule runs 6.6× faster**: `BasicTransitionModel` built a full
SOFA Context per instant of every slew, and the built-in strategies now observe
both ends through the Context they evaluate constraints with (#485).
