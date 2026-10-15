---
type: Fixed
pr: 801
---
**`skybrightness/dataset/raster` sizes memory from the data a GeoTIFF holds, not what it claims.** A 26-byte file made `Open` allocate 34 GB. A 65 KB deflate strip of a one-pixel image inflated to 64 MiB and came back as a valid grid. Reads now grow as bytes arrive, a block that decodes past its layout is `ErrBadTIFF`, and `ReadGrid` grows its grid as pixels decode, where 2³¹−1 pixels a side panicked.
