---
type: Fixed
pr: 818
---
**`kepler.Provider` placed a heliocentric orbit with SOFA's `Epv00` Earth even
over a JPL base** — 4.0 km rms and 11.7 km at worst from de440s, 1.6″ for a body
at 0.01 AU — while the Sun in the same place came from the kernel. The orbit is
now made geocentric with the base's own Sun; without `WithBase` the result is
unchanged to the bit.
