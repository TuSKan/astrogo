---
type: Added
pr: 73
---
**`kepler.LaplacePlane` — the reference plane published satellite mean elements actually use.** `Elements.WithLaplacePlane` reads the angles against the tabulated pole instead of the J2000 ecliptic; without it Io lands 16,500 km from where it belongs — 4% of its orbital radius, 5 arcsec from Earth against a 1.2 arcsec disc — with the period still correct. Measured against Horizons, the remaining two-body drift is at most 5,900 km over ten days, and it is concentrated in Io and Europa: their periods are off by 0.41% and 0.76% while Ganymede's and Callisto's agree to one part in 100,000.
