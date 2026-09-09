---
type: Added
pr: 254
---
An offline tier that isolates the Earth-orientation *coupling* from the
Earth-orientation *data*: varying UT1 and the pole by known amounts and
asserting astrogo responds by the amount physics requires. Modelled on
Skyfield's pinned-input NOVAS comparison, but checked against the sidereal
rate rather than a second implementation (#254).
