---
type: Fixed
pr: 780
---
**`coord.HEALPix` keeps the pixels near the poles at every nside it accepts.** `PixelOf` and `Center` worked from the sine of the latitude, which rounds to ±1 within milliarcseconds of a pole: from nside 2^24 `Center` drifted off the ring latitudes, by 4.8 pixel widths at 2^29, and from 2^28 `PixelOf` returned the wrong pixel. Both now carry the sine of the colatitude as well, as the HEALPix C++ library does.
