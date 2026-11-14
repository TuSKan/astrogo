---
type: Fixed
pr: 1163
---
`fits.BintableHDU.GetStringColumn` and `GetFloatColumn` match a column's name without regard to case, as the FITS Standard says, so `fitscat.New` reads a table whose columns are named `ra` and `dec`. Its error now names each column it could not read.
