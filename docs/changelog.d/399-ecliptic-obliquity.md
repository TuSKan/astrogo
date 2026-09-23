---
type: Fixed
pr: 399
---
**Kepler positions are no longer tilted 0.042″ about the equinox.**
`ephemeris/kepler` rotated J2000-ecliptic elements into the equator by the
IAU 2006 obliquity, where JPL defines their ecliptic with IAU 1976's: 33 km
for C/2023 A3 at 2.5 AU. 433 Eros now matches Horizons to 0.000″ at its epoch
of osculation, where it read 0.04″ (#391).
