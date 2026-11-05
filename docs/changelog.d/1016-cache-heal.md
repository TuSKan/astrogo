---
type: Fixed
pr: 1016
---
**Nine dataset providers no longer keep a cached file that does not parse.** CAMS, OpenNGC, MPCORB, the MPC observatory codes, SFD dust, CIE luminosity, CALSPEC, the Gaia star map and VIIRS failed every call over such a file, an error page served with a 200 included, until it was deleted by hand. They now replace it, as `lsk` does through the same path. OpenNGC also refuses a CSV header missing a column it reads, which had made an error page parse as an empty catalog.
