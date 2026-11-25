---
type: Removed
pr: 1283
---
**The `Load` method of `fits.HDU` was removed**, with the `DataOffset` and `PayloadSize` fields nothing set: `Load` did nothing on any HDU, and `fits.ImageHDU.Section` reads an image's pixels. `fits.ErrInvalidWhence` was removed too, with `fits.ErrNegativeOffset`, which only the memory-mapped reader's seeker returned.
