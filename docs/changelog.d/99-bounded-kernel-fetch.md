---
type: Fixed
pr: 99
---
**The JPL tests still hung when NAIF stalled.** #95's skip could never fire,
because `remote.NAIFSPK` allows a 30-minute download and the test binary dies
at ten. The fetch is now bounded and attempted once per package, so a stalled
NAIF skips in seconds instead of failing the build.
