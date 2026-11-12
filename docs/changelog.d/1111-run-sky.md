---
type: Changed
pr: 1111
---
**A scheduling run computes each instant's Sun and Moon once**: the `Sun`, `MoonSep` and `MoonIllum` constraints
computed the Sun's altitude, the Moon's place and its illuminated fraction again for every target, so greedy and swap
schedules paid for the same sky once per target. A run now keeps each instant's answer for the rest of the run, and
every target sees the same sky. Twenty targets over a night under the full chain schedule 48% faster by swap passes and
6.5% faster greedily, and the schedules are identical. A `MoonIllum` given its own `Provider` still uses it.
