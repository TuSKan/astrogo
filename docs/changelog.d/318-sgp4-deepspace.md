---
type: Added
pr: 318
---
`ephemeris/satellite/sgp4` gains the deep-space (SDP4) path — lunisolar
periodics, geopotential resonance, and the Lyddane formulation. All **33** of
Vallado's reference cases now run, 666 states, agreeing to a maximum of 4.1e-06
km. Every one of the seven divergences astrogo has been reporting against the
current dependency is gone, including the one [#309] predicted would survive.
Vallado's three error-return cases are exercised for the first time. [#310]
