---
type: Fixed
pr: 1257
---
**Tests:** the Gaia network and validation tests skip when an archive is down, whether it refuses every request or serves its own web page, and still fail on a fault of astrogo's. Each checks the archive it asks: starlight's checked that ESA was reachable while asking Gaia@AIP.
