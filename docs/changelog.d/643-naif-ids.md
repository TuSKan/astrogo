---
type: Fixed
pr: 643
---
**`plan.FromCatalog` read `catalog/jpl`'s NAIF IDs as astrogo body IDs**, which number the bodies differently: the JPL Sun (NAIF 10) became astrogo's body 10, the Moon, and the Moon (301) and Mars (499) failed. A NAIF major body now becomes the planet, Sun or Moon it names, and one astrogo cannot place is `ErrNoCoordinates`.
