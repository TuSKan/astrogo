---
type: Fixed
pr: 639
---
**`plan.FromCatalog` ignored a catalog star's epoch**, so a star whose position was given at another epoch (Gaia DR3's J2016.0, a VizieR table's own) was moved as if from J2000. Barnard's star built from Gaia's row was 166″ from the same star built from SIMBAD's. `FromCatalog` now moves the position to J2000 with the star's own proper motion, parallax and radial velocity first, and the two rows agree within 0.5″.
