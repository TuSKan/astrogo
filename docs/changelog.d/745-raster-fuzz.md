---
type: Fixed
pr: 745
---
**`raster.Open` returns an error for a malformed tile layout instead of panicking.** A tiled GeoTIFF with a tile width but no tile length crashed the caller. The GeoTIFF reader is now fuzzed, which is how it was found.
