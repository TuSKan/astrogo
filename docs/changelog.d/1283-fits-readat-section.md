---
type: Added
pr: 1283
---
**`fits.ReadAt` and `fits.ImageHDU.Section` read an image larger than memory a box at a time.** `ReadAt` reads a file over any `io.ReaderAt`, leaving each image's pixels in it, and `Section` reads only the bytes a box covers into a tensor of the box's size: a 100 × 100 box of a 320 GB image allocates 26 kB.
