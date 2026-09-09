---
type: Fixed
pr: 257
---
The Horizons state comparison no longer claims both sides evaluate the same JPL
integration. Horizons serves DE441 and astrogo reads DE440, and the two differ
by 2.42 m at the Moon — most of that test's measured residual is the kernel gap
rather than astrogo, which the tolerance's derivation now accounts for (#257).
