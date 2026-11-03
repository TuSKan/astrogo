---
type: Fixed
pr: 946
---
**A site reused under new weather no longer gets the old sky.** `ArtificialSkyglow` and `ScatteredMoonlight` cached scene geometry keyed only on the observer and the time. A hazy sky evaluated after a clean one at the same site and instant came out 94× too bright for artificial light. Their caches, and the natural components' frame, now key on the atmosphere, the refraction and the ephemeris they were computed from.
