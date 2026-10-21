---
type: Changed — BREAKING
pr: 831
---
**A provider `Locate`s each body about its own centre, and `ephemeris.At`
composes an `Instant` whose apparent place follows SOFA's order**: against
Horizons the tail falls from 32 to 2 mas, and an astrometric place on a kernel
from about 200 µs to 2 µs. `State`, `ApparentState`, `AstrometricState`,
`Position`, `Velocity`, the `Center` type and `State.Require` are removed.
