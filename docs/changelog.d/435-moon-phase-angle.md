---
type: Fixed
pr: 435
---
**`plan.MoonIllumination` returns the Moon's phase angle**, the Sun–Moon–Earth
angle, where it returned the elongation, and the fraction (1 + cos i)/2, up to
0.0014 closer to Skyfield's. Lunar-phase events carry that fraction as their
`Value`, so a quarter Moon is 50.13% lit, not exactly half (#416).
