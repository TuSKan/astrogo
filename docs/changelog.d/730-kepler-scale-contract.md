---
type: Changed
pr: 730
---
**The scale-invariance contract covers `ephemeris/kepler`'s provider.** It holds every provider to one state for one instant however its time scale is labeled, and the two-body provider, added after the contract, had never been enrolled. It passes; the case is there so that reading the caller's scale raw cannot return unseen.
