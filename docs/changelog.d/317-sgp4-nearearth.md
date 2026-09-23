---
type: Added
pr: 317
---
`ephemeris/satellite/sgp4` gains the near-Earth propagator: `New`, `At`
(minutes from epoch as a float), `AtTime`, and the model's own branch
predicates. Measured against Vallado's reference states it agrees to a maximum
of **9 nanometers** across 158 states — and the three near-Earth cases the
current dependency misses by up to 3438 km agree to 6 nanometers, confirming
the diagnosis in [#309]. Deep space (SDP4) is next. [#310]
