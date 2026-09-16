---
type: Fixed
pr: 338
---
**The six-element frame conversions no longer need a parallax.** `ICRSToFK5`,
`FK5ToICRS`, `ICRSToFK4` and `FK4ToICRS` route through SOFA's `iauH2fk5` /
`iauFk52h`, which build a space-motion pv-vector and therefore need a distance.
A parallax below `PXMIN` was replaced by one putting the star at 10 Mpc, where
any real proper motion exceeds `VMAX = 0.5c` and the space velocity is set to
**zero** — and because those SOFA routines are `void`, the status saying so was
discarded before astrogo could see it. A star with 150 mas/yr and no recorded
parallax — most of any pre-Hipparcos catalogue — came back from a round trip
with no motion at all, and a star declared *at rest* in FK4 came back with
2.4 mas/yr in each component and 0.34 km/s it never had. `internal/gofaext` now
dispatches on `iauStarpv`'s own status and falls back to a formulation in which
the distance cancels exactly: the transformation is linear in velocity and
orthogonal in position, so dividing through by the distance leaves the proper
motion transforming on its own, with parallax and radial velocity untouched.
That is the exact limit rather than an approximation, and SOFA's route is still
taken whenever it can answer, since it carries relativistic and light-time terms
that no distance-free formulation can. Verified against SOFA in the overlapping
regime: the two agree to 8.9e-17 mas/yr for a star at rest and diverge only with
radial velocity, reaching 5.6e-05 mas/yr at 20 km/s. Closes [#331].
