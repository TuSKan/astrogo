---
type: Fixed
pr: 378
---
**UT1 is no longer up to a second wrong on the day before a leap second.** The
IERS series was interpolated straight across the whole-second jump in UT1−UTC,
so on each such day it ramped from one side of the step to the other: measured
on 2016-12-31, half a second out at noon and a full second — 15 arcsec of Earth
rotation — just before midnight, on every leap-second day from 1973 to 2016. The
step is now removed before interpolating.
