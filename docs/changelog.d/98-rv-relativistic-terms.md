---
type: Added
pr: 98
---
**Radial velocity now carries the observer's own clock.**
`coord.Context.ObserverFrameShift` adds the second-order Doppler shift, the
Sun's gravitational potential at the observer and Earth's own — about 4.65 m/s.
Against Astropy over 175 cases the disagreement falls from **4.66 m/s to
0.5 mm/s**. `BarycentricRadialVelocity` and `ObservedRadialVelocity` now return
an error.
