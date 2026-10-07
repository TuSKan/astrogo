---
type: Fixed
pr: 602
---
**`catalog/simbad` classified objects by string-matching their type codes**, so candidate stars (Aldebaran among them), most galaxies (M33, M77) and most nebulae came back as `KindOther`, and open and globular clusters as a generic cluster. Kinds now follow SIMBAD's own type hierarchy, its `otypedef` table.
