---
type: Fixed
pr: 1018
---
**The cache-healing paths #1016 added are tested where a real state reaches them.** These include a refused copy that cannot be removed, `GetFile`'s refusals before any I/O, CAMS files of another product or off the grid, a star map of the wrong order, a damaged VIIRS archive and TIFF together, and an early MPCORB break. A comment claiming Windows refuses to remove an open file was wrong and is gone.
