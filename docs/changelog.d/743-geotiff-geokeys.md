---
type: Fixed
pr: 743
---
**`raster.Open` reads a GeoTIFF's GeoKeys.** A projected raster, which it read with its meters as degrees, is refused, and a PixelIsPoint one, which it read half a pixel off, is placed where it says. A file without GeoKeys, and VIIRS, read as before.
