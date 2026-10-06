---
type: Changed
pr: 514
---
**`satellite.Satellite.Altitude` is about 85 times faster** (50 µs to 0.6 µs)
and no longer looks up UT1 or loads EOP. It turned the position Earth-fixed by
GAST, the wrong sidereal time for TEME, to take a height no rotation about the
axis changes. Heights are unchanged.
