---
type: Changed
pr: 1119
---
**The SOFA base owns its `Epv00` memo**: the bounded memo is a field of the one base every Kepler provider shares
rather than a second package variable, and the test that providers share that base can fail again, which with an
empty base it could not. Results and timings are unchanged.
