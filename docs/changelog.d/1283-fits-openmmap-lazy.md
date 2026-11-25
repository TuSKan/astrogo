---
type: Changed — BREAKING
pr: 1283
---
**`fits.OpenMmap` leaves each image's pixels in the mapping**, as `fits.ReadAt` does: an image's `Tensor` is nil, and `fits.ImageHDU.Section` reads a box of it until `fits.MMapFile.Close`. It used to copy every image into memory.
