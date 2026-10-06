---
type: Fixed
pr: 525
---
**A rotated `CDi_j` matrix with ordinary sky parity was read rotated the wrong
way.** `fits.ExtractWCS` split CD into CDELT and PC by columns, which flips the
cross terms whenever the axes have opposite signs: 324″ out at 900 pixels for a
30° turn. It now splits by rows, and the WCS is held to WCSLIB through a
checked-in Astropy fixture, to 4e-10″.
