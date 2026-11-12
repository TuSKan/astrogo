---
type: Changed
pr: 1110
---
**Every Kepler provider shares one SOFA base and its `Epv00` memo**: `eph.Default()` built a provider, and with it an
empty memo, on every call, so `plan`'s Sun and Moon constraints paid a full `Epv00` on every check. Asking a fresh
`Default()` about an instant just evaluated goes from about 56 µs to 1.9 µs, with results unchanged.
