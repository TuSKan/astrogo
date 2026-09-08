---
type: Changed — BREAKING
pr: 218
---
**`ephemeris`'s kernel sources now need one blank import.** `import _
"github.com/TuSKan/astrogo/ephemeris/jpl"` registers the backend `Planets`,
`SmallBody`, `Asteroids`, `Comets` and `Moons` use; without it they return an
error naming it. Asking SOFA where Mars is went from 13.9 MB and 424 packages to
4.7 MB and 224, with gRPC, OpenTelemetry, protobuf and `gocloud.dev` at zero —
64 packages of gRPC were arriving for an error-code enum. `eph.JPL` is removed;
name `jpl.Provider` (#112).
