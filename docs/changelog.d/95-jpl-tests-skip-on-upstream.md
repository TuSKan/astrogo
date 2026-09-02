---
type: Fixed
pr: 95
---
**A slow NAIF turned into a red build.** `ephemeris/jpl`'s untagged tests fetch
a 32 MB kernel and treated a download timeout as a test failure — four of them
sat at 120 s each and took the package past its limit. An upstream failure now
skips, as this repository's policy says it must.
