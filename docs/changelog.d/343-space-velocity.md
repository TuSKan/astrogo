---
type: Added
pr: 343
---
`coord.SpaceVelocity` returns a target's velocity with respect to the solar
system barycentre in km/s, as Cartesian components on the ICRS axes, and
`coord.SpaceSpeed` its magnitude. Proper motion is an angular rate and radial
velocity is a linear one, so neither can be compared with the other; this is the
combination that Galactic UVW velocities, cluster membership tests and orbit
integrations all start from, and astrogo had no way to compute it. Validated
against Barnard's Star at 142.5 km/s, a figure published independently of this
library, and internally against the classical v = 4.74047·μ·d identity. Both
report a bool rather than a velocity when the target records no kinematics or no
usable parallax: unlike a frame conversion, where the distance divides out and
#331's fix exploits that, turning an angular rate into km/s genuinely needs the
distance — the same 150 mas/yr is 7 km/s at 10 pc and 700 at a kiloparsec — so
there is nothing to return rather than a plausible figure built on a distance
nobody supplied. This is the first of the three pieces [#335] needs before
`Galactocentric` can carry velocities; the frame-specific parts follow
separately.
