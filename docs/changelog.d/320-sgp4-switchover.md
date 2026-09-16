---
type: Changed
pr: 320
---
`ephemeris/satellite` propagates through astrogo's own
`ephemeris/satellite/sgp4` instead of `github.com/joshuaferrara/go-satellite`,
which is **removed from `go.mod`**. Measured end to end through the public
wrapper against Vallado's reference states: 588 states, worst **4.1e-06 km**,
against 0.0031 km before — and the seven cases astrogo could not reproduce at
all, by up to 3438 km, now agree to nanometres. [#310]
