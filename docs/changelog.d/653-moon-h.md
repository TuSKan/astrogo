---
type: Fixed
pr: 653
---
**Four planetary moons' absolute magnitudes had drifted from the Horizons `V(1,0)` `plan/moons.go` cites**: Enceladus, Tethys, Titan and Hyperion, by up to 0.17 mag, so `VisibleTonight` filtered each as fainter than it is. They are Horizons' values again, and `TestMoonAbsoluteMagnitudesAreHorizons` (network) holds every moon the table sources from Horizons to its live `V(1,0)`.
