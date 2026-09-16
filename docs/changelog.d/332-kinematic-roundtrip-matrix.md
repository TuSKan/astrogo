---
type: Added
pr: 332
---
**Round-trip tests over all six elements, for every conversion between the
frames that carry kinematics.** ICRS, FK5 and FK4 give six directed
conversions; each is now exercised over a grid of eight sky positions crossed
with five kinematic profiles — 240 cases — asserting position, both proper
motion components, parallax and radial velocity. The existing tests compared
positions, which is how [#278] shipped: the proper motion was wrong by
0.6–0.9 mas/yr while the position closed to 19 µas. The matrix also turned up
[#331], where a conversion silently needs a non-zero parallax.
