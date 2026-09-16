---
type: Fixed
pr: 340
---
**The six-element frame conversions no longer label their output with an epoch
it is not at.** `ICRSToFK4` and `FK5ToFK4` answer at B1950.0 through SOFA's
`Fk524`, and `ICRSToFK5` answers at J2000.0 through `H2fk5`; none of those SOFA
routines takes an epoch, by design, because a star with a recorded proper motion
has its state stated at the catalogue equinox and moving it is a separate
operation. The conversions nonetheless stored the caller's epoch in the returned
struct, so `ICRSToFK4(star, 1975)` returned B1950 numbers reporting
`Epoch() == 1975` — the numbers right and the label wrong, which is the worse
way round, since a wrong epoch propagates into `FK4ToFK5`'s position-only route
and into anything reading `FK4.Epoch`. They now report `B1950` and `J2000Epoch`,
and the doc comments say plainly that the argument applies to the position-only
route only, why propagating instead would mean inventing a convention SOFA
declines to define (the E-terms of aberration would be evaluated at a different
epoch on each branch), and that `PropagateEpoch` is the rigorous way to move a
star to another epoch. The same defect was present in `ICRSToFK5`, which [#330]
recorded only for FK4. Closes [#330].
