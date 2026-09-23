---
type: Added
pr: 304
---
`coord.SunBarycentric`, `BarycentricToHeliocentric` and
`HeliocentricToBarycentric` move a position between the solar system
barycenter and the center of the Sun — the HCRS frame, which keeps the ICRS
axes and shifts only the origin. That shift reaches 0.009 AU, nearly two solar
radii, so it is half a degree seen from one AU and nine milliarcseconds seen
from a parsec. Unlike every other frame here it is a translation rather than a
rotation, so it takes a position vector rather than a direction (#126).
