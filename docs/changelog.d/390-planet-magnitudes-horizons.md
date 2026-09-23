---
type: Added
pr: 390
---
**Every planet's magnitude is pinned to JPL Horizons.**
`TestPlanetMagnitudesAgreeWithHorizons` holds Mercury, Venus, Jupiter, Uranus
and Neptune to 0.02 mag at 29 dates across their phase ranges (measured: 0.007
at worst). Mars is held to 0.1 mag, because the rotation and orbital-longitude
corrections of Mallama & Hilton (2018) are not yet applied (#389).
