---
type: Fixed
pr: 220
---
**`fits.WCS` could be rewritten without going through a setter, in both
directions.** The getters returned the internal slice and the setters kept the
caller's, so reading `CRVAL` to inspect it, or holding the slice you passed in,
let you move an image on the sky at a distance. Both now copy, including the SIP
and TPV coefficient maps (#178).
