---
type: Added
pr: 344
---
`coord.Galactocentric` now carries a velocity as well as a position.
`GalactocentricFrame.FromICRS` attaches one in km/s whenever the target's
kinematics can supply it, `Galactocentric.Velocity` reports whether it did, and
`ToICRS` reconstructs catalogue proper motion, parallax and radial velocity on
the way back, so the pair is a real inverse rather than one that silently drops
half the state. The Sun's velocity is a third measured frame parameter beside R₀
and z☉, and `coord.SolarVelocityFromSgrA` derives it rather than copying a
triple: the rotational component is the Sun's distance times the apparent proper
motion of Sgr A* (6.379 ± 0.024 mas/yr, Reid & Brunthaler 2004), which is the
reflex of the Sun's own orbit, while the radial and vertical components are its
peculiar motion with respect to the LSR (Schönrich, Binney & Dehnen 2010, already
cited here for `LSRDynamical`). Deriving it means the rotational component tracks
whatever R₀ the frame was built with, so a frame cannot mix one paper's distance
with another's velocity — and it reconstructs Astropy's own number exactly:
evaluated at their R₀ of 8122 pc it gives 245.6049 km/s against their published
245.6, because their V *is* R₀ × μ. Closes [#335].
