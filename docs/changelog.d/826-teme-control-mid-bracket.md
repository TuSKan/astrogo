---
type: Fixed
pr: 826
---
**The test that holds a satellite's look angle to a fresh Context's failed on
main** once #822 made a moved Context accurate enough to hide its control. It
now probes mid-bracket, where the interpolation's error peaks, and holds the
look angle to 0.01 mas.
