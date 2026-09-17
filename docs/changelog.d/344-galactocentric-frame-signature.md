---
type: Changed — BREAKING
pr: 344
---
**`coord.NewGalactocentricFrame` takes the Sun's velocity as a third argument.**
It is a measured frame parameter beside R₀ and z☉ — the frame cannot place a
target's velocity without knowing its own — so it is supplied the same way they
are rather than being a hidden default. A caller passes
`coord.SolarVelocityFromSgrA(sunDistance)` for the derived value, the same
distance as the first argument so the two stay consistent, or their own
`vector.Vec3` in km/s to reproduce another frame:

```go
// before
f := coord.NewGalactocentricFrame(8178, 20.8)

// after
f := coord.NewGalactocentricFrame(8178, 20.8, coord.SolarVelocityFromSgrA(8178))
```

`coord.DefaultGalactocentricFrame` is unchanged and already does this, so code
using it needs no edit. The function being reshaped was added in [#334] and has
not appeared in a release.
