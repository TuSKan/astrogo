---
type: Fixed
pr: 1164
---
`fitscat.New` reads RA and DEC in the unit each column's TUNITn declares, through the new `fits.BintableHDU.ColumnUnit`, instead of as degrees whatever it says: a right ascension in hours had put targets tens of degrees from where they are. A unit that is not an angle is refused with `fitscat.ErrColumnUnit`.
