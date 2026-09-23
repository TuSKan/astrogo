---
type: Fixed
pr: 376
---
**FK4 conversions no longer invent a radial velocity for a star whose parallax
describes no distance.** A star declared at rest came back from
`FK4 → ICRS → FK4` with a velocity proportional to 1/parallax — 38.9 km/s at
1e-9 arcsec. `iauFk524` builds its pv-vector with a radius of 1.0, so the
geometry never needed the distance: measured, the direction and proper motion
are bit-identical across nine decades of parallax, and only the final
`rv = rd/(px·VF)` division was at fault. The same change closes a band where
`iauFk52h` reported complete success while implying a star moving at 0.43c.
Where the parallax is real, SOFA's answer is kept unchanged.
